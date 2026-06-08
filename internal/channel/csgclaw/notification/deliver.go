package notification

import (
	"strings"

	"csgclaw/internal/apitypes"
)

// RoomMessenger delivers notification chat content to IM rooms.
type RoomMessenger interface {
	RoomIDsForMember(memberID string) []string
	PostMessage(req apitypes.CreateMessageRequest) error
}

// Fanouter delivers notification chat content to IM rooms.
type Fanouter interface {
	DeliverFanout(memberID, content string) error
}

// DeliverFanout posts notification chat content to every IM room that includes memberID.
func DeliverFanout(memberID, content string, m RoomMessenger) error {
	memberID = strings.TrimSpace(memberID)
	if m == nil || memberID == "" {
		return nil
	}
	roomIDs := m.RoomIDsForMember(memberID)
	var lastErr error
	for _, rid := range roomIDs {
		if err := m.PostMessage(apitypes.CreateMessageRequest{
			RoomID:   rid,
			SenderID: memberID,
			Content:  content,
		}); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
