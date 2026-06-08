package team

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/internal/im"
)

const builtinChannelName = "csgclaw"

type CSGClawAdapter struct {
	im     *im.Service
	locale string
}

func NewCSGClawAdapter(imSvc *im.Service) *CSGClawAdapter {
	return &CSGClawAdapter{
		im:     imSvc,
		locale: "zh",
	}
}

func (a *CSGClawAdapter) Channel() string {
	return builtinChannelName
}

func (a *CSGClawAdapter) EnsureRoom(_ context.Context, req EnsureRoomRequest) (RoomRef, error) {
	if a == nil || a.im == nil {
		return RoomRef{}, fmt.Errorf("im service is required")
	}

	leadBotID := strings.TrimSpace(req.LeadBotID)
	if leadBotID == "" {
		return RoomRef{}, fmt.Errorf("lead_bot_id is required")
	}
	if _, err := a.ensureBotUser(leadBotID, "manager"); err != nil {
		return RoomRef{}, err
	}
	for _, memberID := range req.MemberBotIDs {
		if _, err := a.ensureBotUser(memberID, "worker"); err != nil {
			return RoomRef{}, err
		}
	}

	roomID := strings.TrimSpace(req.RoomID)
	if roomID != "" {
		if _, ok := a.im.Room(roomID); !ok {
			return RoomRef{}, fmt.Errorf("room not found")
		}
		return RoomRef{Channel: a.Channel(), RoomID: roomID}, nil
	}

	room, err := a.im.CreateRoom(im.CreateRoomRequest{
		Title:       firstNonEmpty(strings.TrimSpace(req.Title), "team"),
		CreatorID:   leadBotID,
		MemberIDs:   cloneStrings(req.MemberBotIDs),
		Description: "",
		Locale:      a.locale,
	})
	if err != nil {
		return RoomRef{}, err
	}
	return RoomRef{Channel: a.Channel(), RoomID: room.ID}, nil
}

func (a *CSGClawAdapter) AddMembers(_ context.Context, req AddMembersRequest) error {
	if a == nil || a.im == nil {
		return fmt.Errorf("im service is required")
	}
	roomID := strings.TrimSpace(req.Room.RoomID)
	if roomID == "" {
		return fmt.Errorf("room_id is required")
	}
	inviterID := strings.TrimSpace(req.InviterBotID)
	if inviterID == "" {
		return fmt.Errorf("inviter_bot_id is required")
	}
	if _, err := a.ensureBotUser(inviterID, "manager"); err != nil {
		return err
	}

	userIDs := make([]string, 0, len(req.MemberBotIDs))
	for _, memberID := range req.MemberBotIDs {
		user, err := a.ensureBotUser(memberID, "worker")
		if err != nil {
			return err
		}
		userIDs = append(userIDs, user.ID)
	}
	if len(userIDs) == 0 {
		return nil
	}
	_, err := a.im.AddRoomMembers(im.AddRoomMembersRequest{
		RoomID:    roomID,
		InviterID: inviterID,
		UserIDs:   userIDs,
		Locale:    a.locale,
	})
	return err
}

func (a *CSGClawAdapter) SendMessage(_ context.Context, req SendMessageRequest) (MessageRef, error) {
	if a == nil || a.im == nil {
		return MessageRef{}, fmt.Errorf("im service is required")
	}
	roomID := strings.TrimSpace(req.Room.RoomID)
	if roomID == "" {
		return MessageRef{}, fmt.Errorf("room_id is required")
	}
	senderID := strings.TrimSpace(req.SenderBotID)
	if senderID == "" {
		return MessageRef{}, fmt.Errorf("sender_bot_id is required")
	}
	senderID = a.im.ResolveUserID(senderID)
	senderRole := "worker"
	if user, ok := a.im.User(senderID); ok && strings.EqualFold(strings.TrimSpace(user.Role), "manager") {
		senderRole = "manager"
	}
	sender, err := a.ensureBotUser(senderID, senderRole)
	if err != nil {
		return MessageRef{}, err
	}
	senderID = sender.ID

	mentionID := a.im.ResolveUserID(strings.TrimSpace(req.MentionID))

	msg, err := a.im.DeliverMessage(im.DeliverMessageRequest{
		RoomID:    roomID,
		SenderID:  senderID,
		MentionID: mentionID,
		Content:   strings.TrimSpace(req.Content),
		MessageID: strings.TrimSpace(req.IdempotencyKey),
	})
	if err != nil {
		return MessageRef{}, err
	}
	return MessageRef{
		Channel:   a.Channel(),
		RoomID:    roomID,
		MessageID: msg.ID,
	}, nil
}

func (a *CSGClawAdapter) ensureBotUser(botID string, role string) (im.User, error) {
	var err error
	botID, err = requireCanonicalParticipantID("bot_id", botID)
	if err != nil {
		return im.User{}, err
	}
	if botID == "" {
		return im.User{}, fmt.Errorf("participant id is required")
	}
	user, _, err := a.im.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID:     botID,
		Name:   botDisplayName(botID),
		Handle: botHandle(botID),
		Role:   role,
	})
	if err != nil {
		return im.User{}, err
	}
	return user, nil
}

func botDisplayName(botID string) string {
	name := strings.TrimSpace(strings.TrimPrefix(botID, "bot-"))
	name = strings.ReplaceAll(name, "_", "-")
	if name == "" {
		return "bot"
	}
	return name
}

func botHandle(botID string) string {
	handle := strings.ToLower(strings.TrimSpace(botID))
	handle = strings.ReplaceAll(handle, "_", "-")
	if handle == "" {
		return strings.ToLower(strings.TrimSpace(botID))
	}
	return handle
}
