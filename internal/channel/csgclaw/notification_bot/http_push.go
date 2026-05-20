package notification_bot

import (
	"io"
	"net/http"
	"strings"
)

const maxNotificationWebhookBody = 4 << 20

// PushHTTPDeps supplies bot lookup and room delivery for inbound push notifications.
type PushHTTPDeps struct {
	Reload func() error
	// LookupNotificationBot returns runtime_options and user_id for webhook auth and delivery.
	LookupNotificationBot func(botID string) (runtimeOptions map[string]any, userID string, ok bool)
	Deliver               Fanouter
}

// BearerTokenFromRequest returns the bearer value from Authorization, or empty when absent.
func BearerTokenFromRequest(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// ServeNotificationPush handles POST push delivery for a notification bot.
func ServeNotificationPush(w http.ResponseWriter, r *http.Request, botID string, deps PushHTTPDeps) {
	if deps.Reload == nil || deps.LookupNotificationBot == nil {
		http.Error(w, "service not configured", http.StatusServiceUnavailable)
		return
	}
	botID = strings.TrimSpace(botID)
	if botID == "" {
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
	runtimeOptions, userID, ok := deps.LookupNotificationBot(botID)
	if !ok {
		http.Error(w, "notification bot not found", http.StatusNotFound)
		return
	}
	cfg := ConfigFromBotRuntimeOptions(runtimeOptions)
	if !cfg.AllowsWebhook() {
		http.Error(w, "webhook delivery not enabled for this bot", http.StatusForbidden)
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
		userID = botID
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
