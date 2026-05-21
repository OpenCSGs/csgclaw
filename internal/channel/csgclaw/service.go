package csgclaw

import (
	"strings"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
)

// Service adapts the local CSGClaw IM service to the channel-facing
// room/member/message boundary. Today CSGClaw bot IDs and IM user IDs are the
// same value, but keeping the conversion here prevents bot identity semantics
// from leaking into internal/im.
type Service struct {
	im *im.Service
}

func NewService(imSvc *im.Service) *Service {
	if imSvc == nil {
		return nil
	}
	return &Service{im: imSvc}
}

func (s *Service) ListRooms() []im.Room {
	return s.im.ListRooms()
}

func (s *Service) CreateRoom(req apitypes.CreateRoomRequest) (im.Room, error) {
	req.CreatorID = botIDToUserID(req.CreatorID)
	req.MemberIDs = botIDsToUserIDs(req.MemberIDs)
	return s.im.CreateRoom(req)
}

func (s *Service) DeleteRoom(roomID string) error {
	return s.im.DeleteRoom(roomID)
}

func (s *Service) ListRoomMembers(roomID string) ([]im.User, error) {
	return s.im.ListMembers(roomID)
}

func (s *Service) AddRoomMembers(req apitypes.AddRoomMembersRequest) (im.Room, error) {
	req.InviterID = botIDToUserID(req.InviterID)
	req.UserIDs = botIDsToUserIDs(req.UserIDs)
	return s.im.AddRoomMembers(req)
}

func (s *Service) ListMessages(roomID string) ([]im.Message, error) {
	return s.im.ListMessages(roomID)
}

func (s *Service) ListMessagesWithOptions(roomID string, opts im.ListMessagesOptions) ([]im.Message, error) {
	return s.im.ListMessagesWithOptions(roomID, opts)
}

func (s *Service) SendMessage(req apitypes.CreateMessageRequest) (im.Message, error) {
	req.SenderID = botIDToUserID(req.SenderID)
	req.MentionID = botIDToUserID(req.MentionID)
	if req.RelatesTo != nil {
		req.RelatesTo.EventID = botIDToUserID(req.RelatesTo.EventID)
	}
	return s.im.CreateMessage(req)
}

func botIDToUserID(id string) string {
	return strings.TrimSpace(id)
}

func botIDsToUserIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if userID := botIDToUserID(id); userID != "" {
			out = append(out, userID)
		}
	}
	return out
}
