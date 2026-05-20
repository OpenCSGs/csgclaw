package channelwiring

import (
	"context"

	"csgclaw/internal/bot"
	"csgclaw/internal/channel/csgclaw/notification_bot"
	notificationpull "csgclaw/internal/channel/csgclaw/notification_bot/pull"
	"csgclaw/internal/im"
)

// WireNotificationBotPull starts the pull supervisor for notification bots and returns the fanout deliverer.
func WireNotificationBotPull(ctx context.Context, botSvc *bot.Service, imSvc *im.Service, apiBaseURL, accessToken string) notification_bot.Fanouter {
	if botSvc == nil {
		return nil
	}
	deliver := NewNotificationDeliver(imSvc, apiBaseURL, accessToken)
	if deliver == nil {
		return nil
	}
	go notificationpull.NewSupervisor(botSvc, deliver).Run(ctx)
	return deliver
}

// NewNotificationDeliver posts notification fan-out via POST /api/v1/messages.
func NewNotificationDeliver(imSvc *im.Service, apiBaseURL, accessToken string) *notification_bot.APIDeliver {
	if imSvc == nil {
		return nil
	}
	return notification_bot.NewAPIDeliver(imSvc, apiBaseURL, accessToken)
}
