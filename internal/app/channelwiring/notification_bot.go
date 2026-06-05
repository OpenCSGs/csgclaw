package channelwiring

import (
	"context"
	"strings"

	"csgclaw/internal/bot"
	"csgclaw/internal/channel/csgclaw/notification_bot"
	notificationpull "csgclaw/internal/channel/csgclaw/notification_bot/pull"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
)

// WireNotificationBotPull starts the pull supervisor for notification bots and returns the fanout deliverer.
func WireNotificationBotPull(ctx context.Context, botSvc *bot.Service, participantSvc *participant.Service, imSvc *im.Service, apiBaseURL, accessToken string) notification_bot.Fanouter {
	if botSvc == nil && participantSvc == nil {
		return nil
	}
	deliver := NewNotificationDeliver(imSvc, apiBaseURL, accessToken)
	if deliver == nil {
		return nil
	}
	go notificationpull.NewSupervisor(notificationPullSource{
		botSvc:      botSvc,
		participant: participantSvc,
	}, deliver).Run(ctx)
	return deliver
}

// NewNotificationDeliver posts notification fan-out via POST /api/v1/messages.
func NewNotificationDeliver(imSvc *im.Service, apiBaseURL, accessToken string) *notification_bot.APIDeliver {
	if imSvc == nil {
		return nil
	}
	return notification_bot.NewAPIDeliver(imSvc, apiBaseURL, accessToken)
}

type notificationPullSource struct {
	botSvc      *bot.Service
	participant *participant.Service
}

func (s notificationPullSource) Reload() error {
	if s.botSvc != nil {
		return s.botSvc.Reload()
	}
	return nil
}

func (s notificationPullSource) ListNotificationBots(channel string) ([]bot.Bot, error) {
	seen := map[string]struct{}{}
	out := make([]bot.Bot, 0)
	if s.participant != nil {
		for _, item := range s.participant.List(participant.ListOptions{
			Channel: channel,
			Type:    participant.TypeNotification,
		}) {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, notificationParticipantAsBot(item))
		}
	}
	if s.botSvc != nil {
		bots, err := s.botSvc.ListNotificationBots(channel)
		if err != nil {
			return nil, err
		}
		for _, b := range bots {
			id := strings.TrimSpace(b.ID)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			out = append(out, b)
		}
	}
	return out, nil
}

func (s notificationPullSource) LookupNotificationBotForDelivery(channel, id string) (map[string]any, string, bool) {
	channel = strings.TrimSpace(channel)
	id = strings.TrimSpace(id)
	if s.participant != nil {
		item, ok := s.participant.Get(channel, id)
		if ok && strings.EqualFold(strings.TrimSpace(item.Type), participant.TypeNotification) {
			return item.Metadata, item.ChannelUserRef, true
		}
	}
	if s.botSvc != nil {
		return s.botSvc.LookupNotificationBotForDelivery(channel, id)
	}
	return nil, "", false
}

func notificationParticipantAsBot(item participant.Participant) bot.Bot {
	return bot.Bot{
		ID:             item.ID,
		Name:           item.Name,
		Type:           bot.BotTypeNotification,
		Role:           string(bot.RoleWorker),
		Channel:        item.Channel,
		UserID:         item.ChannelUserRef,
		RuntimeOptions: item.Metadata,
		Available:      true,
		CreatedAt:      item.CreatedAt,
	}
}
