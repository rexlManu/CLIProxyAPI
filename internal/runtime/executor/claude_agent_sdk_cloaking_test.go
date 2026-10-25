package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestClaudeExecutorAgentSDKCloaking(t *testing.T) {
	for _, mode := range []string{"default", "compatibility", "never"} {
		for _, operation := range []string{"execute", "stream", "count"} {
			t.Run(mode+"/"+operation, func(t *testing.T) {
				var seenBody []byte
				var seenHeaders http.Header
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					seenBody, _ = io.ReadAll(r.Body)
					seenHeaders = r.Header.Clone()
					if operation == "stream" {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_sdk\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-5-5\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					if operation == "count" {
						_, _ = io.WriteString(w, `{"input_tokens":1}`)
						return
					}
					_, _ = io.WriteString(w, `{"id":"msg_sdk","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[{"type":"text","text":"OK"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
				}))
				defer server.Close()

				cfg, errParse := config.ParseConfigBytes([]byte(fmt.Sprintf("upstream:\n  claude:\n    cloak-agent-sdk: %t\n", mode != "default")))
				if errParse != nil {
					t.Fatal(errParse)
				}
				auth := &cliproxyauth.Auth{
					ID: "sdk-cloaking-test",
					Attributes: map[string]string{
						"api_key":  "sk-ant-oat-sdk-cloaking-test",
						"base_url": server.URL,
					},
					Metadata: claudeOAuthTestMetadata(),
				}
				if mode == "never" {
					auth.Attributes["cloak_mode"] = "never"
				}
				userID, _ := json.Marshal(`{"device_id":"0000000000000000000000000000000000000000000000000000000000000000","account_uuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","session_id":"11111111-2222-4333-8444-555555555555"}`)
				payload := []byte(`{"model":"claude-sonnet-5-5","max_tokens":32,"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.304.179; cc_entrypoint=sdk-ts;"},{"type":"text","text":"You are a Claude agent, built on Anthropic's Claude Agent SDK."},{"type":"text","text":"keep-project-rules"}],"messages":[{"role":"user","content":"hello"}],"metadata":{"user_id":` + string(userID) + `}}`)
				const sdkUA = "claude-cli/2.1.304 (external, sdk-ts, agent-sdk/0.3.276)"
				opts := cliproxyexecutor.Options{
					SourceFormat:    sdktranslator.FormatClaude,
					OriginalRequest: payload,
					Headers: http.Header{
						"User-Agent":     {sdkUA},
						"X-App":          {"cli"},
						"Anthropic-Beta": {"claude-code-20250219"},
					},
				}
				req := cliproxyexecutor.Request{Model: "claude-sonnet-5-5", Payload: payload}
				executor := NewClaudeExecutor(cfg)
				var errRequest error
				switch operation {
				case "execute":
					_, errRequest = executor.Execute(context.Background(), auth, req, opts)
				case "stream":
					stream, errStream := executor.ExecuteStream(context.Background(), auth, req, opts)
					errRequest = errStream
					if errStream == nil {
						for chunk := range stream.Chunks {
							if chunk.Err != nil {
								errRequest = chunk.Err
								break
							}
						}
					}
				case "count":
					_, errRequest = executor.countTokensUpstream(context.Background(), auth, req, opts)
				}
				if errRequest != nil {
					t.Fatal(errRequest)
				}
				if mode == "compatibility" {
					if !strings.Contains(seenHeaders.Get("User-Agent"), "(external, cli)") {
						t.Fatalf("upstream User-Agent = %q, want CLI", seenHeaders.Get("User-Agent"))
					}
					if operation != "count" && !strings.Contains(gjson.GetBytes(seenBody, "system.0.text").String(), "cc_entrypoint=cli;") {
						t.Fatal("upstream billing block did not use the CLI entrypoint")
					}
				} else if seenHeaders.Get("User-Agent") != sdkUA {
					t.Fatalf("upstream User-Agent = %q, want original SDK identity", seenHeaders.Get("User-Agent"))
				}
				if !bytes.Contains(seenBody, []byte("keep-project-rules")) {
					t.Fatal("caller instructions were lost")
				}
			})
		}
	}
}

func TestClaudeAgentSDKCloakingPreservesSession(t *testing.T) {
	const sessionID = "11111111-2222-4333-8444-555555555555"
	var headersSessions, metadataSessions []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		headersSessions = append(headersSessions, r.Header.Get("X-Claude-Code-Session-Id"))
		metadataSessions = append(metadataSessions, gjson.Get(gjson.GetBytes(body, "metadata.user_id").String(), "session_id").String())
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg_session","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[{"type":"text","text":"OK"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer server.Close()
	cfg, errParse := config.ParseConfigBytes([]byte("upstream:\n  claude:\n    cloak-agent-sdk: true\n"))
	if errParse != nil {
		t.Fatal(errParse)
	}
	auth := &cliproxyauth.Auth{ID: "sdk-session-test", Attributes: map[string]string{"api_key": "sk-ant-oat-sdk-session-test", "base_url": server.URL}, Metadata: claudeOAuthTestMetadata()}
	userID, _ := json.Marshal(`{"device_id":"0000000000000000000000000000000000000000000000000000000000000000","account_uuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","session_id":"` + sessionID + `"}`)
	payload := []byte(`{"model":"claude-sonnet-5-5","max_tokens":32,"system":"project rules","messages":[{"role":"user","content":"hello"}],"metadata":{"user_id":` + string(userID) + `}}`)
	executor := NewClaudeExecutor(cfg)
	for _, requestID := range []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"} {
		_, errExecute := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{Model: "claude-sonnet-5-5", Payload: payload}, cliproxyexecutor.Options{
			SourceFormat: sdktranslator.FormatClaude,
			Headers: http.Header{
				"User-Agent":               {"claude-cli/2.1.304 (external, sdk-ts, agent-sdk/0.3.276)"},
				"X-App":                    {"cli"},
				"Anthropic-Beta":           {"claude-code-20250219"},
				"X-Claude-Code-Session-Id": {sessionID},
				"X-Client-Request-Id":      {requestID},
			},
		})
		if errExecute != nil {
			t.Fatal(errExecute)
		}
	}
	for _, sessions := range [][]string{headersSessions, metadataSessions} {
		if len(sessions) != 2 || sessions[0] != sessionID || sessions[1] != sessionID {
			t.Fatalf("session identities = %v, want %s for both requests", sessions, sessionID)
		}
	}
}
