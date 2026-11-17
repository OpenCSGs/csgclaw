package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var authSiteSharedNetwork = netip.MustParsePrefix("100.64.0.0/10")

func dialAuthSite(ctx context.Context, network, address string, allowPrivate bool) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{}
	for _, address := range addresses {
		ip := address.Unmap()
		// Connect to the validated IP rather than resolving the host again.
		if !authSiteAddressAllowed(ip, allowPrivate) {
			continue
		}
		if conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port)); err == nil {
			return conn, nil
		}
	}
	return nil, errors.New("site has no reachable public address")
}

var (
	authSitePublicIPv6 = netip.MustParsePrefix("2000::/3")
	authSiteSixToFour  = netip.MustParsePrefix("2002::/16")
)

func authSiteAddressAllowed(ip netip.Addr, allowPrivate bool) bool {
	ip = ip.Unmap()
	if ip.Zone() != "" || authSiteSharedNetwork.Contains(ip) {
		return false
	}
	if ip.IsPrivate() || ip.IsLoopback() {
		return allowPrivate
	}
	return ip.IsGlobalUnicast() && (!ip.Is6() || (authSitePublicIPv6.Contains(ip) && !authSiteSixToFour.Contains(ip)))
}

func (h *Handler) handleAuthSiteCheck(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var input struct {
		BaseURL string `json:"base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeCodedAPIError(w, http.StatusBadRequest, "invalid_auth_site_url", "Invalid site address")
		return
	}
	// Private deployments can be checked only from the local management session.
	// In the usual no-auth local mode, browser requests have no explicit bearer
	// token, so validateServerAccessToken intentionally accepts that mode.
	allowPrivate := localWorkspaceClient(r) && h.validateServerAccessToken(r.Header.Get("Authorization"))
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialAuthSite(ctx, network, address, allowPrivate)
		},
		MaxResponseHeaderBytes: 64 << 10,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport:     transport,
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	latency, err := probeAuthSite(r.Context(), client, input.BaseURL)
	if err != nil {
		writeCodedAPIError(w, http.StatusBadGateway, "auth_site_unreachable", "Could not reach the site login endpoint")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"latency_ms": latency.Milliseconds()})
}

func probeAuthSite(ctx context.Context, client *http.Client, baseURL string) (time.Duration, error) {
	target, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || target.Hostname() == "" || target.User != nil || (target.Scheme != "http" && target.Scheme != "https") {
		return 0, errors.New("invalid site address")
	}
	target = target.JoinPath("sso/login")
	// Supply the required SSO parameter without sending the workspace's callback
	// URL or login state to a site that is only being probed.
	target.RawQuery = url.Values{"redirect_url": {"http://127.0.0.1" + authCallbackPath}}.Encode()
	target.Fragment, target.ForceQuery = "", false
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return 0, err
	}
	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return 0, errors.New("login endpoint returned an unsuccessful status")
	}
	return time.Since(started), nil
}
