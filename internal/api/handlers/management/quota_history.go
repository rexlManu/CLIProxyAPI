package management

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

const (
	quotaHistoryFile     = "quota-history.jsonl"
	quotaHistoryInterval = 5 * time.Minute
	claudeUsageURL       = "https://api.anthropic.com/api/oauth/usage"
	codexUsageURL        = "https://chatgpt.com/backend-api/wham/usage"
)

// QuotaSample is one usage reading for one quota window of one credential.
type QuotaSample struct {
	// T is the reading time in unix seconds.
	T        int64  `json:"t"`
	Auth     string `json:"auth"`
	Provider string `json:"provider"`
	// Window names the quota window, for example "5h" or "7d" for Claude and "primary" or "secondary" for Codex.
	Window string `json:"window"`
	// Used is the consumed share of the window, 0..100.
	Used float64 `json:"used"`
	// ResetAt is when the window resets, in unix seconds.
	ResetAt int64 `json:"reset_at"`
	// PeriodHours is the window length when the provider states it, else 0.
	PeriodHours float64 `json:"period_hours,omitempty"`
}

// quotaHistory is an append-only JSON Lines file of quota samples. The panel
// draws its forecast and drain-rate charts from it, so history keeps filling
// while no panel is open. Samples are retained indefinitely.
type quotaHistory struct {
	mu   sync.Mutex
	path string
}

func (q *quotaHistory) append(samples []QuotaSample) error {
	if len(samples) == 0 {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	f, err := os.OpenFile(q.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, s := range samples {
		if err := enc.Encode(s); err != nil {
			return err
		}
	}
	return nil
}

// read returns samples with T >= since. Unreadable lines are skipped.
func (q *quotaHistory) read(since int64) ([]QuotaSample, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	f, err := os.Open(q.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []QuotaSample
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		var s QuotaSample
		if len(line) > 0 && json.Unmarshal(line, &s) == nil && s.T >= since {
			out = append(out, s)
		}
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
	}
}

// quotaSamplesFromUsage reads the official Claude and Codex usage payloads
// into samples. Other providers have no history yet.
func quotaSamplesFromUsage(auth *coreauth.Auth, body []byte, now time.Time) []QuotaSample {
	if !gjson.ValidBytes(body) {
		return nil
	}
	root := gjson.ParseBytes(body)
	var out []QuotaSample
	add := func(window string, used float64, resetAt int64, periodHours float64) {
		if used < 0 || used > 100 || resetAt <= 0 {
			return
		}
		out = append(out, QuotaSample{
			T: now.Unix(), Auth: auth.Index, Provider: auth.Provider,
			Window: window, Used: used, ResetAt: resetAt, PeriodHours: periodHours,
		})
	}
	switch strings.ToLower(auth.Provider) {
	case "claude":
		for field, window := range map[string]string{
			"five_hour": "5h", "seven_day": "7d", "seven_day_sonnet": "7d-sonnet",
			"seven_day_opus": "7d-opus", "seven_day_fable": "7d-fable",
		} {
			value := root.Get(field)
			used := value.Get("utilization")
			reset, err := time.Parse(time.RFC3339, value.Get("resets_at").String())
			if used.Type != gjson.Number || err != nil {
				continue
			}
			period := 7 * 24.0
			if field == "five_hour" {
				period = 5
			}
			add(window, used.Float(), reset.Unix(), period)
		}
	case "codex":
		for _, window := range []string{"primary", "secondary"} {
			value := root.Get("rate_limit." + window + "_window")
			used, period, reset := value.Get("used_percent"), value.Get("limit_window_seconds"), value.Get("reset_at")
			if used.Type != gjson.Number || period.Type != gjson.Number || reset.Type != gjson.Number {
				continue
			}
			add(window, used.Float(), reset.Int(), period.Float()/3600)
		}
	}
	return out
}

// recordQuotaHistory polls every enabled Claude and Codex credential once.
func (h *Handler) recordQuotaHistory(ctx context.Context) {
	if h.authManager == nil || h.quotaHistory == nil {
		return
	}
	for _, auth := range h.authManager.List() {
		if ctx.Err() != nil {
			return
		}
		if auth == nil || auth.Disabled {
			continue
		}
		var target string
		var headers map[string]string
		switch strings.ToLower(auth.Provider) {
		case "claude":
			target = claudeUsageURL
			headers = map[string]string{"User-Agent": "claude-cli/2.1.280 (external, cli)", "anthropic-beta": "oauth-2025-04-20"}
		case "codex":
			target = codexUsageURL
			headers = map[string]string{"User-Agent": "codex-tui/0.149.1 (Mac OS 26.5.2; arm64) iTerm.app/3.6.11 (codex-tui; 0.149.1)"}
			if accountID := stringValue(auth.Metadata, "account_id"); accountID != "" {
				headers["Chatgpt-Account-Id"] = accountID
			}
		default:
			continue
		}
		auth.EnsureIndex()
		token, err := h.resolveTokenForAuth(ctx, auth, "")
		if err != nil || token == "" {
			continue
		}
		reqCtx, cancel := context.WithTimeout(ctx, defaultAPICallTimeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target, nil)
		if err != nil {
			cancel()
			continue
		}
		req.Header.Set("Authorization", "Bearer "+token)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		observedAt := time.Now()
		resp, err := (&http.Client{Transport: h.apiCallTransport(auth, "")}).Do(req)
		if err != nil {
			cancel()
			log.WithError(err).Debugf("quota history: usage request failed for %s", auth.Index)
			continue
		}
		body, errRead := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		if errRead != nil || resp.StatusCode != http.StatusOK {
			continue
		}
		if parsed, errParse := url.Parse(target); errParse == nil {
			// Keep the routing selector as fresh as the charts.
			h.authManager.ObserveQuotaHeaders(auth.ID, quotaUsageHeaders(auth.Provider, parsed, body), observedAt)
		}
		if err := h.quotaHistory.append(quotaSamplesFromUsage(auth, body, observedAt)); err != nil {
			log.WithError(err).Warn("quota history: write failed")
		}
	}
}

// StartQuotaHistory records quota samples every few minutes until ctx ends.
// The file lives in the auth directory, so it survives deploys with the auth volume.
func (h *Handler) StartQuotaHistory(ctx context.Context) {
	dir, err := util.ResolveAuthDir(h.cfg.AuthDir)
	if err != nil || dir == "" {
		log.Warn("quota history disabled: auth directory unavailable")
		return
	}
	h.quotaHistory = &quotaHistory{path: filepath.Join(dir, quotaHistoryFile)}
	go func() {
		ticker := time.NewTicker(quotaHistoryInterval)
		defer ticker.Stop()
		for {
			h.recordQuotaHistory(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// GetQuotaHistory returns recorded samples. Query: since=<unix seconds>, default 24 hours.
func (h *Handler) GetQuotaHistory(c *gin.Context) {
	if h.quotaHistory == nil {
		c.JSON(http.StatusOK, gin.H{"samples": []QuotaSample{}})
		return
	}
	since := time.Now().Add(-24 * time.Hour).Unix()
	if raw := c.Query("since"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			since = v
		}
	}
	samples, err := h.quotaHistory.read(since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read quota history"})
		return
	}
	if samples == nil {
		samples = []QuotaSample{}
	}
	c.JSON(http.StatusOK, gin.H{"samples": samples})
}
