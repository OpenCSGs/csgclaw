package notifier

import (
	"io"
	"net/http"
	"strings"

	"csgclaw/internal/sandbox"
)

const maxNotifierWebhookBody = 4 << 20

// WebhookHTTPDeps supplies agent lookup and IM fanout for ServeAgentWebhook.
type WebhookHTTPDeps struct {
	Reload func() error
	// LookupNotifierAgent returns runtime_options and agent fields needed for webhook auth.
	LookupNotifierAgent func(agentID string) (ext map[string]any, role, runtimeKind, status string, ok bool)
	Fanout              IMFanoutBridge
}

// BearerTokenFromRequest returns the bearer value from Authorization, or empty when absent.
func BearerTokenFromRequest(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// ServeAgentWebhook handles POST …/agents/{id}/webhooks/notify for notifier delivery workers.
func ServeAgentWebhook(w http.ResponseWriter, r *http.Request, agentID string, deps WebhookHTTPDeps) {
	if deps.Reload == nil || deps.LookupNotifierAgent == nil {
		http.Error(w, "service not configured", http.StatusServiceUnavailable)
		return
	}
	if err := deps.Reload(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ext, role, runtimeKind, status, ok := deps.LookupNotifierAgent(agentID)
	if !ok {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	if !IsDeliveryWorker(role, runtimeKind) {
		http.Error(w, "not a notifier delivery agent", http.StatusBadRequest)
		return
	}
	cfg := ConfigFromAgentParts(ext)
	if !cfg.AllowsWebhook() {
		http.Error(w, "webhook delivery not enabled for this agent", http.StatusForbidden)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(status), string(sandbox.StateRunning)) {
		http.Error(w, "notifier agent is not running", http.StatusServiceUnavailable)
		return
	}
	got := BearerTokenFromRequest(r)
	if !SecretMatch(cfg.WebhookToken, got) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxNotifierWebhookBody))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ct := r.Header.Get("Content-Type")
	content := FormatPayloadAsChatContent(body, ct, r.Header)
	if err := DeliverNotifierFanout(agentID, content, deps.Fanout); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
