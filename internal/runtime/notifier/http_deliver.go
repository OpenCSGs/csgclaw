package notifier

import (
	"io"
	"net/http"
	"strings"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/sandbox"
)

const maxNotifierWebhookBody = 4 << 20

// IMFanoutBridge posts notifier chat content into IM rooms and publishes message-created events.
// Implemented by the HTTP layer (e.g. api.Handler) so notifier stays free of internal/im imports.
type IMFanoutBridge interface {
	RoomIDsForMember(memberID string) []string
	CreateMessage(req apitypes.CreateMessageRequest) (apitypes.Message, error)
	User(userID string) (apitypes.User, bool)
	PublishMessageCreated(roomID, senderID string, msg apitypes.Message, sender apitypes.User)
}

// DeliverNotifierFanout posts notifier chat content to every IM room that includes the agent as a member.
func DeliverNotifierFanout(agentID, content string, bridge IMFanoutBridge) error {
	agentID = strings.TrimSpace(agentID)
	if bridge == nil {
		return nil
	}
	roomIDs := bridge.RoomIDsForMember(agentID)
	var lastErr error
	for _, rid := range roomIDs {
		msg, err := bridge.CreateMessage(apitypes.CreateMessageRequest{
			RoomID:   rid,
			SenderID: agentID,
			Content:  content,
		})
		if err != nil {
			lastErr = err
			continue
		}
		sender, ok := bridge.User(agentID)
		if ok {
			bridge.PublishMessageCreated(rid, agentID, msg, sender)
		}
	}
	return lastErr
}

// WebhookHTTPDeps supplies agent lookup and IM fanout for ServeAgentWebhook / ServeNotifyHTTP.
// When Reload or LookupNotifierAgent is nil, requests fail with 503. When Fanout is nil after auth,
// ServeAgentWebhook fails with 503 (IM not configured for delivery).
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

// NotifyHTTPPathPrefix is the URL prefix for inbound third-party notifier webhooks; the agent id
// is the single path segment after this prefix (POST only).
const NotifyHTTPPathPrefix = "/api/v1/notify/"

// ServeNotifyHTTP handles POST {NotifyHTTPPathPrefix}{agent_id} and delegates to [ServeAgentWebhook].
func ServeNotifyHTTP(w http.ResponseWriter, r *http.Request, deps WebhookHTTPDeps) {
	if !strings.HasPrefix(r.URL.Path, NotifyHTTPPathPrefix) {
		http.NotFound(w, r)
		return
	}
	agentID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, NotifyHTTPPathPrefix))
	if agentID == "" || strings.Contains(agentID, "/") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ServeAgentWebhook(w, r, agentID, deps)
}

// ServeAgentWebhook handles POST webhook delivery for a notifier agent after the caller has resolved agentID.
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
	if deps.Fanout == nil {
		http.Error(w, "service not configured", http.StatusServiceUnavailable)
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
