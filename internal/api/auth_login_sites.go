package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"csgclaw/internal/auth"
	"csgclaw/internal/relay"
)

const loginSitesPath = "/relay/v1/login-sites"

var loginSitesHTTPClient = &http.Client{
	Timeout: 8 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

type loginSite struct {
	ID      string                     `json:"id"`
	URL     string                     `json:"url"`
	Icon    string                     `json:"icon"`
	Locales map[string]loginSiteLocale `json:"locales"`
}

type loginSiteLocale struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type loginSiteCatalog struct {
	Sites []loginSite `json:"sites"`
}

func (h *Handler) handleAuthLoginSites(w http.ResponseWriter, r *http.Request) {
	catalog, err := fetchLoginSites(r.Context(), loginSitesHTTPClient, auth.DefaultOpenCSGBaseURL)
	if err != nil {
		writeCodedAPIError(w, http.StatusBadGateway, "login_sites_unavailable", "OpenCSG login sites are unavailable")
		return
	}
	writeJSON(w, http.StatusOK, catalog)
}

func fetchLoginSites(ctx context.Context, client relay.HTTPDoer, origin string) (loginSiteCatalog, error) {
	var catalog loginSiteCatalog
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+loginSitesPath, nil)
	if err != nil {
		return catalog, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return catalog, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return catalog, fmt.Errorf("login sites proxy returned HTTP %d", resp.StatusCode)
	}
	const limit = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return catalog, err
	}
	if len(body) > limit {
		return catalog, fmt.Errorf("login sites response exceeds %d bytes", limit)
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		return catalog, err
	}
	if len(catalog.Sites) == 0 {
		return catalog, fmt.Errorf("login sites catalog is empty")
	}
	seen := make(map[string]bool, len(catalog.Sites))
	for _, site := range catalog.Sites {
		origin, err := url.Parse(site.URL)
		if site.ID == "" || site.ID == "custom" || seen[site.ID] || err != nil || origin.Scheme != "https" ||
			origin.Hostname() == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" ||
			site.Locales["en"].Title == "" || site.Locales["zh"].Title == "" ||
			(!strings.HasPrefix(site.Icon, "data:image/svg+xml;base64,") && !strings.HasPrefix(site.Icon, "data:image/png;base64,")) {
			return catalog, fmt.Errorf("invalid login site")
		}
		seen[site.ID] = true
	}
	return catalog, nil
}
