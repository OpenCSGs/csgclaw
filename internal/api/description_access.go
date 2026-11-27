package api

import (
	"mime"
	"net/http"
	"net/url"
)

// Probe using the same method and browser headers as the generation requests.
func (h *Handler) getDescriptionAccess(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, struct {
		Allowed bool `json:"allowed"`
	}{Allowed: h.descriptionRequestAllowed(r)})
}

func (h *Handler) authorizeDescriptionRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.descriptionRequestAllowed(r) {
			writeCodedAPIError(w, http.StatusUnauthorized, "unauthorized", "Description generation requires a local browser or management credential")
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeCodedAPIError(w, http.StatusUnsupportedMediaType, "invalid_request", "JSON is required")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) descriptionRequestAllowed(r *http.Request) bool {
	if appRequestAgentID(r) != "" || r.Header.Get("X-CSGClaw-Caller-Agent") != "" {
		return false
	}
	if header := r.Header.Get("Authorization"); header != "" {
		return h.validServerCredential(header)
	}
	// The local Web UI does not receive the management token. Restrict its
	// requests to loopback and an Origin matching the HTTP listener.
	if !localWorkspaceClient(r) {
		return false
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	origin, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && origin.Scheme == scheme && origin.Host == r.Host &&
		origin.User == nil && origin.Path == "" && origin.RawQuery == "" && origin.Fragment == ""
}
