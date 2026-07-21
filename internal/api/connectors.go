package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/agentmanager"
	"csgclaw/internal/connectors"
)

const githubConnectorCallbackPath = "/api/v1/connectors/github/oauth/callback"

type connectorConfigRequest struct {
	ClientID     string   `json:"client_id,omitempty"`
	ClientSecret *string  `json:"client_secret,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
}

type connectorOAuthStartRequest struct {
	ReturnURL string `json:"return_url,omitempty"`
}

type gitLabConnectorConfigRequest struct {
	BaseURL     string  `json:"base_url"`
	AccessToken *string `json:"access_token,omitempty"`
}

func (h *Handler) handleConnectors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireConnectorService(w)
	if !ok {
		return
	}
	statuses, err := svc.List(r.Context(), connectorLocalCallbackURL(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, statuses)
}

func (h *Handler) handleGitHubConnector(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireConnectorService(w)
	if !ok {
		return
	}
	status, err := svc.Status(r.Context(), connectors.ProviderGitHub, connectorLocalCallbackURL(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) handleGitHubConnectorConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireConnectorService(w)
	if !ok {
		return
	}
	var req connectorConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	config := connectors.Config{
		ClientID: req.ClientID,
		Scopes:   req.Scopes,
	}
	if req.ClientSecret != nil {
		config.ClientSecret = *req.ClientSecret
	}
	status, err := svc.SaveConfig(r.Context(), connectors.ProviderGitHub, config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	status.CallbackURL = connectorLocalCallbackURL(r)
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) handleGitHubConnectorOAuthStart(w http.ResponseWriter, r *http.Request) {
	var req connectorOAuthStartRequest
	switch r.Method {
	case http.MethodGet:
		req.ReturnURL = r.URL.Query().Get("return_url")
	case http.MethodPost:
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireConnectorService(w)
	if !ok {
		return
	}
	callbackURL := connectorLocalCallbackURL(r)
	if callbackURL == "" {
		http.Error(w, "local callback url is required", http.StatusBadRequest)
		return
	}
	resp, err := svc.StartOAuth(r.Context(), connectors.ProviderGitHub, connectors.OAuthStartOptions{
		CallbackURL: callbackURL,
		ReturnURL:   req.ReturnURL,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodGet {
		http.Redirect(w, r, resp.AuthorizationURL, http.StatusFound)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleGitHubConnectorAppInstallStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireConnectorService(w)
	if !ok {
		return
	}
	resp, err := svc.StartGitHubAppInstall(r.Context(), connectors.ProviderGitHub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleGitHubConnectorOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireConnectorService(w)
	if !ok {
		return
	}
	status, err := svc.CompleteOAuth(r.Context(), connectors.ProviderGitHub, r.URL.Query())
	if err != nil {
		code := http.StatusBadGateway
		if connectors.IsCallbackValidationError(err) {
			code = http.StatusBadRequest
		}
		http.Error(w, err.Error(), code)
		return
	}
	if status.AppManageable {
		start, err := svc.StartGitHubAppInstall(r.Context(), connectors.ProviderGitHub)
		if err != nil {
			http.Error(w, fmt.Sprintf("start github app install: %v", err), http.StatusInternalServerError)
			return
		}
		installURL := strings.TrimSpace(start.InstallURL)
		if installURL != "" {
			http.Redirect(w, r, installURL, http.StatusFound)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	login := "GitHub"
	if status.Account != nil && status.Account.Login != "" {
		login = status.Account.Login
	}
	_, _ = fmt.Fprintf(w, "<!doctype html><html><head><title>GitHub connected</title></head><body><h1>GitHub connected</h1><p>%s is connected. You can close this tab.</p></body></html>", html.EscapeString(login))
}

func (h *Handler) handleGitHubConnectorDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireConnectorService(w)
	if !ok {
		return
	}
	status, err := svc.Disconnect(r.Context(), connectors.ProviderGitHub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	status.CallbackURL = connectorLocalCallbackURL(r)
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) handleGitHubConnectorCredential(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	svc, ok := h.requireConnectorService(w)
	if !ok {
		return
	}
	credential, err := svc.Credential(r.Context(), connectors.ProviderGitHub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, credential)
}

func (h *Handler) handleGitLabConnector(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireConnectorService(w)
	if !ok {
		return
	}
	status, err := svc.Status(r.Context(), connectors.ProviderGitLab, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) handleGitLabConnectorConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireConnectorService(w)
	if !ok {
		return
	}
	var req gitLabConnectorConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	config := connectors.Config{BaseURL: req.BaseURL}
	if req.AccessToken != nil {
		config.AccessToken = *req.AccessToken
	}
	status, err := svc.SaveGitLabConfig(r.Context(), config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) handleGitLabConnectorDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireConnectorService(w)
	if !ok {
		return
	}
	status, err := svc.Disconnect(r.Context(), connectors.ProviderGitLab)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) handleAgentConnectorCredential(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h == nil || h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	agentID := pathValue(r, "id")
	provider := pathValue(r, "provider")
	if !h.svc.AuthorizesConnectorCapability(agentID, r.Header.Get(agent.ConnectorCapabilityHeader)) {
		http.Error(w, "connector credential capability denied", http.StatusForbidden)
		return
	}
	got, ok := h.svc.Agent(agentID)
	if !ok {
		http.Error(w, fmt.Sprintf("agent %q not found", agentID), http.StatusNotFound)
		return
	}
	connectorSvc, ok := h.requireConnectorService(w)
	if !ok {
		return
	}
	credentialProvider := agentmanager.NewConnectorServiceCredentialProvider(connectorSvc, agentmanager.DefaultConnectorGrantPolicy{})
	lease, err := credentialProvider.ManagedCredentialLease(r.Context(), agentmanager.AgentConnectorRef{
		AgentID:   got.ID,
		AgentRole: got.Role,
	}, provider)
	if err != nil {
		if errors.Is(err, agentmanager.ErrConnectorCredentialAccessDenied) {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, lease)
}

func (h *Handler) requireConnectorService(w http.ResponseWriter) (*connectors.Service, bool) {
	svc, err := h.connectorService()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, false
	}
	if svc == nil {
		http.Error(w, "connector service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return svc, true
}

func (h *Handler) connectorService() (*connectors.Service, error) {
	if h != nil && h.connectors != nil {
		return h.connectors, nil
	}
	store, err := connectors.DefaultStore()
	if err != nil {
		return nil, err
	}
	return connectors.NewService(store), nil
}

func connectorLocalCallbackURL(r *http.Request) string {
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
		Path:   githubConnectorCallbackPath,
	}
	return u.String()
}

func connectorErrorStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if errors.Is(err, http.ErrNoCookie) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
