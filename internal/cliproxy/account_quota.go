package cliproxy

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"csgclaw/internal/modelprovider"

	proxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	proxyusage "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/usage"
)

type quotaWindow struct {
	used  float64
	reset time.Time
}

type quotaObservation struct {
	tokenHash   string
	requestedAt time.Time
	observedAt  time.Time
	windows     []quotaWindow
}

type accountQuotas struct {
	mu           sync.Mutex
	observations map[string]quotaObservation
}

func registerAccountQuotaObserver(quotas *accountQuotas) {
	proxyusage.RegisterNamedPlugin("csgclaw-account-quota", quotas)
}

func (q *accountQuotas) HandleUsage(_ context.Context, record proxyusage.Record) {
	if record.AuthType != proxyauth.AuthKindOAuth || record.AccessTokenSHA256 == "" {
		return
	}
	provider, _, err := normalizeAuthProvider(record.Provider)
	if err != nil {
		return
	}
	now := time.Now()
	windows := readQuotaWindows(provider, record.ResponseHeaders)
	if len(windows) == 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if previous := q.observations[provider]; record.RequestedAt.Before(previous.requestedAt) {
		return
	}
	if q.observations == nil {
		q.observations = make(map[string]quotaObservation)
	}
	q.observations[provider] = quotaObservation{record.AccessTokenSHA256, record.RequestedAt, now, windows}
}

// AccountQuota reads usage for the configured OAuth credential.
func (s *Service) AccountQuota(ctx context.Context, provider string) *modelprovider.AccountUsage {
	provider, _, err := normalizeAuthProvider(provider)
	if err != nil {
		return nil
	}
	s.quota.mu.Lock()
	observation := s.quota.observations[provider]
	s.quota.mu.Unlock()
	if provider != ProviderCodex && observation.tokenHash == "" {
		return nil
	}
	cfg, err := authConfig()
	if err != nil {
		return nil
	}
	record, err := findAuth(ctx, cfg.AuthDir, provider)
	if err != nil || record == nil {
		return nil
	}
	observed := observation.current(proxyauth.AccessTokenSHA256(record), time.Now())
	if provider != ProviderCodex || record.AuthKind() != proxyauth.AuthKindOAuth {
		return observed
	}
	token := stringFromMap(record.Metadata, "access_token")
	accountID := stringFromMap(record.Metadata, "account_id")
	queried := s.codexQuota.LookupCodex(ctx, token, accountID, configuredProxyURL())
	if queried != nil && (observed == nil || queried.UpdatedAt >= observed.UpdatedAt) {
		return queried
	}
	return observed
}

func (o quotaObservation) current(tokenHash string, now time.Time) *modelprovider.AccountUsage {
	if tokenHash == "" || o.tokenHash != tokenHash {
		return nil
	}
	var selected *quotaWindow
	for _, window := range o.windows {
		if window.reset.IsZero() {
			if !now.Before(o.observedAt.Add(5 * time.Minute)) {
				continue
			}
		} else if !now.Before(window.reset) {
			continue
		}
		if selected == nil || window.used > selected.used {
			selected = &window
		}
	}
	if selected == nil {
		return nil
	}
	total := float64(100)
	remaining := total - selected.used
	usage := &modelprovider.AccountUsage{
		Kind: "quota", Total: &total, Remaining: &remaining,
		UpdatedAt: o.observedAt.UTC().Format(time.RFC3339),
	}
	if !selected.reset.IsZero() {
		usage.ResetAt = selected.reset.UTC().Format(time.RFC3339)
	}
	return usage
}

func readQuotaWindows(provider string, headers http.Header) []quotaWindow {
	var windows []quotaWindow
	add := func(usedHeader, resetHeader string, scale float64) {
		used, err := strconv.ParseFloat(headers.Get(usedHeader), 64)
		if err != nil || math.IsNaN(used) || used < 0 || used > 100/scale {
			return
		}
		window := quotaWindow{used: used * scale}
		if reset, err := strconv.ParseInt(headers.Get(resetHeader), 10, 64); err == nil && reset > 0 {
			window.reset = time.Unix(reset, 0)
		}
		windows = append(windows, window)
	}
	switch provider {
	case ProviderCodex:
		add("X-Codex-Primary-Used-Percent", "X-Codex-Primary-Reset-At", 1)
		add("X-Codex-Secondary-Used-Percent", "X-Codex-Secondary-Reset-At", 1)
	case authProviderClaude:
		add("Anthropic-Ratelimit-Unified-5h-Utilization", "Anthropic-Ratelimit-Unified-5h-Reset", 100)
		add("Anthropic-Ratelimit-Unified-7d-Utilization", "Anthropic-Ratelimit-Unified-7d-Reset", 100)
	}
	return windows
}
