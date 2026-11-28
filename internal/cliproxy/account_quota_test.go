package cliproxy

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	proxyusage "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/usage"
)

func TestObservedAccountQuota(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	future := now.Add(time.Hour)
	reset := strconv.FormatInt(future.Unix(), 10)
	for _, tc := range []struct {
		provider  string
		headers   http.Header
		remaining float64
	}{
		{"codex", http.Header{"X-Codex-Primary-Used-Percent": {"100"}, "X-Codex-Primary-Reset-At": {reset}}, 0},
		{"claude", http.Header{
			"Anthropic-Ratelimit-Unified-5h-Utilization": {"0.2"},
			"Anthropic-Ratelimit-Unified-7d-Utilization": {"0.75"},
			"Anthropic-Ratelimit-Unified-7d-Reset":       {reset},
		}, 25},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			var quotas accountQuotas
			quotas.HandleUsage(t.Context(), proxyusage.Record{
				Provider: tc.provider, AuthType: "oauth", AccessTokenSHA256: "current", RequestedAt: now, ResponseHeaders: tc.headers,
			})
			// A slower request from a previous login must not replace the latest snapshot.
			quotas.HandleUsage(t.Context(), proxyusage.Record{
				Provider: tc.provider, AuthType: "oauth", AccessTokenSHA256: "previous", RequestedAt: now.Add(-time.Minute), ResponseHeaders: tc.headers,
			})
			observation := quotas.observations[tc.provider]
			got := observation.current("current", now)
			if got == nil || *got.Remaining != tc.remaining || got.ResetAt != future.UTC().Format(time.RFC3339) {
				t.Fatalf("unexpected quota: %+v", got)
			}
			if observation.current("previous", now) != nil || observation.current("current", future) != nil {
				t.Fatal("quota must be omitted after changing credentials or after the effective windows reset")
			}
		})
	}
}
