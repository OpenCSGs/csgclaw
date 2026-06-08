package channelwiring

import (
	"testing"
	"time"

	"csgclaw/internal/apitypes"
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

	items, err := source.ListNotificationParticipants(participant.ChannelCSGClaw)
	if err != nil {
		t.Fatalf("ListNotificationParticipants() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "alerts" || items[0].UserID != "n-alerts" {
		t.Fatalf("participants = %+v, want alerts notification participant", items)
	}

	metadata, userID, ok := source.LookupNotificationParticipantForDelivery(participant.ChannelCSGClaw, "alerts")
	if !ok {
		t.Fatal("LookupNotificationParticipantForDelivery() ok = false, want true")
	}
	if userID != "n-alerts" || metadata["remote_token"] != "secret-token" {
		t.Fatalf("lookup metadata=%#v userID=%q, want stored participant delivery config", metadata, userID)
	}
}
