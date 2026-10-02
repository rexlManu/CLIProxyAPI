package management

import (
	"net/url"
	"testing"
)

func TestQuotaUsageHeaders(t *testing.T) {
	tests := []struct{ provider, target, body, header, want string }{
		{"claude", "https://api.anthropic.com/api/oauth/usage", `{"seven_day":{"utilization":42,"resets_at":"2026-10-03T12:00:00Z"}}`, "Anthropic-Ratelimit-Unified-7d-Utilization", "0.42"},
		{"codex", "https://chatgpt.com/backend-api/wham/usage", `{"rate_limit":{"secondary_window":{"used_percent":17,"limit_window_seconds":604800,"reset_at":1791028800}}}`, "X-Codex-Secondary-Window-Minutes", "10080"},
		{"claude", "https://example.com/api/oauth/usage", `{"seven_day":{"utilization":42,"resets_at":"2026-10-03T12:00:00Z"}}`, "Anthropic-Ratelimit-Unified-7d-Utilization", ""},
		{"claude", "http://api.anthropic.com/api/oauth/usage", `{"seven_day":{"utilization":42,"resets_at":"2026-10-03T12:00:00Z"}}`, "Anthropic-Ratelimit-Unified-7d-Utilization", ""},
		{"claude", "https://api.anthropic.com/api/oauth/usage", `{"seven_day":{"utilization":null,"resets_at":"2026-10-03T12:00:00Z"}}`, "Anthropic-Ratelimit-Unified-7d-Utilization", ""},
	}
	for _, tt := range tests {
		t.Run(tt.provider+tt.target+tt.want, func(t *testing.T) {
			target, _ := url.Parse(tt.target)
			headers := quotaUsageHeaders(tt.provider, target, []byte(tt.body))
			if got := headers.Get(tt.header); got != tt.want {
				t.Fatalf("header = %q, want %q", got, tt.want)
			}
		})
	}
}
