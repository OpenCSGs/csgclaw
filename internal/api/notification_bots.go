package api

import (
	"net/http"
	"strings"

	"csgclaw/internal/channel/csgclaw/notification_bot"
	participantpkg "csgclaw/internal/participant"
)

func (h *Handler) pushNotificationParticipant(w http.ResponseWriter, r *http.Request) {
	id := pathValue(r, "id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	channel := participantChannelName(pathValue(r, "channel"))
	if channel == "" {
		http.NotFound(w, r)
		return
	}
	deps := h.notificationParticipantPushDeps(channel)
	notification_bot.ServeNotificationPush(w, r, id, deps)
}

func (h *Handler) notificationParticipantPushDeps(channel string) notification_bot.PushHTTPDeps {
	return notification_bot.PushHTTPDeps{
		Reload: func() error { return nil },
		LookupNotificationBot: func(id string) (map[string]any, string, bool) {
			if h.participant == nil {
				return nil, "", false
			}
			item, ok := h.participant.Get(channel, id)
			if ok && strings.EqualFold(item.Type, participantpkg.TypeNotification) {
				return item.Metadata, item.ChannelUserRef, true
			}
			return nil, "", false
		},
		Deliver: h.notificationDeliver,
	}
}
