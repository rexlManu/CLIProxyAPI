package auth

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

func resetTestAuth(id, provider string, now time.Time, reset time.Duration) *Auth {
	a := &Auth{ID: id, Provider: provider, Quota: QuotaState{ObservedAt: now, Signals: map[string]string{}}}
	if provider == "claude" {
		a.Quota.Signals["Anthropic-Ratelimit-Unified-7d-Reset"] = strconv.FormatInt(now.Add(reset).Unix(), 10)
		a.Quota.Signals["Anthropic-Ratelimit-Unified-7d-Utilization"] = "0.4"
	} else {
		a.Quota.Signals["X-Codex-Secondary-Reset-At"] = strconv.FormatInt(now.Add(reset).Unix(), 10)
		a.Quota.Signals["X-Codex-Secondary-Used-Percent"] = "40"
		a.Quota.Signals["X-Codex-Secondary-Window-Minutes"] = "10080"
	}
	return a
}

func TestEarliestResetSelector(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			now := time.Now()
			later := resetTestAuth("a-later", provider, now, 4*24*time.Hour)
			sooner := resetTestAuth("b-sooner", provider, now, 24*time.Hour)
			selector := &EarliestResetSelector{}
			for range 3 {
				picked, errPick := selector.Pick(context.Background(), provider, "", cliproxyexecutor.Options{}, []*Auth{later, sooner})
				if errPick != nil || picked.ID != sooner.ID {
					t.Fatalf("pick = %v, %v; want sooner", picked, errPick)
				}
			}
			sooner.Unavailable = true
			sooner.NextRetryAfter = now.Add(time.Hour)
			picked, errPick := selector.Pick(context.Background(), provider, "", cliproxyexecutor.Options{}, []*Auth{later, sooner})
			if errPick != nil || picked.ID != later.ID {
				t.Fatalf("cooldown pick = %v, %v; want later", picked, errPick)
			}
		})
	}
}

func TestQuotaResetPrefersWeeklyAndIgnoresUnusableSignals(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	a := resetTestAuth("a", "codex", now, 24*time.Hour)
	a.Quota.Signals["X-Codex-Primary-Window-Minutes"] = "300"
	a.Quota.Signals["X-Codex-Primary-Used-Percent"] = "1"
	a.Quota.Signals["X-Codex-Primary-Reset-After-Seconds"] = "3600"
	if got := quotaResetForAuth(a, "gpt-5", now); !got.Equal(now.Add(24 * time.Hour)) {
		t.Fatalf("reset = %v, want weekly reset", got)
	}
	for _, used := range []string{"NaN", "-1", "bad"} {
		a.Quota.Signals["X-Codex-Secondary-Used-Percent"] = used
		if got := quotaResetForAuth(a, "gpt-5", now); !got.Equal(now.Add(time.Hour)) {
			t.Fatalf("used %q reset = %v, want short reset", used, got)
		}
	}
	a.Quota.Signals["X-Codex-Secondary-Used-Percent"] = "100"
	if got := quotaResetForAuth(a, "gpt-5", now); !got.IsZero() {
		t.Fatalf("exhausted weekly reset = %v", got)
	}
	if got := quotaResetForAuth(a, "gpt-5", now.Add(2*time.Hour)); !got.IsZero() {
		t.Fatalf("stale reset = %v", got)
	}
}

func TestQuotaResetClaudeModelScope(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	a := resetTestAuth("a", "claude", now, 4*24*time.Hour)
	a.Quota.Signals["Anthropic-Ratelimit-Unified-7d-Opus-Reset"] = now.Add(time.Hour).Format(time.RFC3339)
	a.Quota.Signals["Anthropic-Ratelimit-Unified-7d-Opus-Utilization"] = "0.5"
	if got := quotaResetForAuth(a, "claude-sonnet-4", now); !got.Equal(now.Add(4 * 24 * time.Hour)) {
		t.Fatalf("sonnet reset = %v", got)
	}
	if got := quotaResetForAuth(a, "claude-opus-4", now); !got.Equal(now.Add(time.Hour)) {
		t.Fatalf("opus reset = %v", got)
	}
}

func TestEarliestResetSelectorFallsBackAndKeepsPriority(t *testing.T) {
	now := time.Now()
	for _, observed := range []bool{false, true} {
		selector := &EarliestResetSelector{}
		a, b := resetTestAuth("a", "claude", now, time.Hour), resetTestAuth("b", "claude", now, time.Hour)
		if !observed {
			a.Quota.Signals, b.Quota.Signals = nil, nil
		}
		for _, want := range []string{"a", "b", "a"} {
			picked, errPick := selector.Pick(context.Background(), "claude", "", cliproxyexecutor.Options{}, []*Auth{b, a})
			if errPick != nil || picked.ID != want {
				t.Fatalf("pick = %v, %v; want %s", picked, errPick, want)
			}
		}
	}
	a, b := resetTestAuth("a", "claude", now, 24*time.Hour), resetTestAuth("b", "claude", now, time.Hour)
	a.Attributes = map[string]string{"priority": "2"}
	picked, errPick := (&EarliestResetSelector{}).Pick(context.Background(), "claude", "", cliproxyexecutor.Options{}, []*Auth{a, b})
	if errPick != nil || picked.ID != "a" {
		t.Fatalf("priority pick = %v, %v", picked, errPick)
	}
}

func TestManagementQuotaObservationAffectsSelectionWithoutClearingCooldown(t *testing.T) {
	now := time.Now()
	manager := NewManager(nil, &EarliestResetSelector{}, nil)
	a := resetTestAuth("a", "claude", now.Add(-time.Minute), 4*24*time.Hour)
	a.Unavailable = true
	a.NextRetryAfter = now.Add(time.Hour)
	a.Quota.Exceeded = true
	a.Quota.NextRecoverAt = now.Add(time.Hour)
	if _, errRegister := manager.Register(WithSkipPersist(context.Background()), a); errRegister != nil {
		t.Fatal(errRegister)
	}
	headers := make(http.Header)
	headers.Set("Anthropic-Ratelimit-Unified-7d-Utilization", "0.42")
	headers.Set("Anthropic-Ratelimit-Unified-7d-Reset", strconv.FormatInt(now.Add(24*time.Hour).Unix(), 10))
	manager.ObserveQuotaHeaders(a.ID, headers, now)
	updated, _ := manager.GetByID(a.ID)
	if !updated.Unavailable || !updated.Quota.Exceeded || !updated.NextRetryAfter.Equal(a.NextRetryAfter) {
		t.Fatal("quota refresh cleared cooldown")
	}
	if got := quotaResetForAuth(updated, "claude", now); !got.Equal(time.Unix(now.Add(24*time.Hour).Unix(), 0)) {
		t.Fatalf("reset = %v", got)
	}
	manager.ObserveQuotaHeaders(a.ID, nil, now.Add(time.Second))
	manager.ObserveQuotaHeaders(a.ID, make(http.Header), now.Add(-time.Hour))
	after, _ := manager.GetByID(a.ID)
	if !after.Quota.ObservedAt.Equal(now) {
		t.Fatal("empty or old observation replaced snapshot")
	}
}
