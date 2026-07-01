package team

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/internal/agent"
	channelfeishu "csgclaw/internal/channel/feishu"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
)

type FeishuAdapter struct {
	feishu       *channelfeishu.Service
	participants participantLookup
}

func NewFeishuAdapter(feishuSvc *channelfeishu.Service, participantSvc participantLookup) *FeishuAdapter {
	return &FeishuAdapter{
		feishu:       feishuSvc,
		participants: participantSvc,
	}
}

func (a *FeishuAdapter) Channel() string {
	return participant.ChannelFeishu
}

func (a *FeishuAdapter) ParticipantDisplayName(participantID string) string {
	participantID = strings.TrimSpace(participantID)
	if participantID == "" || a == nil || a.participants == nil {
		return ""
	}
	item, ok := a.participants.Get(participant.ChannelFeishu, participantID)
	if !ok {
		return ""
	}
	if name := strings.TrimSpace(item.Name); name != "" {
		return name
	}
	if ref := strings.TrimSpace(item.ChannelUserRef); ref != "" {
		return ref
	}
	return strings.TrimSpace(item.ID)
}

func (a *FeishuAdapter) ParticipantIDForAgentID(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || a == nil || a.participants == nil {
		return ""
	}
	for _, item := range a.participants.List(participant.ListOptions{Channel: participant.ChannelFeishu, Type: participant.TypeAgent, AgentID: agentID}) {
		if participantID := strings.TrimSpace(item.ID); participantID != "" {
			return participantID
		}
	}
	return ""
}

func (a *FeishuAdapter) EnsureRoom(_ context.Context, req EnsureRoomRequest) (RoomRef, error) {
	if a == nil || a.feishu == nil {
		return RoomRef{}, fmt.Errorf("feishu service is required")
	}
	leadParticipantID := strings.TrimSpace(req.LeadParticipantID)
	if leadParticipantID == "" {
		return RoomRef{}, fmt.Errorf("lead_participant_id is required")
	}
	if roomID := strings.TrimSpace(req.RoomID); roomID != "" {
		return RoomRef{Channel: a.Channel(), RoomID: roomID}, nil
	}
	room, err := a.feishu.CreateRoom(im.CreateRoomRequest{
		Title:     firstNonEmpty(strings.TrimSpace(req.Title), "team"),
		CreatorID: leadParticipantID,
		MemberIDs: uniqueTrimmedStrings(req.MemberParticipantIDs),
	})
	if err != nil {
		return RoomRef{}, err
	}
	return RoomRef{Channel: a.Channel(), RoomID: room.ID}, nil
}

func (a *FeishuAdapter) AddMembers(_ context.Context, req AddMembersRequest) error {
	if a == nil || a.feishu == nil {
		return fmt.Errorf("feishu service is required")
	}
	roomID := strings.TrimSpace(req.Room.RoomID)
	if roomID == "" {
		return fmt.Errorf("room_id is required")
	}
	memberIDs := uniqueTrimmedStrings(req.MemberParticipantIDs)
	if len(memberIDs) == 0 {
		return nil
	}
	_, err := a.feishu.AddRoomMembers(im.AddRoomMembersRequest{
		RoomID:    roomID,
		InviterID: strings.TrimSpace(req.InviterParticipantID),
		UserIDs:   memberIDs,
	})
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "no new users to invite") {
		return nil
	}
	return err
}

func (a *FeishuAdapter) SendMessage(_ context.Context, req SendMessageRequest) (MessageRef, error) {
	if a == nil || a.feishu == nil {
		return MessageRef{}, fmt.Errorf("feishu service is required")
	}
	roomID := strings.TrimSpace(req.Room.RoomID)
	if roomID == "" {
		return MessageRef{}, fmt.Errorf("room_id is required")
	}
	senderID := strings.TrimSpace(req.SenderParticipantID)
	if senderID == "" {
		return MessageRef{}, fmt.Errorf("sender_participant_id is required")
	}
	message, err := a.feishu.SendMessage(im.CreateMessageRequest{
		RoomID:    roomID,
		SenderID:  senderID,
		MentionID: strings.TrimSpace(req.MentionID),
		Content:   strings.TrimSpace(req.Content),
	})
	if err != nil {
		return MessageRef{}, err
	}
	return MessageRef{Channel: a.Channel(), RoomID: roomID, MessageID: message.ID}, nil
}

type FeishuTeamDirectory struct {
	feishu       *channelfeishu.Service
	agents       *agent.Service
	participants participantLookup
}

func NewFeishuTeamDirectory(feishuSvc *channelfeishu.Service, agentSvc *agent.Service, participantSvc participantLookup) *FeishuTeamDirectory {
	return &FeishuTeamDirectory{
		feishu:       feishuSvc,
		agents:       agentSvc,
		participants: participantSvc,
	}
}

func (d *FeishuTeamDirectory) RoomTitle(roomID string) (string, bool) {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" || d == nil || d.feishu == nil {
		return "", false
	}
	rooms, err := d.feishu.ListRooms()
	if err != nil {
		return "", false
	}
	for _, room := range rooms {
		if strings.TrimSpace(room.ID) == roomID {
			return strings.TrimSpace(room.Title), true
		}
	}
	return "", false
}

func (d *FeishuTeamDirectory) ListMembers(roomID string) ([]MemberProfile, error) {
	if d == nil || d.feishu == nil {
		return nil, nil
	}
	members, err := d.feishu.ListRoomMembers(roomID)
	if err != nil {
		return nil, err
	}
	out := make([]MemberProfile, 0, len(members))
	for _, member := range members {
		out = append(out, memberProfileFromUser(member))
	}
	return out, nil
}

func (d *FeishuTeamDirectory) UserProfile(id string) (MemberProfile, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return MemberProfile{}, false
	}
	if d != nil && d.participants != nil {
		if item, ok := d.participants.Get(participant.ChannelFeishu, id); ok {
			return MemberProfile{
				ID:   strings.TrimSpace(item.ID),
				Name: strings.TrimSpace(item.Name),
				Role: participantRole(item),
			}, true
		}
	}
	return MemberProfile{ID: id, Name: id}, true
}

func (d *FeishuTeamDirectory) AgentProfile(id string) (MemberProfile, bool) {
	if d == nil || d.agents == nil {
		return MemberProfile{}, false
	}
	got, ok := d.agents.Agent(id)
	if !ok {
		return MemberProfile{}, false
	}
	return MemberProfile{
		ID:          strings.TrimSpace(got.ID),
		Name:        strings.TrimSpace(got.Name),
		Role:        strings.TrimSpace(got.Role),
		Description: strings.TrimSpace(got.Description),
	}, true
}

func (d *FeishuTeamDirectory) ResolveAgentID(participantID string) string {
	participantID = strings.TrimSpace(participantID)
	if participantID == "" || d == nil || d.participants == nil {
		return ""
	}
	if item, ok := d.participants.Get(participant.ChannelFeishu, participantID); ok {
		return strings.TrimSpace(item.AgentID)
	}
	return ""
}

func (d *FeishuTeamDirectory) ParticipantIDForAgentID(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || d == nil || d.participants == nil {
		return ""
	}
	for _, item := range d.participants.List(participant.ListOptions{Channel: participant.ChannelFeishu, Type: participant.TypeAgent, AgentID: agentID}) {
		if participantID := strings.TrimSpace(item.ID); participantID != "" {
			return participantID
		}
	}
	return ""
}

func participantRole(item participant.Participant) string {
	if strings.TrimSpace(item.Type) == participant.TypeAgent {
		return agent.RoleWorker
	}
	return strings.TrimSpace(item.Type)
}
