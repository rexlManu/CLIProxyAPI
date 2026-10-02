package executor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
)

func TestCodexUpstreamWebsocketTransportSelection(t *testing.T) {
	for _, tt := range []struct {
		name                      string
		enabled, downstream, want bool
		attributes                map[string]string
		metadata                  map[string]any
	}{
		{name: "HTTP default", want: false},
		{name: "HTTP OAuth opt in", enabled: true, want: true},
		{name: "HTTP explicit opt out", enabled: true, metadata: map[string]any{"websockets": false}, want: false},
		{name: "attribute overrides metadata", enabled: true, attributes: map[string]string{"websockets": "false"}, metadata: map[string]any{"websockets": true}, want: false},
		{name: "API key requires override", enabled: true, attributes: map[string]string{"api_key": "test"}, want: false},
		{name: "API key with override", enabled: true, attributes: map[string]string{"api_key": "test", "websockets": "true"}, want: true},
		{name: "downstream WebSocket opt in", downstream: true, metadata: map[string]any{"websockets": true}, want: true},
		{name: "downstream default unchanged", downstream: true, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.downstream {
				ctx = cliproxyexecutor.WithDownstreamWebsocket(ctx)
			}
			exec := NewCodexAutoExecutor(&config.Config{Codex: config.CodexConfig{EnableWebsocketUpstream: tt.enabled}})
			metadata := map[string]any{"access_token": "test"}
			for key, value := range tt.metadata {
				metadata[key] = value
			}
			auth := &cliproxyauth.Auth{Provider: "codex", Attributes: tt.attributes, Metadata: metadata}
			if got := exec.useWebsocketUpstream(ctx, auth); got != tt.want {
				t.Fatalf("websocket = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCodexHTTPClientUsesWebsocketUpstream(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "response", true: "stream"}[stream], func(t *testing.T) {
			var upgrades atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !websocket.IsWebSocketUpgrade(r) {
					http.Error(w, "expected WebSocket upgrade", http.StatusBadRequest)
					return
				}
				conn, errUpgrade := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if errUpgrade != nil {
					t.Errorf("upgrade: %v", errUpgrade)
					return
				}
				defer func() { _ = conn.Close() }()
				upgrades.Add(1)
				if _, _, errRead := conn.ReadMessage(); errRead != nil {
					t.Errorf("read: %v", errRead)
					return
				}
				completed := []byte(`{"type":"response.completed","response":{"id":"resp_test","model":"gpt-5-codex","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)
				if errWrite := conn.WriteMessage(websocket.TextMessage, completed); errWrite != nil {
					t.Errorf("write: %v", errWrite)
				}
			}))
			defer server.Close()
			exec := NewCodexAutoExecutor(&config.Config{Codex: config.CodexConfig{EnableWebsocketUpstream: true}})
			auth := &cliproxyauth.Auth{ID: "ws-http-test", Provider: "codex", Attributes: map[string]string{"base_url": server.URL}, Metadata: map[string]any{"access_token": "test"}}
			req := cliproxyexecutor.Request{Model: "gpt-5-codex", Payload: []byte(`{"model":"gpt-5-codex","input":[]}`)}
			opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("codex")}
			if stream {
				result, errExecute := exec.ExecuteStream(context.Background(), auth, req, opts)
				if errExecute != nil {
					t.Fatal(errExecute)
				}
				for chunk := range result.Chunks {
					if chunk.Err != nil {
						t.Fatal(chunk.Err)
					}
				}
			} else {
				if _, errExecute := exec.Execute(context.Background(), auth, req, opts); errExecute != nil {
					t.Fatal(errExecute)
				}
			}
			if upgrades.Load() != 1 {
				t.Fatalf("upgrades = %d", upgrades.Load())
			}
		})
	}
}
