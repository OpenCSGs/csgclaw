package api

import (
	"io"
	"net/http"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/im"
	"csgclaw/internal/notifier"
)

const maxNotifierWebhookBody = 4 << 20

func notifierBearerToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

// DeliverNotifierFanout posts Markdown to every IM room that includes this agent as a member and publishes SSE events.
func (h *Handler) DeliverNotifierFanout(agentID string, markdown string) error {
	return h.deliverNotifierAndPublish(agentID, markdown)
}

func (h *Handler) handleAgentNotifierWebhook(w http.ResponseWriter, r *http.Request, agentID string) {
	if h.svc == nil || h.im == nil {
		http.Error(w, "service not configured", http.StatusServiceUnavailable)
		return
	}
	if err := h.svc.Reload(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a, ok := h.svc.Agent(agentID)
	if !ok {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(a.Role), agent.RoleNotifier) {
		http.Error(w, "not a notifier agent", http.StatusBadRequest)
		return
	}
	cfg := notifier.ParseConfigFromRequestOptions(a.AgentProfile.RequestOptions)
	if !cfg.AllowsWebhook() {
		http.Error(w, "webhook delivery not enabled for this agent", http.StatusForbidden)
		return
	}
	got := notifierBearerToken(r)
	if !notifier.SecretMatch(cfg.WebhookToken, got) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxNotifierWebhookBody))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ct := r.Header.Get("Content-Type")
	md := notifier.FormatPayloadAsMarkdown(body, ct)
	if err := h.deliverNotifierAndPublish(agentID, md); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) deliverNotifierAndPublish(agentID, markdown string) error {
	agentID = strings.TrimSpace(agentID)
	if h.im == nil {
		return nil
	}
	roomIDs := h.im.RoomIDsForMember(agentID)
	var lastErr error
	for _, rid := range roomIDs {
		msg, err := h.im.CreateMessage(im.CreateMessageRequest{
			RoomID:   rid,
			SenderID: agentID,
			Content:  markdown,
		})
		if err != nil {
			lastErr = err
			continue
		}
		h.publishMessageCreated(rid, agentID, msg)
	}
	return lastErr
}
