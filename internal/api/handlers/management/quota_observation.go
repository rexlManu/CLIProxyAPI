package management

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// quotaUsageHeaders translates official usage responses into the same bounded
// quota signals collected from generation responses. Other endpoints are ignored.
func quotaUsageHeaders(provider string, target *url.URL, body []byte) http.Header {
	if target == nil || target.Scheme != "https" || !gjson.ValidBytes(body) {
		return nil
	}
	headers := make(http.Header)
	root := gjson.ParseBytes(body)
	switch strings.ToLower(provider) {
	case "claude":
		if target.Host != "api.anthropic.com" || target.Path != "/api/oauth/usage" {
			return nil
		}
		for field, window := range map[string]string{
			"five_hour": "5h", "seven_day": "7d", "seven_day_sonnet": "7d-sonnet",
			"seven_day_opus": "7d-opus", "seven_day_fable": "7d-fable",
		} {
			value := root.Get(field)
			used := value.Get("utilization")
			reset, errReset := time.Parse(time.RFC3339, value.Get("resets_at").String())
			if used.Type != gjson.Number || used.Float() < 0 || used.Float() > 100 || errReset != nil {
				continue
			}
			prefix := "Anthropic-Ratelimit-Unified-" + window + "-"
			headers.Set(prefix+"Utilization", strconv.FormatFloat(used.Float()/100, 'f', -1, 64))
			headers.Set(prefix+"Reset", strconv.FormatInt(reset.Unix(), 10))
		}
	case "codex":
		if target.Host != "chatgpt.com" || target.Path != "/backend-api/wham/usage" {
			return nil
		}
		for _, window := range []string{"primary", "secondary"} {
			value := root.Get("rate_limit." + window + "_window")
			used, period, reset := value.Get("used_percent"), value.Get("limit_window_seconds"), value.Get("reset_at")
			if used.Type != gjson.Number || used.Float() < 0 || used.Float() > 100 || period.Type != gjson.Number || period.Int() <= 0 || reset.Type != gjson.Number || reset.Int() <= 0 {
				continue
			}
			prefix := "X-Codex-" + window + "-"
			headers.Set(prefix+"Used-Percent", used.Raw)
			headers.Set(prefix+"Window-Minutes", strconv.FormatFloat(period.Float()/60, 'f', -1, 64))
			headers.Set(prefix+"Reset-At", reset.Raw)
		}
	}
	return headers
}
