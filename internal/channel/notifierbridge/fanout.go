// Package notifierbridge wires notifier runtime fan-out to IM and SSE-style publishers,
// similar in role to codexbridge for the Codex channel.
package notifierbridge

import (
	"errors"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/runtime/notifier"
)

// IMCore is the subset of IM services used for notifier fan-out.
type IMCore interface {
	RoomIDsForMember(memberID string) []string
	CreateMessage(req apitypes.CreateMessageRequest) (apitypes.Message, error)
	User(userID string) (apitypes.User, bool)
}

// Fanout adapts IM + publish hooks to notifier.IMFanoutBridge (mirrors codexbridge-style wiring).
type Fanout struct {
	IM      IMCore
	Publish func(roomID, senderID string, msg apitypes.Message, sender apitypes.User)
}

// NewFanout returns a notifier.IMFanoutBridge backed by IM and an optional publish callback.
func NewFanout(im IMCore, publish func(roomID, senderID string, msg apitypes.Message, sender apitypes.User)) notifier.IMFanoutBridge {
	return Fanout{IM: im, Publish: publish}
}

func (f Fanout) RoomIDsForMember(memberID string) []string {
	if f.IM == nil {
		return nil
	}
	return f.IM.RoomIDsForMember(memberID)
}

func (f Fanout) CreateMessage(req apitypes.CreateMessageRequest) (apitypes.Message, error) {
	if f.IM == nil {
		return apitypes.Message{}, errors.New("im not configured")
	}
	return f.IM.CreateMessage(req)
}

func (f Fanout) User(userID string) (apitypes.User, bool) {
	if f.IM == nil {
		return apitypes.User{}, false
	}
	return f.IM.User(userID)
}

func (f Fanout) PublishMessageCreated(roomID, senderID string, msg apitypes.Message, sender apitypes.User) {
	if f.Publish == nil {
		return
	}
	f.Publish(roomID, senderID, msg, sender)
}
