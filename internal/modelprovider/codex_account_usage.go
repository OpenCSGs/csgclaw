package modelprovider

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"
)

const codexAccountUsageURL = "https://chatgpt.com/backend-api/wham/usage"

// LookupCodex reads the same ChatGPT account usage used by the Codex client.
func (c *AccountUsageCache) LookupCodex(ctx context.Context, token, accountID, proxyURL string) *AccountUsage {
	if token == "" {
		return nil
	}
	return c.lookupCached(ctx, codexAccountUsageURL+"\x00"+accountID+"\x00"+token, func() *AccountUsage {
		client := *accountUsageClient
		if proxy, err := url.Parse(proxyURL); err == nil && proxy.Host != "" {
			transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
			client.Transport = transport
			defer transport.CloseIdleConnections()
		}
		return fetchCodexAccountUsage(&client, codexAccountUsageURL, token, accountID)
	})
}

func fetchCodexAccountUsage(client *http.Client, endpoint, token, accountID string) *AccountUsage {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if accountID != "" {
		req.Header.Set("ChatGPT-Account-Id", accountID)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	return decodeCodexAccountUsage(json.NewDecoder(io.LimitReader(resp.Body, 128<<10)), time.Now().UTC())
}

func decodeCodexAccountUsage(decoder *json.Decoder, now time.Time) *AccountUsage {
	type window struct {
		Used  *float64 `json:"used_percent"`
		Reset int64    `json:"reset_at"`
	}
	var response struct {
		RateLimit struct {
			Primary   window `json:"primary_window"`
			Secondary window `json:"secondary_window"`
		} `json:"rate_limit"`
	}
	if decoder.Decode(&response) != nil {
		return nil
	}
	var usage *AccountUsage
	for _, w := range []window{response.RateLimit.Primary, response.RateLimit.Secondary} {
		if w.Used == nil || math.IsNaN(*w.Used) || *w.Used < 0 || *w.Used > 100 {
			continue
		}
		reset := time.Unix(w.Reset, 0).UTC()
		if w.Reset > 0 && !reset.After(now) {
			continue
		}
		total := float64(100)
		remaining := total - *w.Used
		if usage != nil && remaining >= *usage.Remaining {
			continue
		}
		usage = &AccountUsage{Kind: "quota", Remaining: &remaining, Total: &total, UpdatedAt: now.Format(time.RFC3339)}
		if w.Reset > 0 {
			usage.ResetAt = reset.Format(time.RFC3339)
		}
	}
	return usage
}
