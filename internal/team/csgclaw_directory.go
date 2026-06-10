package team

import (
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
)

type participantLookup interface {
	Get(channel, id string) (apitypes.Participant, bool)
	List(opts participant.ListOptions) []apitypes.Participant
}

type CSGClawTeamDirectory struct {
	im           *im.Service
	agents       *agent.Service
	participants participantLookup
}

func NewCSGClawTeamDirectory(imSvc *im.Service, agentSvc *agent.Service, participantSvc ...participantLookup) *CSGClawTeamDirectory {
	var lookup participantLookup
	if len(participantSvc) > 0 {
		lookup = participantSvc[0]
	}
	return &CSGClawTeamDirectory{
		im:           imSvc,
		agents:       agentSvc,
		participants: lookup,
	}
}

func (d *CSGClawTeamDirectory) TeamRoomMemberIDs(roomID string) []string {
	if d == nil || d.im == nil {
		return nil
	}
	room, ok := d.im.Room(roomID)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(room.Members))
	for _, memberID := range room.Members {
		if participantID := d.participantIDForChannelUser(memberID); participantID != "" {
			out = append(out, participantID)
		}
	}
	return out
}

func (d *CSGClawTeamDirectory) RoomTitle(roomID string) (string, bool) {
	if d == nil || d.im == nil {
		return "", false
	}
	room, ok := d.im.Room(roomID)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(room.Title), true
}

func (d *CSGClawTeamDirectory) UserProfile(id string) (MemberProfile, bool) {
	if d == nil || d.im == nil {
		return MemberProfile{}, false
	}
	participantID := strings.TrimSpace(id)
	user, ok := d.im.User(d.channelUserIDForParticipant(participantID))
	if !ok {
		return MemberProfile{}, false
	}
	profile := memberProfileFromUser(user)
	profile.ID = firstNonEmpty(participantID, d.participantIDForChannelUser(user.ID))
	return profile, true
}

func (d *CSGClawTeamDirectory) AgentProfile(id string) (MemberProfile, bool) {
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

func (d *CSGClawTeamDirectory) ListMembers(roomID string) ([]MemberProfile, error) {
	if d == nil || d.im == nil {
		return nil, nil
	}
	members, err := d.im.ListMembers(roomID)
	if err != nil {
		return nil, err
	}
	out := make([]MemberProfile, 0, len(members))
	for _, member := range members {
		profile := memberProfileFromUser(member)
		profile.ID = d.participantIDForChannelUser(member.ID)
		out = append(out, profile)
	}
	return out, nil
}

func (d *CSGClawTeamDirectory) ResolveUserID(participantID string) string {
	participantID = strings.TrimSpace(participantID)
	if d == nil || d.im == nil {
		return participantID
	}
	return d.channelUserIDForParticipant(participantID)
}

func (d *CSGClawTeamDirectory) ResolveAgentID(participantID string) string {
	participantID = strings.TrimSpace(participantID)
	if participantID == "" {
		return ""
	}
	if d != nil && d.participants != nil {
		if item, ok := d.participants.Get(participant.ChannelCSGClaw, participantID); ok {
			if agentID := strings.TrimSpace(item.AgentID); agentID != "" {
				return agentID
			}
		}
		for _, item := range d.participants.List(participant.ListOptions{Channel: participant.ChannelCSGClaw, Type: participant.TypeAgent}) {
			if participantIdentityMatches(item, participantID) {
				if agentID := strings.TrimSpace(item.AgentID); agentID != "" {
					return agentID
				}
			}
		}
	}
	if participantID == agent.ManagerParticipantID {
		return agent.ManagerUserID
	}
	return "u-" + participantID
}

func (d *CSGClawTeamDirectory) ParticipantIDForAgentID(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ""
	}
	if d != nil && d.participants != nil {
		for _, item := range d.participants.List(participant.ListOptions{Channel: participant.ChannelCSGClaw, AgentID: agentID}) {
			if participantID := strings.TrimSpace(item.ID); participantID != "" {
				return participantID
			}
		}
	}
	return defaultParticipantIDForAgentID(agentID)
}

func (d *CSGClawTeamDirectory) channelUserIDForParticipant(participantID string) string {
	participantID = strings.TrimSpace(participantID)
	if participantID == "" {
		return ""
	}
	if d != nil && d.participants != nil {
		if item, ok := d.participants.Get(participant.ChannelCSGClaw, participantID); ok {
			if ref := strings.TrimSpace(item.ChannelUserRef); ref != "" {
				return ref
			}
		}
	}
	if participantID == agent.ManagerParticipantID || participantID == im.AdminUserID {
		return participantID
	}
	if d != nil && d.im != nil {
		if user, ok := d.im.User(participantID); ok {
			return strings.TrimSpace(user.ID)
		}
		if resolved := strings.TrimSpace(d.im.ResolveUserID(participantID)); resolved != "" && resolved != participantID {
			return resolved
		}
	}
	return "u-" + participantID
}

func (d *CSGClawTeamDirectory) participantIDForChannelUser(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ""
	}
	if d != nil && d.participants != nil {
		for _, item := range d.participants.List(participant.ListOptions{Channel: participant.ChannelCSGClaw}) {
			if strings.TrimSpace(item.ChannelUserRef) == userID {
				return strings.TrimSpace(item.ID)
			}
		}
	}
	if userID == agent.ManagerParticipantID || userID == im.AdminUserID {
		return userID
	}
	if strings.HasPrefix(userID, "u-") && len(userID) > len("u-") {
		return strings.TrimPrefix(userID, "u-")
	}
	return userID
}

func memberProfileFromUser(user im.User) MemberProfile {
	return MemberProfile{
		ID:   strings.TrimSpace(user.ID),
		Name: strings.TrimSpace(user.Name),
		Role: strings.TrimSpace(user.Role),
	}
}

func participantIdentityMatches(item apitypes.Participant, id string) bool {
	id = strings.TrimSpace(id)
	return id != "" && (strings.TrimSpace(item.ID) == id ||
		strings.TrimSpace(item.ChannelUserRef) == id ||
		strings.TrimSpace(item.AgentID) == id)
}
