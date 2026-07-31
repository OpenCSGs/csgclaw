package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/auth"
)

const authCallbackPath = "/api/v1/auth/callback"

type authLoginRequest struct {
	ReturnURL        string `json:"return_url,omitempty"`
	OpenCSGBaseURL   string `json:"opencsg_base_url,omitempty"`
	CSGHubBaseURL    string `json:"csghub_base_url,omitempty"`
	AIGatewayBaseURL string `json:"ai_gateway_base_url,omitempty"`
	CallbackURL      string `json:"-"`
	AdvertiseBaseURL string `json:"-"`
}

var appAuthStatus = func(r *http.Request) (auth.Status, error) {
	return auth.Default().Status(r.Context())
}

var appAuthLogin = func(r *http.Request, req authLoginRequest) (auth.LoginResponse, error) {
	return auth.Default().Login(r.Context(), auth.LoginOptions{
		ReturnURL:        req.ReturnURL,
		CallbackURL:      req.CallbackURL,
		AdvertiseBaseURL: req.AdvertiseBaseURL,
		OpenCSGBaseURL:   req.OpenCSGBaseURL,
		CSGHubBaseURL:    req.CSGHubBaseURL,
		AIGatewayBaseURL: req.AIGatewayBaseURL,
	})
}

var appAuthLogout = func(r *http.Request) (auth.Status, error) {
	return auth.Default().Logout(r.Context())
}

var appAuthCallback = func(r *http.Request, advertiseBaseURL string) (string, error) {
	values := r.URL.Query()
	if values.Get("jwt_token") == "" {
		if token := bearerToken(r.Header.Get("Authorization")); token != "" {
			values = cloneURLValues(values)
			values.Set("jwt_token", token)
		}
	}
	return auth.Default().CompleteCallback(r.Context(), values, auth.CallbackOptions{
		AdvertiseBaseURL: advertiseBaseURL,
	})
}

func (h *Handler) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := appAuthStatus(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.syncAgentHubService(r)
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	advertiseBaseURL := h.authAdvertiseBaseURL()
	redirectURL, err := appAuthCallback(r, advertiseBaseURL)
	if err != nil {
		status := http.StatusBadRequest
		reason := "invalid_callback"
		if !auth.IsCallbackValidationError(err) {
			status = http.StatusBadGateway
			reason = "account_sync_failed"
		}
		if returnURL := auth.CallbackReturnURL(r.URL.Query(), advertiseBaseURL); returnURL != "" {
			setNoStoreHeaders(w)
			http.Redirect(w, r, authCallbackResultRedirectURL(returnURL, "failed", reason), http.StatusFound)
			return
		}
		http.Error(w, "OpenCSG sign-in failed", status)
		return
	}
	if err := h.refreshOpenCSGModelProvider(r.Context()); err != nil {
		slog.Warn("refresh OpenCSG models after login failed", "error", err)
	}
	h.syncAgentHubService(r)
	h.resetEnvironmentSensitiveRuntimes()
	setNoStoreHeaders(w)
	if h.runtimeDistribution == "electron" {
		writeOAuthCompletePage(w, "Login complete", "Authentication completed. You can close this tab and return to CSGClaw.")
		return
	}
	w.Header().Set("Location", authCallbackResultRedirectURL(redirectURL, "success", ""))
	w.WriteHeader(http.StatusFound)
}

func authCallbackResultRedirectURL(rawReturnURL, result, reason string) string {
	u, err := url.Parse(strings.TrimSpace(rawReturnURL))
	if err != nil {
		return rawReturnURL
	}
	if u.Fragment != "" {
		fragment, err := url.Parse(u.Fragment)
		if err == nil {
			q := fragment.Query()
			q.Set("auth_result", result)
			if reason != "" {
				q.Set("auth_reason", reason)
			} else {
				q.Del("auth_reason")
			}
			fragment.RawQuery = q.Encode()
			u.Fragment = fragment.String()
			return u.String()
		}
	}
	q := u.Query()
	q.Set("auth_result", result)
	if reason != "" {
		q.Set("auth_reason", reason)
	} else {
		q.Del("auth_reason")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func setNoStoreHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func (h *Handler) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req authLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	if req.ReturnURL == "" {
		req.ReturnURL = r.Referer()
	}
	req.AdvertiseBaseURL = h.authAdvertiseBaseURL()
	if req.CallbackURL == "" {
		req.CallbackURL = authAdvertisedCallbackURL(req.AdvertiseBaseURL)
		if req.CallbackURL == "" {
			req.CallbackURL = authLocalCallbackURL(r)
		}
	}
	resp, err := appAuthLogin(r, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) authAdvertiseBaseURL() string {
	if h != nil && h.advertiseBaseURL != "" {
		return h.advertiseBaseURL
	}
	if h == nil || strings.TrimSpace(h.configPath) == "" {
		return ""
	}
	cfg, _, err := h.loadBootstrapConfig()
	if err != nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(cfg.Server.AdvertiseBaseURL), "/")
}

func writeOAuthCompletePage(w http.ResponseWriter, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(
		w,
		"<!doctype html><html><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width\"><title>%s</title></head><body><main><h1>%s</h1><p>%s</p></main></body></html>",
		html.EscapeString(title),
		html.EscapeString(title),
		html.EscapeString(message),
	)
}

func authAdvertisedCallbackURL(advertiseBaseURL string) string {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(advertiseBaseURL), "/"))
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return ""
	}
	u.Path = strings.TrimRight(u.Path, "/") + authCallbackPath
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func (h *Handler) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := appAuthLogout(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.resetEnvironmentSensitiveRuntimes()
	if err := h.clearOpenCSGModelProviderCache(); err != nil {
		slog.Warn("clear OpenCSG models after logout failed", "error", err)
	}
	h.syncAgentHubService(r)
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) syncAgentHubService(r *http.Request) {
	if h == nil || h.svc == nil {
		return
	}
	hubSvc, err := h.hubServiceForRequest(r)
	if err != nil {
		slog.Warn("sync agent Hub service after OpenCSG environment change failed", "error", err)
		return
	}
	h.svc.SetHubService(hubSvc)
}

func (h *Handler) resetEnvironmentSensitiveRuntimes() {
	if h == nil {
		return
	}
	reset := h.environmentRuntimeReset
	if reset == nil && h.svc != nil {
		reset = h.svc.ResetSandboxRuntimes
	}
	if reset == nil {
		return
	}
	if err := reset(); err != nil {
		slog.Warn("reset sandbox runtimes after OpenCSG environment change failed", "error", err)
	}
}

func (h *Handler) clearOpenCSGModelProviderCache() error {
	if h == nil || strings.TrimSpace(h.configPath) == "" {
		return nil
	}
	cfg, path, err := h.loadBootstrapConfig()
	if err != nil {
		return err
	}
	models, changed := agent.ClearModelProviderCachedState(cfg.Models, agent.ModelProviderIDOpenCSG)
	if !changed {
		return nil
	}
	cfg.Models = models
	return h.saveModelProvidersConfig(path, cfg)
}

func authLocalCallbackURL(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := strings.TrimSpace(r.Host)
	if host == "" && r.URL != nil {
		host = strings.TrimSpace(r.URL.Host)
	}
	if !isLocalRequestHost(host) {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	u := url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   authCallbackPath,
	}
	return u.String()
}

func isLocalRequestHost(hostport string) bool {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return false
	}
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	switch strings.ToLower(strings.Trim(host, "[]")) {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

func bearerToken(authHeader string) string {
	const prefix = "bearer "
	authHeader = strings.TrimSpace(authHeader)
	if len(authHeader) <= len(prefix) || strings.ToLower(authHeader[:len(prefix)]) != prefix {
		return ""
	}
	return strings.TrimSpace(authHeader[len(prefix):])
}

func cloneURLValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, list := range values {
		cloned[key] = append([]string(nil), list...)
	}
	return cloned
}
