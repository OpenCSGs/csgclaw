package api

import (
	"net/http"

	"csgclaw/internal/channel/csgclaw/notification_bot"
)

func (h *Handler) pushNotificationBot(w http.ResponseWriter, r *http.Request) {
	id := pathValue(r, "id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	channel := botChannelName(r)
	if channel == "" {
		channel = "csgclaw"
	}
	deps := h.notificationPushDeps(channel)
	notification_bot.ServeNotificationPush(w, r, id, deps)
}

func (h *Handler) notificationPushDeps(channel string) notification_bot.PushHTTPDeps {
	var reload func() error
	var lookup func(string) (map[string]any, string, bool)
	if h.botSvc != nil {
		reload = h.botSvc.Reload
		lookup = func(id string) (map[string]any, string, bool) {
			return h.botSvc.LookupNotificationBotForDelivery(channel, id)
		}
	}
	return notification_bot.PushHTTPDeps{
		Reload:                reload,
		LookupNotificationBot: lookup,
		Deliver:               h.notificationDeliver,
	}
}
