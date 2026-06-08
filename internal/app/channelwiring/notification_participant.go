package channelwiring

import (
	"context"
	"strings"

	"csgclaw/internal/channel/csgclaw/notification"
	notificationpull "csgclaw/internal/channel/csgclaw/notification/pull"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
)

// WireNotificationParticipantPull starts the pull supervisor for notification participants and returns the fanout deliverer.
func WireNotificationParticipantPull(ctx context.Context, participantSvc *participant.Service, imSvc *im.Service, apiBaseURL, accessToken string) notification.Fanouter {
	if participantSvc == nil {
		return nil
	}
	deliver := NewNotificationDeliver(imSvc, apiBaseURL, accessToken)
	if deliver == nil {
		return nil
	}
	go notificationpull.NewSupervisor(notificationPullSource{
		participant: participantSvc,
	}, deliver).Run(ctx)
	return deliver
}

// NewNotificationDeliver posts notification fan-out via POST /api/v1/messages.
func NewNotificationDeliver(imSvc *im.Service, apiBaseURL, accessToken string) *notification.APIDeliver {
	if imSvc == nil {
		return nil
	}
	return notification.NewAPIDeliver(imSvc, apiBaseURL, accessToken)
}

type notificationPullSource struct {
	participant *participant.Service
}

func (s notificationPullSource) Reload() error {
	return nil
}

func (s notificationPullSource) ListNotificationParticipants(channel string) ([]notificationpull.NotificationParticipant, error) {
	out := make([]notificationpull.NotificationParticipant, 0)
	if s.participant != nil {
		for _, item := range s.participant.List(participant.ListOptions{
			Channel: channel,
			Type:    participant.TypeNotification,
		}) {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			out = append(out, notificationpull.NotificationParticipant{
				ID:     id,
				UserID: item.ChannelUserRef,
			})
		}
	}
	return out, nil
}

func (s notificationPullSource) LookupNotificationParticipantForDelivery(channel, id string) (map[string]any, string, bool) {
	channel = strings.TrimSpace(channel)
	id = strings.TrimSpace(id)
	if s.participant != nil {
		item, ok := s.participant.Get(channel, id)
		if ok && strings.EqualFold(strings.TrimSpace(item.Type), participant.TypeNotification) {
			return item.Metadata, item.ChannelUserRef, true
		}
	}
	return nil, "", false
}
