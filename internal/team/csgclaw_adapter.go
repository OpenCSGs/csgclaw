package team

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
)

const builtinChannelName = "csgclaw"

type CSGClawAdapter struct {
	im           *im.Service
	participants participantLookup
	locale       string
}

func NewCSGClawAdapter(imSvc *im.Service, participantSvc ...participantLookup) *CSGClawAdapter {
	var lookup participantLookup
	if len(participantSvc) > 0 {
		lookup = participantSvc[0]
	}
	return &CSGClawAdapter{
		im:           imSvc,
		participants: lookup,
		locale:       "zh",
	}
}

func (a *CSGClawAdapter) Channel() string {
	return builtinChannelName
}

func (a *CSGClawAdapter) ParticipantDisplayName(participantID string) string {
	participantID = strings.TrimSpace(participantID)
	if participantID == "" || a == nil || a.im == nil {
		return ""
	}
	userID := a.channelUserIDForParticipant(participantID)
	user, ok := a.im.User(userID)
	if !ok {
		return ""
	}
	if name := strings.TrimSpace(user.Name); name != "" {
		return name
	}
	return strings.TrimSpace(user.ID)
}

func (a *CSGClawAdapter) ParticipantIDForAgentID(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ""
	}
	if a != nil && a.participants != nil {
		for _, item := range a.participants.List(participant.ListOptions{Channel: participant.ChannelCSGClaw, AgentID: agentID}) {
			if participantID := strings.TrimSpace(item.ID); participantID != "" {
				return participantID
			}
		}
	}
	return defaultParticipantIDForAgentID(agentID)
}

func (a *CSGClawAdapter) EnsureRoom(_ context.Context, req EnsureRoomRequest) (RoomRef, error) {
	if a == nil || a.im == nil {
		return RoomRef{}, fmt.Errorf("im service is required")
	}

	leadParticipantID := strings.TrimSpace(req.LeadParticipantID)
	if leadParticipantID == "" {
		return RoomRef{}, fmt.Errorf("lead_participant_id is required")
	}
	leadUser, err := a.ensureParticipantUser(leadParticipantID, "manager")
	if err != nil {
		return RoomRef{}, err
	}
	memberUserIDs := make([]string, 0, len(req.MemberParticipantIDs))
	for _, memberID := range req.MemberParticipantIDs {
		user, err := a.ensureParticipantUser(memberID, "worker")
		if err != nil {
			return RoomRef{}, err
		}
		memberUserIDs = append(memberUserIDs, user.ID)
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
		CreatorID:   leadUser.ID,
		MemberIDs:   memberUserIDs,
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
	inviterID := strings.TrimSpace(req.InviterParticipantID)
	if inviterID == "" {
		return fmt.Errorf("inviter_participant_id is required")
	}
	inviter, err := a.ensureParticipantUser(inviterID, "manager")
	if err != nil {
		return err
	}

	userIDs := make([]string, 0, len(req.MemberParticipantIDs))
	for _, memberID := range req.MemberParticipantIDs {
		user, err := a.ensureParticipantUser(memberID, "worker")
		if err != nil {
			return err
		}
		userIDs = append(userIDs, user.ID)
	}
	if len(userIDs) == 0 {
		return nil
	}
	_, err = a.im.AddRoomMembers(im.AddRoomMembersRequest{
		RoomID:    roomID,
		InviterID: inviter.ID,
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
	senderParticipantID := strings.TrimSpace(req.SenderParticipantID)
	if senderParticipantID == "" {
		return MessageRef{}, fmt.Errorf("sender_participant_id is required")
	}
	if _, err := requireCanonicalParticipantID("sender_participant_id", senderParticipantID); err != nil {
		return MessageRef{}, err
	}
	resolvedSenderID := a.channelUserIDForParticipant(senderParticipantID)
	senderRole := "worker"
	if user, ok := a.im.User(resolvedSenderID); ok && strings.EqualFold(strings.TrimSpace(user.Role), "manager") {
		senderRole = "manager"
	}
	sender, ok := a.im.User(resolvedSenderID)
	if !ok {
		var err error
		sender, err = a.ensureParticipantUser(senderParticipantID, senderRole)
		if err != nil {
			return MessageRef{}, err
		}
	}
	senderID := sender.ID

	mentionID := a.channelUserIDForParticipant(strings.TrimSpace(req.MentionID))
	if strings.TrimSpace(req.Kind) == "team_event" {
		targetIDs := []string(nil)
		if mentionID != "" {
			targetIDs = []string{mentionID}
		}
		msg, err := a.im.DeliverEvent(im.DeliverEventRequest{
			RoomID:    roomID,
			SenderID:  senderID,
			MentionID: mentionID,
			Content:   strings.TrimSpace(req.Content),
			MessageID: strings.TrimSpace(req.IdempotencyKey),
			Event: &im.EventPayload{
				Key:       "team_event",
				ActorID:   senderID,
				Title:     strings.TrimSpace(req.Content),
				TargetIDs: targetIDs,
			},
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

func (a *CSGClawAdapter) ensureParticipantUser(participantID string, role string) (im.User, error) {
	var err error
	participantID, err = requireCanonicalParticipantID("participant_id", participantID)
	if err != nil {
		return im.User{}, err
	}
	if participantID == "" {
		return im.User{}, fmt.Errorf("participant id is required")
	}
	userID := a.channelUserIDForParticipant(participantID)
	if user, ok := a.lookupExistingParticipantUser(participantID); ok {
		return user, nil
	}
	user, _, err := a.im.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID:   userID,
		Name: participantDisplayName(participantID),
		Role: role,
	})
	if err != nil {
		return im.User{}, err
	}
	return user, nil
}

func (a *CSGClawAdapter) lookupExistingParticipantUser(participantID string) (im.User, bool) {
	if a == nil || a.im == nil {
		return im.User{}, false
	}
	channelUserID := a.channelUserIDForParticipant(participantID)
	candidates := []string{
		channelUserID,
		strings.TrimSpace(a.im.ResolveUserID(channelUserID)),
		strings.TrimSpace(a.im.ResolveUserID(participantID)),
	}
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if user, ok := a.im.User(candidate); ok {
			return user, true
		}
	}
	return im.User{}, false
}

func (a *CSGClawAdapter) channelUserIDForParticipant(participantID string) string {
	participantID = strings.TrimSpace(participantID)
	if participantID == "" {
		return ""
	}
	if a != nil && a.participants != nil {
		if item, ok := a.participants.Get(participant.ChannelCSGClaw, participantID); ok {
			if ref := strings.TrimSpace(item.ChannelUserRef); ref != "" {
				return ref
			}
		}
	}
	if participantID == agent.ManagerParticipantID {
		return im.ManagerUserID
	}
	if participantID == "pt-admin" || participantID == im.AdminUserID {
		return im.AdminUserID
	}
	if a != nil && a.im != nil {
		if user, ok := a.im.User(participantID); ok {
			return strings.TrimSpace(user.ID)
		}
		if resolved := strings.TrimSpace(a.im.ResolveUserID(participantID)); resolved != "" && resolved != participantID {
			return resolved
		}
	}
	return "user-" + strings.TrimPrefix(cleanParticipantID(participantID), "pt-")
}

func participantDisplayName(participantID string) string {
	name := strings.TrimSpace(strings.TrimPrefix(participantID, "bot-"))
	name = strings.TrimSpace(strings.TrimPrefix(name, "pt-"))
	name = strings.TrimSpace(strings.TrimPrefix(name, "u-"))
	name = strings.ReplaceAll(name, "_", "-")
	if name == "" {
		return "participant"
	}
	return name
}

func participantHandle(participantID string) string {
	handle := strings.ToLower(strings.TrimSpace(participantID))
	handle = strings.TrimPrefix(handle, "pt-")
	handle = strings.ReplaceAll(handle, "_", "-")
	if handle == "" {
		return strings.ToLower(strings.TrimSpace(participantID))
	}
	return handle
}
