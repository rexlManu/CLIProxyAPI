package management

import (
	"path/filepath"
	"testing"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestQuotaSamplesFromUsage(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)

	claude := &coreauth.Auth{Index: "c1", Provider: "claude"}
	got := quotaSamplesFromUsage(claude, []byte(`{"seven_day":{"utilization":42.5,"resets_at":"2027-01-15T08:00:00Z"},"five_hour":{"utilization":120,"resets_at":"2027-01-15T08:00:00Z"}}`), now)
	if len(got) != 1 || got[0].Window != "7d" || got[0].Used != 42.5 || got[0].PeriodHours != 168 {
		t.Fatalf("claude samples = %+v, want one 7d sample at 42.5%% (out-of-range 5h dropped)", got)
	}

	codex := &coreauth.Auth{Index: "x1", Provider: "codex"}
	got = quotaSamplesFromUsage(codex, []byte(`{"rate_limit":{"secondary_window":{"used_percent":10,"limit_window_seconds":604800,"reset_at":1800500000}}}`), now)
	if len(got) != 1 || got[0].Window != "secondary" || got[0].PeriodHours != 168 || got[0].ResetAt != 1800500000 {
		t.Fatalf("codex samples = %+v, want one secondary 168h sample", got)
	}
}

func TestQuotaHistoryRetainsOldSamplesAcrossReopen(t *testing.T) {
	q := &quotaHistory{path: filepath.Join(t.TempDir(), quotaHistoryFile)}
	now := time.Unix(1_800_000_000, 0)
	old := QuotaSample{T: now.AddDate(-2, 0, 0).Unix(), Auth: "a", Window: "7d"}
	recent := QuotaSample{T: now.Add(-time.Hour).Unix(), Auth: "a", Window: "7d", Used: 5}
	if err := q.append([]QuotaSample{old, recent}); err != nil {
		t.Fatal(err)
	}
	q = &quotaHistory{path: q.path}
	got, err := q.read(0)
	if err != nil || len(got) != 2 || got[0].T != old.T || got[1].T != recent.T {
		t.Fatalf("after reopen got %+v, err %v; want old and recent samples", got, err)
	}
	got, err = q.read(recent.T)
	if err != nil || len(got) != 1 || got[0].T != recent.T {
		t.Fatalf("filtered read got %+v, err %v; want only recent sample", got, err)
	}
	got, err = q.read(0)
	if err != nil || len(got) != 2 {
		t.Fatalf("filtered read changed stored history: got %+v, err %v", got, err)
	}
}
