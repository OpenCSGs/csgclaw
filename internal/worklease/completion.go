package worklease

import (
	"log/slog"
	"strings"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
)

const participantTurnStoppedText = "Conversation interrupted"

func (r *Registry) recordParticipantTurnStopped(update apitypes.ParticipantWorkUpdate) {
	if r == nil || r.im == nil ||
		update.State != apitypes.ParticipantWorkStateIdle ||
		update.StopState != apitypes.ParticipantWorkStopStateStopped {
		return
	}
	senderID := strings.TrimSpace(update.UserID)
	if senderID == "" {
		senderID = strings.TrimSpace(update.ParticipantID)
	}
	if senderID == "" || strings.TrimSpace(update.RoomID) == "" || strings.TrimSpace(update.LeaseID) == "" {
		return
	}
	_, err := r.im.DeliverMessage(im.DeliverMessageRequest{
		RoomID:       update.RoomID,
		SenderID:     senderID,
		Content:      participantTurnStoppedText,
		MessageID:    "msg-turn-stopped-" + update.LeaseID,
		ThreadRootID: update.ThreadRootID,
		Metadata: map[string]any{
			"csgclaw": map[string]any{
				"delivery_kind": "turn_stopped",
				"lease_id":      update.LeaseID,
				"request_id":    update.RequestID,
			},
		},
	})
	if err != nil {
		slog.Warn("record participant turn stop failed",
			"participant_id", update.ParticipantID,
			"room_id", update.RoomID,
			"lease_id", update.LeaseID,
			"request_id", update.RequestID,
			"error", err,
		)
	}
}
