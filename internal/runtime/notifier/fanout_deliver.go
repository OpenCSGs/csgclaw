package notifier

import (
	"strings"

	"csgclaw/internal/apitypes"
)

// IMFanoutBridge posts notifier chat content into IM rooms and publishes message-created events.
// Implemented by the HTTP layer (e.g. api.Handler) so notifier stays free of internal/im imports.
type IMFanoutBridge interface {
	RoomIDsForMember(memberID string) []string
	CreateMessage(req apitypes.CreateMessageRequest) (apitypes.Message, error)
	User(userID string) (apitypes.User, bool)
	PublishMessageCreated(roomID, senderID string, msg apitypes.Message, sender apitypes.User)
}

// DeliverNotifierFanout posts notifier chat content to every IM room that includes the agent as a member.
func DeliverNotifierFanout(agentID, content string, bridge IMFanoutBridge) error {
	agentID = strings.TrimSpace(agentID)
	if bridge == nil {
		return nil
	}
	roomIDs := bridge.RoomIDsForMember(agentID)
	var lastErr error
	for _, rid := range roomIDs {
		msg, err := bridge.CreateMessage(apitypes.CreateMessageRequest{
			RoomID:   rid,
			SenderID: agentID,
			Content:  content,
		})
		if err != nil {
			lastErr = err
			continue
		}
		sender, ok := bridge.User(agentID)
		if ok {
			bridge.PublishMessageCreated(rid, agentID, msg, sender)
		}
	}
	return lastErr
}
