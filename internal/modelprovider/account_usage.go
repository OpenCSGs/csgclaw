package modelprovider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AccountUsage struct {
	Kind      string   `json:"kind"`
	Remaining *float64 `json:"remaining,omitempty"`
	Total     *float64 `json:"total,omitempty"`
	Currency  string   `json:"currency,omitempty"`
	ResetAt   string   `json:"reset_at,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

type usageEntry struct {
	done    chan struct{}
	usage   *AccountUsage
	expires time.Time
}

// AccountUsageCache coalesces requests for the same account and caches failures too.
type AccountUsageCache struct {
	mu      sync.Mutex
	entries map[[32]byte]*usageEntry
}

func (c *AccountUsageCache) Lookup(ctx context.Context, preset, baseURL, key string) *AccountUsage {
	return c.lookup(ctx, preset, accountUsageEndpoint(preset, baseURL), key)
}

// LookupOpenCSG uses the signed-in site's accounting API and user access token.
func (c *AccountUsageCache) LookupOpenCSG(ctx context.Context, site, userUUID, token string) *AccountUsage {
	base, err := url.Parse(site)
	if err != nil || base.Scheme != "https" || base.Hostname() == "" || base.User != nil ||
		base.RawQuery != "" || base.Fragment != "" || userUUID == "" || strings.ContainsAny(userUUID, "/\\") || userUUID == "." || userUUID == ".." {
		return nil
	}
	endpoint := base.JoinPath("api/v1/accounting/credit", url.PathEscape(userUUID), "balance")
	return c.lookup(ctx, "opencsg", endpoint.String(), token)
}

// LookupCSGHubLiteBalance reads the community account from the local console API.
func LookupCSGHubLiteBalance(ctx context.Context, baseURL string) *AccountUsage {
	ctx, cancel := context.WithTimeout(ctx, accountUsageClient.Timeout)
	defer cancel()
	for _, candidate := range csgHubLiteEndpointCandidates(baseURL) {
		base, err := url.Parse(candidate)
		if err != nil || base.Scheme != "http" || base.User != nil ||
			!net.ParseIP(base.Hostname()).IsLoopback() {
			continue
		}
		endpoint := (&url.URL{Scheme: base.Scheme, Host: base.Host, Path: "/api/cloud/balance"}).String()
		if usage := fetchAccountUsage(ctx, accountUsageClient, "csghub-lite", endpoint, ""); usage != nil {
			return usage
		}
	}
	return nil
}

func (c *AccountUsageCache) lookup(ctx context.Context, preset, endpoint, key string) *AccountUsage {
	if endpoint == "" || key == "" {
		return nil
	}
	return c.lookupCached(ctx, endpoint+"\x00"+key, func() *AccountUsage {
		return fetchAccountUsage(context.Background(), accountUsageClient, preset, endpoint, key)
	})
}

func (c *AccountUsageCache) lookupCached(ctx context.Context, identity string, fetch func() *AccountUsage) *AccountUsage {
	cacheKey := sha256.Sum256([]byte(identity))
	c.mu.Lock()
	now := time.Now()
	for id, entry := range c.entries {
		if !entry.expires.IsZero() && !now.Before(entry.expires) {
			delete(c.entries, id)
		}
	}
	if entry := c.entries[cacheKey]; entry != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil
		case <-entry.done:
			return entry.usage
		}
	}
	if c.entries == nil {
		c.entries = make(map[[32]byte]*usageEntry)
	}
	entry := &usageEntry{done: make(chan struct{})}
	c.entries[cacheKey] = entry
	c.mu.Unlock()

	// A canceled list request should not cancel the shared refresh.
	go c.refresh(entry, fetch)
	select {
	case <-ctx.Done():
		return nil
	case <-entry.done:
		return entry.usage
	}
}

func (c *AccountUsageCache) refresh(entry *usageEntry, fetch func() *AccountUsage) {
	usage := fetch()
	ttl := 5 * time.Minute
	if usage == nil {
		ttl = time.Minute
	}
	c.mu.Lock()
	entry.usage = usage
	entry.expires = time.Now().Add(ttl)
	if usage != nil && usage.ResetAt != "" {
		if reset, err := time.Parse(time.RFC3339, usage.ResetAt); err == nil && reset.Before(entry.expires) {
			entry.expires = reset
		}
	}
	close(entry.done)
	c.mu.Unlock()
}

var accountUsageClient = &http.Client{
	Timeout: 1500 * time.Millisecond,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func accountUsageEndpoint(preset, baseURL string) string {
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme != "https" || base.User != nil || base.Port() != "" {
		return ""
	}
	switch preset {
	case "deepseek":
		if base.Hostname() == "api.deepseek.com" {
			return "https://api.deepseek.com/user/balance"
		}
	case "minimax-plan":
		switch base.Hostname() {
		case "api.minimax.cn":
			return "https://www.minimax.cn/v1/token_plan/remains"
		case "api.minimax.io":
			return "https://www.minimax.io/v1/token_plan/remains"
		}
	}
	return ""
}

func fetchAccountUsage(ctx context.Context, client *http.Client, preset, endpoint, key string) *AccountUsage {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	now := time.Now().UTC()
	data := json.NewDecoder(io.LimitReader(resp.Body, 128<<10))
	var usage *AccountUsage
	switch preset {
	case "opencsg":
		usage = decodeOpenCSGUsage(data)
	case "csghub-lite":
		var response struct {
			Balance *float64 `json:"balance"`
		}
		if data.Decode(&response) == nil {
			usage = cnyBalanceFromCents(response.Balance)
		}
	case "deepseek":
		usage = decodeDeepSeekUsage(data)
	case "minimax-plan":
		usage = decodeMiniMaxUsage(data, now)
	}
	if usage != nil {
		usage.UpdatedAt = now.Format(time.RFC3339)
	}
	return usage
}

// StarHub accounting returns the available balance in CNY cents.
func decodeOpenCSGUsage(data *json.Decoder) *AccountUsage {
	var response struct {
		Data struct {
			Balance *float64 `json:"balance"`
		} `json:"data"`
	}
	if data.Decode(&response) != nil {
		return nil
	}
	return cnyBalanceFromCents(response.Data.Balance)
}

func cnyBalanceFromCents(balance *float64) *AccountUsage {
	if balance == nil {
		return nil
	}
	amount := *balance / 100
	return &AccountUsage{Kind: "balance", Remaining: &amount, Currency: "CNY"}
}

func decodeDeepSeekUsage(data *json.Decoder) *AccountUsage {
	var response struct {
		Balances []struct {
			Total    string `json:"total_balance"`
			Currency string `json:"currency"`
		} `json:"balance_infos"`
	}
	if data.Decode(&response) != nil {
		return nil
	}
	for _, balance := range response.Balances {
		amount, err := strconv.ParseFloat(balance.Total, 64)
		if err == nil && !math.IsNaN(amount) && !math.IsInf(amount, 0) &&
			(balance.Currency == "CNY" || balance.Currency == "USD") {
			return &AccountUsage{Kind: "balance", Remaining: &amount, Currency: balance.Currency}
		}
	}
	return nil
}

func decodeMiniMaxUsage(data *json.Decoder, now time.Time) *AccountUsage {
	var response struct {
		Base struct {
			Code int `json:"status_code"`
		} `json:"base_resp"`
		Models []struct {
			Name             string   `json:"model_name"`
			RemainingPercent *float64 `json:"current_interval_remaining_percent"`
			LegacyRemaining  *float64 `json:"current_interval_usage_count"`
			Total            *float64 `json:"current_interval_total_count"`
			EndMillis        int64    `json:"end_time"`
		} `json:"model_remains"`
	}
	if data.Decode(&response) != nil || response.Base.Code != 0 {
		return nil
	}
	for _, model := range response.Models {
		if !strings.HasPrefix(strings.ToLower(model.Name), "minimax-m") {
			continue
		}
		remaining, total := model.LegacyRemaining, model.Total
		if percent := model.RemainingPercent; percent != nil && *percent >= 0 && *percent <= 100 {
			hundred := float64(100)
			remaining, total = percent, &hundred
		}
		if remaining == nil || total == nil || *total <= 0 || *remaining < 0 || *remaining > *total {
			continue
		}
		reset := time.UnixMilli(model.EndMillis).UTC()
		if model.EndMillis > 0 && !reset.After(now) {
			continue
		}
		usage := &AccountUsage{Kind: "quota", Remaining: remaining, Total: total}
		if model.EndMillis > 0 {
			usage.ResetAt = reset.Format(time.RFC3339)
		}
		return usage
	}
	return nil
}
