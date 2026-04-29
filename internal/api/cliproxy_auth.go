package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"csgclaw/internal/cliproxy"
)

type cliproxyAuthLoginRequest struct {
	Provider  string `json:"provider"`
	NoBrowser bool   `json:"no_browser,omitempty"`
}

var cliproxyAuthStatus = func(r *http.Request, provider string) (cliproxy.AuthStatus, error) {
	return cliproxy.Default().AuthStatus(r.Context(), provider)
}

var cliproxyAuthLogin = func(r *http.Request, req cliproxyAuthLoginRequest) (cliproxy.AuthStatus, error) {
	return cliproxy.Default().Login(r.Context(), req.Provider, cliproxy.LoginOptions{NoBrowser: req.NoBrowser})
}

func (h *Handler) handleCLIProxyAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	if provider == "" {
		http.Error(w, "provider is required", http.StatusBadRequest)
		return
	}
	status, err := cliproxyAuthStatus(r, provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) handleCLIProxyAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req cliproxyAuthLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Provider) == "" {
		http.Error(w, "provider is required", http.StatusBadRequest)
		return
	}
	status, err := cliproxyAuthLogin(r, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, status)
}
