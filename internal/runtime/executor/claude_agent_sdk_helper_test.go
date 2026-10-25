package executor

import (
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

func TestClaudeExecutorAgentSDKHaiku55TitleHelperWire(t *testing.T) {
	const sdkUA = "claude-cli/2.1.304 (external, sdk-ts, agent-sdk/0.3.276)"
	const helperBetas = "interleaved-thinking-2025-05-14,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,per-turn-control-2026-07-01,mid-conversation-tool-changes-2026-07-01,inline-tools-2026-09-15,advisor-tool-2026-03-01,effort-2025-11-24,structured-outputs-2025-12-15,extended-cache-ttl-2025-04-11"
	const sdkIdentity = "You are a Claude agent, built on Anthropic's Claude Agent SDK."
	const titleInstruction = "You are naming a coding session so the user can pick it out of a long list of sessions."
	encodedUserID, errMarshal := json.Marshal(`{"device_id":"0000000000000000000000000000000000000000000000000000000000000000","account_uuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","session_id":"11111111-2222-4333-8444-555555555555"}`)
	if errMarshal != nil {
		t.Fatal(errMarshal)
	}
	// Keep the measured title-helper shape from TestDetectClaudeCodeRequestRecognizesAgentSDKHaiku55Helpers.
	payload := []byte(`{"max_tokens":1024,"messages":[{"role":"user","content":[{"type":"text","text":"name session"}]}],"metadata":{"user_id":` + string(encodedUserID) + `},"model":"claude-haiku-5-5","output_config":{"effort":"medium","format":{"type":"json_schema","schema":{"type":"object","properties":{"title":{"type":"string"}},"required":["title"],"additionalProperties":false}}},"stream":true,"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.304.c80; cc_entrypoint=sdk-ts"},{"type":"text","text":"` + sdkIdentity + `"},{"type":"text","text":"` + titleInstruction + `"}],"tools":[]}`)
	incoming := make(http.Header)
	for name, value := range map[string]string{
		"Accept":            "application/json",
		"Accept-Encoding":   "gzip, deflate, br, zstd",
		"Content-Type":      "application/json",
		"User-Agent":        sdkUA,
		"X-App":             "cli",
		"Anthropic-Beta":    helperBetas,
		"Anthropic-Version": "2023-06-01",
		"Anthropic-Dangerous-Direct-Browser-Access": "true",
		"X-Claude-Code-Session-Id":                  "11111111-2222-4333-8444-555555555555",
		"X-Stainless-Lang":                          "js",
		"X-Stainless-Runtime":                       "node",
		"X-Stainless-Package-Version":               "0.128.0",
		"X-Stainless-Runtime-Version":               "v26.3.0",
		"X-Stainless-OS":                            "MacOS",
		"X-Stainless-Arch":                          "arm64",
		"X-Stainless-Retry-Count":                   "0",
		"X-Stainless-Timeout":                       "600",
	} {
		incoming.Set(name, value)
	}

	for _, operation := range []string{"execute", "stream"} {
		t.Run(operation, func(t *testing.T) {
			var defaultHeaders http.Header
			var defaultSystem string
			for _, tc := range []struct {
				name      string
				optIn     bool
				cloakMode string
				wantCloak bool
			}{
				{name: "default"},
				{name: "opt-in-never", optIn: true, cloakMode: "never"},
				{name: "opt-in-auto", optIn: true, cloakMode: "auto", wantCloak: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					type capturedRequest struct {
						body    []byte
						headers http.Header
					}
					requests := make(chan capturedRequest, 1)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, errRead := io.ReadAll(r.Body)
						if errRead != nil {
							t.Errorf("read mock upstream request: %v", errRead)
						}
						requests <- capturedRequest{body: body, headers: r.Header.Clone()}
						if operation == "stream" {
							w.Header().Set("Content-Type", "text/event-stream")
							_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_sdk_title\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-haiku-5-5\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
							return
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"id":"msg_sdk_title","type":"message","role":"assistant","model":"claude-haiku-5-5","content":[{"type":"text","text":"Test session"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
					}))
					defer server.Close()

					cfg, errParse := config.ParseConfigBytes([]byte(fmt.Sprintf("upstream:\n  claude:\n    cloak-agent-sdk: %t\n", tc.optIn)))
					if errParse != nil {
						t.Fatal(errParse)
					}
					auth := &cliproxyauth.Auth{
						ID: "sdk-title-helper-" + operation + "-" + tc.name,
						Attributes: map[string]string{
							"api_key":  "sk-ant-oat-sdk-title-helper-test",
							"base_url": server.URL,
						},
						Metadata: claudeOAuthTestMetadata(),
					}
					if tc.cloakMode != "" {
						auth.Attributes["cloak_mode"] = tc.cloakMode
					}
					executor := NewClaudeExecutor(cfg)
					req := cliproxyexecutor.Request{Model: "claude-haiku-5-5", Payload: payload}
					opts := cliproxyexecutor.Options{
						SourceFormat:    sdktranslator.FormatClaude,
						OriginalRequest: payload,
						Headers:         incoming.Clone(),
					}
					if operation == "stream" {
						result, errStream := executor.ExecuteStream(context.Background(), auth, req, opts)
						if errStream != nil {
							t.Fatalf("ExecuteStream() error = %v", errStream)
						}
						for chunk := range result.Chunks {
							if chunk.Err != nil {
								t.Fatalf("stream chunk error = %v", chunk.Err)
							}
						}
					} else if _, errExecute := executor.Execute(context.Background(), auth, req, opts); errExecute != nil {
						t.Fatalf("Execute() error = %v", errExecute)
					}

					var seen capturedRequest
					select {
					case seen = <-requests:
					default:
						t.Fatal("executor did not send a request to the mock upstream")
					}
					billing := gjson.GetBytes(seen.body, "system.0.text").String()
					if !strings.Contains(string(seen.body), titleInstruction) {
						t.Fatal("title-helper instruction was lost")
					}
					if tc.wantCloak {
						if ua := seen.headers.Get("User-Agent"); !strings.Contains(ua, "(external, cli)") || strings.Contains(ua, "sdk-") {
							t.Errorf("User-Agent = %q, want CLI identity", ua)
						}
						if !strings.Contains(billing, "cc_entrypoint=cli;") || strings.Contains(billing, "sdk-ts") {
							t.Errorf("billing = %q, want CLI entrypoint", billing)
						}
						if strings.Contains(gjson.GetBytes(seen.body, "system").Raw, "Claude Agent SDK") {
							t.Error("cloaked system still contains SDK identity")
						}
						if got := seen.headers.Get("X-Stainless-Package-Version"); got == "" || got == incoming.Get("X-Stainless-Package-Version") {
							t.Errorf("X-Stainless-Package-Version = %q, want CLI software profile", got)
						}
						if got := seen.headers.Get("X-App"); got != "cli" {
							t.Errorf("X-App = %q, want cli", got)
						}
						if operation == "stream" {
							if got := seen.headers.Get("Accept"); got != "text/event-stream" {
								t.Errorf("Accept = %q, want cloaked gateway stream transport", got)
							}
							if got := seen.headers.Get("Accept-Encoding"); got != "identity" {
								t.Errorf("Accept-Encoding = %q, want cloaked gateway stream transport", got)
							}
						}
						return
					}

					for name := range incoming {
						if name == "Anthropic-Beta" {
							continue
						}
						if got, want := seen.headers.Get(name), incoming.Get(name); got != want {
							t.Errorf("%s = %q, want SDK helper value %q", name, got, want)
						}
					}
					if !strings.Contains(billing, "cc_version=2.1.304.c80;") || !strings.Contains(billing, "cc_entrypoint=sdk-ts") {
						t.Errorf("billing = %q, want original SDK version and entrypoint", billing)
					}
					if got := gjson.GetBytes(seen.body, "system.1.text").String(); got != sdkIdentity {
						t.Errorf("system identity = %q, want %q", got, sdkIdentity)
					}
					if got := gjson.GetBytes(seen.body, "system.#").Int(); got != 3 {
						t.Errorf("system block count = %d, want 3", got)
					}
					// Existing Haiku policy removes effort and adds the OAuth credential beta.
					// The SDK option must preserve all other helper betas, including extended-cache-ttl.
					wantBetas := "oauth-2025-04-20," + strings.Replace(helperBetas, ",effort-2025-11-24", "", 1)
					if got := seen.headers.Get("Anthropic-Beta"); got != wantBetas {
						t.Errorf("Anthropic-Beta = %q, want helper profile %q", got, wantBetas)
					}
					if tc.name == "default" {
						defaultHeaders = seen.headers
						defaultSystem = gjson.GetBytes(seen.body, "system").Raw
					} else {
						if defaultHeaders == nil {
							t.Fatal("default request was not captured")
						}
						for name := range incoming {
							if got, want := seen.headers.Get(name), defaultHeaders.Get(name); got != want {
								t.Errorf("%s = %q, want default wire value %q", name, got, want)
							}
						}
						if got := gjson.GetBytes(seen.body, "system").Raw; got != defaultSystem {
							t.Errorf("system = %s, want default system %s", got, defaultSystem)
						}
					}
				})
			}
		})
	}
}
