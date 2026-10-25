package helps

import (
	"fmt"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
)

func TestDetectClaudeCodeRequestAgentSDKCloaking(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, entrypoint := range []string{"cli", "sdk-cli", "sdk-ts", "claude-vscode"} {
			for _, countTokens := range []bool{false, true} {
				t.Run(fmt.Sprintf("cloak=%t/%s/count=%t", enabled, entrypoint, countTokens), func(t *testing.T) {
					cfg, errParse := config.ParseConfigBytes([]byte(fmt.Sprintf("upstream:\n  claude:\n    cloak-agent-sdk: %t\n", enabled)))
					if errParse != nil {
						t.Fatal(errParse)
					}
					headers := confirmedClaudeCodeHeaders()
					headers.Set("User-Agent", fmt.Sprintf("claude-cli/2.1.304 (external, %s, agent-sdk/0.3.276)", entrypoint))
					payload := claudeCodeDetectionPayload(validClaudeCodeMetadataUserID)
					detection := DetectClaudeCodeRequest(headers, payload, countTokens, cfg)
					if !detection.Confirmed {
						t.Fatal("cloak preference discarded validated client identity")
					}
					if detection.Entrypoint != entrypoint || !detection.ConfirmedShape {
						t.Fatalf("original client identity was lost: %+v", detection)
					}
				})
			}
		}
	}
}
