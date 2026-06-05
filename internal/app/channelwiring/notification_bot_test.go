package channelwiring

import (
	"testing"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/bot"
	"csgclaw/internal/participant"
)

func TestNotificationPullSourceUsesNotificationParticipants(t *testing.T) {
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{
		{
			ID:              "alerts",
			Channel:         participant.ChannelCSGClaw,
			Type:            participant.TypeNotification,
			Name:            "Alerts",
			ChannelUserRef:  "n-alerts",
			ChannelUserKind: participant.ChannelUserKindLocalUserID,
			LifecycleStatus: participant.LifecycleStatusActive,
			Mentionable:     true,
			Metadata: map[string]any{
				"delivery_mode": "pull",
				"remote_token":  "secret-token",
			},
			CreatedAt: time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC),
		},
	}))
	source := notificationPullSource{participant: participantSvc}

	bots, err := source.ListNotificationBots(string(bot.ChannelCSGClaw))
	if err != nil {
		t.Fatalf("ListNotificationBots() error = %v", err)
	}
	if len(bots) != 1 || bots[0].ID != "alerts" || bots[0].UserID != "n-alerts" {
		t.Fatalf("bots = %+v, want participant-backed alerts notification", bots)
	}

	metadata, userID, ok := source.LookupNotificationBotForDelivery(string(bot.ChannelCSGClaw), "alerts")
	if !ok {
		t.Fatal("LookupNotificationBotForDelivery() ok = false, want true")
	}
	if userID != "n-alerts" || metadata["remote_token"] != "secret-token" {
		t.Fatalf("lookup metadata=%#v userID=%q, want stored participant delivery config", metadata, userID)
	}
}
