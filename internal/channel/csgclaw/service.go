package csgclaw

import (
	"strings"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/slashcommand"
)

// Service adapts the local CSGClaw IM service to the channel-facing
// room/member/message boundary. The channel accepts common legacy bot and user
// IDs, while internal/im persists canonical user IDs for collaboration state.
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

func (s *Service) RemoveRoomMembers(req apitypes.AddRoomMembersRequest) (im.Room, error) {
	req.InviterID = botIDToUserID(req.InviterID)
	req.UserIDs = botIDsToUserIDs(req.UserIDs)
	return s.im.RemoveRoomMembers(req)
}

func (s *Service) ListMessages(roomID string) ([]im.Message, error) {
	return s.im.ListMessages(roomID)
}

func (s *Service) ListMessagesWithOptions(roomID string, opts im.ListMessagesOptions) ([]im.Message, error) {
	return s.im.ListMessagesWithOptions(roomID, opts)
}

func (s *Service) SendMessage(req apitypes.CreateMessageRequest) (im.Message, error) {
	content, err := normalizeSlashContent(req.Content)
	if err != nil {
		return im.Message{}, err
	}
	req.Content = content
	req.SenderID = botIDToUserID(req.SenderID)
	req.MentionID = botIDToUserID(req.MentionID)
	if req.RelatesTo != nil {
		req.RelatesTo.EventID = botIDToUserID(req.RelatesTo.EventID)
	}
	return s.im.CreateMessage(req)
}

func normalizeSlashContent(content string) (string, error) {
	normalized, ok, err := slashcommand.Normalize(content)
	if err != nil {
		return "", err
	}
	if ok {
		return normalized, nil
	}
	return content, nil
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
