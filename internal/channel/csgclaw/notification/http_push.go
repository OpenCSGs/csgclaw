package notification

import (
	"io"
	"net/http"
	"strings"
)

const maxNotificationWebhookBody = 4 << 20

// PushHTTPDeps supplies participant lookup and room delivery for inbound push notifications.
type PushHTTPDeps struct {
	Reload func() error
	// LookupNotificationParticipant returns metadata and user_id for webhook auth and delivery.
	LookupNotificationParticipant func(participantID string) (metadata map[string]any, userID string, ok bool)
	Deliver                       Fanouter
}

// BearerTokenFromRequest returns the bearer value from Authorization, or empty when absent.
func BearerTokenFromRequest(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// ServeNotificationPush handles POST push delivery for a notification participant.
func ServeNotificationPush(w http.ResponseWriter, r *http.Request, participantID string, deps PushHTTPDeps) {
	if deps.Reload == nil || deps.LookupNotificationParticipant == nil {
		http.Error(w, "service not configured", http.StatusServiceUnavailable)
		return
	}
	participantID = strings.TrimSpace(participantID)
	if participantID == "" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := deps.Reload(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	metadata, userID, ok := deps.LookupNotificationParticipant(participantID)
	if !ok {
		http.Error(w, "notification participant not found", http.StatusNotFound)
		return
	}
	cfg := ConfigFromMetadata(metadata)
	if !cfg.AllowsWebhook() {
		http.Error(w, "webhook delivery not enabled for this participant", http.StatusForbidden)
		return
	}
	got := BearerTokenFromRequest(r)
	if !SecretMatch(cfg.WebhookToken, got) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if deps.Deliver == nil {
		http.Error(w, "service not configured", http.StatusServiceUnavailable)
		return
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		userID = participantID
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxNotificationWebhookBody))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ct := r.Header.Get("Content-Type")
	content := FormatPayloadAsChatContent(body, ct, r.Header)
	if err := deps.Deliver.DeliverFanout(userID, content); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
