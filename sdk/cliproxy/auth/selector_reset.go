package auth

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

// EarliestResetSelector uses quota before its next reset. Equal or unknown reset
// times use round-robin. Availability, explicit priorities and session bindings
// are still applied by the manager and the session-affinity selector.
type EarliestResetSelector struct {
	fallback RoundRobinSelector
}

func (s *EarliestResetSelector) Pick(ctx context.Context, provider, model string, opts cliproxyexecutor.Options, auths []*Auth) (*Auth, error) {
	now := time.Now()
	available, errAvailable := getSelectorAvailableAuths(ctx, auths, provider, model, now)
	if errAvailable != nil {
		return nil, errAvailable
	}
	available = preferCodexWebsocketAuths(ctx, provider, available)
	var earliest time.Time
	preferred := make([]*Auth, 0, len(available))
	for _, candidate := range available {
		reset := quotaResetForAuth(candidate, model, now)
		if reset.IsZero() {
			continue
		}
		if earliest.IsZero() || reset.Before(earliest) {
			earliest = reset
			preferred = preferred[:0]
		}
		if reset.Equal(earliest) {
			preferred = append(preferred, candidate)
		}
	}
	if len(preferred) > 0 {
		available = preferred
	}
	// These candidates have already passed route-specific availability checks.
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, prevalidatedAuthCandidatesKey{}, true)
	return s.fallback.Pick(ctx, provider, model, opts, available)
}

// quotaResetForAuth prefers the longest reported quota window, then its earliest
// reset. A five-hour reset must not hide unused weekly quota expiring tomorrow.
// Past resets and exhausted windows do not provide a usable routing signal.
func quotaResetForAuth(auth *Auth, model string, now time.Time) time.Time {
	if auth == nil {
		return time.Time{}
	}
	headers := make(map[string]string, len(auth.Quota.Signals))
	for name, value := range auth.Quota.Signals {
		headers[strings.ToLower(name)] = value
	}
	var selected time.Time
	var longest float64
	exhausted := false
	consider := func(prefix string, period float64, usedScale float64) {
		used, errUsed := strconv.ParseFloat(headers[prefix+"utilization"], 64)
		if usedScale == 100 {
			used, errUsed = strconv.ParseFloat(headers[prefix+"used-percent"], 64)
		}
		if errUsed != nil || math.IsNaN(used) || math.IsInf(used, 0) || used < 0 {
			return
		}
		resetText := headers[prefix+"reset"]
		if usedScale == 100 {
			resetText = headers[prefix+"reset-at"]
		}
		var reset time.Time
		if seconds, errSeconds := strconv.ParseInt(resetText, 10, 64); errSeconds == nil && seconds > 0 {
			reset = time.Unix(seconds, 0)
		} else if parsed, errParse := time.Parse(time.RFC3339, resetText); errParse == nil {
			reset = parsed
		} else if usedScale == 100 && !auth.Quota.ObservedAt.IsZero() {
			seconds, errSeconds := strconv.ParseInt(headers[prefix+"reset-after-seconds"], 10, 64)
			if errSeconds == nil && seconds >= 0 && seconds <= int64((365*24*time.Hour)/time.Second) {
				reset = auth.Quota.ObservedAt.Add(time.Duration(seconds) * time.Second)
			}
		}
		if !reset.After(now) || period <= 0 || math.IsNaN(period) || math.IsInf(period, 0) {
			return
		}
		if used >= usedScale {
			exhausted = true
			return
		}
		if selected.IsZero() || period > longest || (period == longest && reset.Before(selected)) {
			selected, longest = reset, period
		}
	}
	switch strings.ToLower(auth.Provider) {
	case "claude":
		consider("anthropic-ratelimit-unified-5h-", 5*60, 1)
		consider("anthropic-ratelimit-unified-7d-", 7*24*60, 1)
		for _, family := range []string{"sonnet", "opus", "fable"} {
			if strings.Contains(strings.ToLower(model), family) {
				consider("anthropic-ratelimit-unified-7d-"+family+"-", 7*24*60, 1)
			}
		}
	case "codex":
		for _, window := range []string{"primary", "secondary"} {
			prefix := "x-codex-" + window + "-"
			minutes, _ := strconv.ParseFloat(headers[prefix+"window-minutes"], 64)
			consider(prefix, minutes, 100)
		}
	}
	if exhausted {
		return time.Time{}
	}
	return selected
}
