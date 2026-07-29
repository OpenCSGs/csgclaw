package server

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type DesktopOptions struct {
	BaseURL           string
	SessionToken      string
	ServerAccessToken string
	ServerAccessHosts []string
}

func desktopRendererSecurityHandler(next http.Handler, listenerAddr net.Addr, opts DesktopOptions) (http.Handler, error) {
	if next == nil {
		return nil, fmt.Errorf("desktop renderer handler is required")
	}
	token := strings.TrimSpace(opts.SessionToken)
	if len(token) < 43 {
		return nil, fmt.Errorf("desktop session token must contain at least 256 bits")
	}

	baseURL, err := url.Parse(strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/"))
	if err != nil || baseURL.Scheme != "http" || baseURL.User != nil || baseURL.Path != "" || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, fmt.Errorf("invalid desktop base URL")
	}
	if baseURL.Hostname() != "127.0.0.1" {
		return nil, fmt.Errorf("desktop base URL must use 127.0.0.1")
	}
	if listenerAddr == nil || baseURL.Host != listenerAddr.String() {
		return nil, fmt.Errorf("desktop base URL does not match listener")
	}

	expectedOrigin := baseURL.Scheme + "://" + baseURL.Host
	expectedSessionAuthorization := "Bearer " + token
	expectedServerAuthorization := ""
	if serverToken := strings.TrimSpace(opts.ServerAccessToken); serverToken != "" {
		expectedServerAuthorization = "Bearer " + serverToken
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setDesktopSecurityHeaders(w.Header())
		got := r.Header.Get("Authorization")
		sessionAuthorized := subtle.ConstantTimeCompare([]byte(got), []byte(expectedSessionAuthorization)) == 1
		serverAuthorized := expectedServerAuthorization != "" &&
			subtle.ConstantTimeCompare([]byte(got), []byte(expectedServerAuthorization)) == 1
		requestHost := strings.ToLower(strings.TrimSpace(r.Host))
		origin := strings.TrimSpace(r.Header.Get("Origin"))

		if requestHost != strings.ToLower(baseURL.Host) {
			http.Error(w, "invalid host", http.StatusBadRequest)
			return
		}
		if origin != "" && origin != expectedOrigin {
			http.Error(w, "invalid origin", http.StatusForbidden)
			return
		}
		if desktopAuthExemptPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if sessionAuthorized || serverAuthorized {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="csgclaw-desktop"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}), nil
}

func desktopSandboxSecurityHandler(next http.Handler, listenerAddr net.Addr, opts DesktopOptions) (http.Handler, error) {
	if next == nil {
		return nil, fmt.Errorf("desktop sandbox handler is required")
	}
	if listenerAddr == nil {
		return nil, fmt.Errorf("desktop sandbox listener is required")
	}
	_, listenerPort, err := net.SplitHostPort(listenerAddr.String())
	if err != nil || listenerPort == "" {
		return nil, fmt.Errorf("invalid desktop sandbox listener address")
	}
	serverAccessHosts, err := normalizeDesktopServerAccessHosts(opts.ServerAccessHosts, listenerPort)
	if err != nil {
		return nil, err
	}
	expectedServerAuthorization := ""
	if serverToken := strings.TrimSpace(opts.ServerAccessToken); serverToken != "" {
		expectedServerAuthorization = "Bearer " + serverToken
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setDesktopSecurityHeaders(w.Header())
		requestHost := strings.ToLower(strings.TrimSpace(r.Host))
		if _, ok := serverAccessHosts[requestHost]; !ok {
			http.Error(w, "invalid host", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(r.Header.Get("Origin")) != "" {
			http.Error(w, "invalid origin", http.StatusForbidden)
			return
		}
		got := r.Header.Get("Authorization")
		serverAuthorized := expectedServerAuthorization != "" &&
			subtle.ConstantTimeCompare([]byte(got), []byte(expectedServerAuthorization)) == 1
		if !serverAuthorized {
			w.Header().Set("WWW-Authenticate", `Bearer realm="csgclaw-desktop"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}), nil
}

func normalizeDesktopServerAccessHosts(hosts []string, expectedPort string) (map[string]struct{}, error) {
	normalized := make(map[string]struct{}, len(hosts))
	for _, rawHost := range hosts {
		rawHost = strings.TrimSpace(rawHost)
		if rawHost == "" {
			continue
		}
		parsed, err := url.Parse("http://" + rawHost)
		if err != nil || parsed.User != nil || parsed.Hostname() == "" || parsed.Path != "" ||
			parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Port() != expectedPort {
			return nil, fmt.Errorf("invalid desktop server access host %q", rawHost)
		}
		normalized[strings.ToLower(parsed.Host)] = struct{}{}
	}
	return normalized, nil
}

func desktopAuthExemptPath(path string) bool {
	switch path {
	case "/healthz", "/api/v1/auth/callback", "/api/v1/connectors/github/oauth/callback":
		return true
	default:
		return false
	}
}

func setDesktopSecurityHeaders(header http.Header) {
	header.Set("Content-Security-Policy", strings.Join([]string{
		"default-src 'self'",
		"base-uri 'self'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: blob: https:",
		"font-src 'self' data:",
		"connect-src 'self' https: wss:",
		"worker-src 'self' blob:",
		"frame-src 'none'",
		"form-action 'self' https:",
	}, "; "))
	header.Set("Cross-Origin-Opener-Policy", "same-origin")
	header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), display-capture=(), usb=(), serial=(), hid=()")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
}
