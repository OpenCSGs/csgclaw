package team

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/internal/agent"
)

type ExecutionRoomDirectory interface {
	ListMembers(roomID string) ([]MemberProfile, error)
}

func (s *Service) EnsureTaskExecutionRoom(ctx context.Context, adapter TeamChannelAdapter, directory ExecutionRoomDirectory, meta TeamMeta, parent TeamTask) (string, error) {
	if ExecutionRoomBound(parent, meta) {
		roomID := strings.TrimSpace(parent.RoomID)
		if err := s.syncTaskExecutionRoomMembers(ctx, adapter, directory, meta, parent, roomID); err != nil {
			return "", err
		}
		return roomID, nil
	}

	leadParticipantID := participantIDForAgentID(adapter, meta.LeadAgentID)
	if leadParticipantID == "" {
		return "", fmt.Errorf("channel %q participant not found for lead agent %q", adapterChannel(adapter), meta.LeadAgentID)
	}
	memberParticipantIDs, err := s.taskExecutionRoomMemberParticipantIDs(adapter, directory, meta, parent)
	if err != nil {
		return "", err
	}
	roomRef, err := adapter.EnsureRoom(ctx, EnsureRoomRequest{
		Title:                TaskExecutionRoomTitle(parent),
		LeadParticipantID:    leadParticipantID,
		CreatorParticipantID: leadParticipantID,
		MemberParticipantIDs: memberParticipantIDs,
	})
	if err != nil {
		return "", err
	}
	return roomRef.RoomID, nil
}

func (s *Service) syncTaskExecutionRoomMembers(ctx context.Context, adapter TeamChannelAdapter, directory ExecutionRoomDirectory, meta TeamMeta, parent TeamTask, roomID string) error {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" || adapter == nil {
		return nil
	}
	memberParticipantIDs, err := s.taskExecutionRoomMemberParticipantIDs(adapter, directory, meta, parent)
	if err != nil {
		return err
	}
	if directory != nil {
		if members, err := directory.ListMembers(roomID); err == nil {
			existing := make(map[string]struct{}, len(members))
			for _, member := range members {
				existing[cleanParticipantID(member.ID)] = struct{}{}
			}
			filtered := make([]string, 0, len(memberParticipantIDs))
			for _, memberID := range memberParticipantIDs {
				if _, ok := existing[cleanParticipantID(memberID)]; ok {
					continue
				}
				filtered = append(filtered, memberID)
			}
			memberParticipantIDs = filtered
		}
	}
	if len(memberParticipantIDs) == 0 {
		return nil
	}
	leadParticipantID := participantIDForAgentID(adapter, meta.LeadAgentID)
	if leadParticipantID == "" {
		return fmt.Errorf("channel %q participant not found for lead agent %q", adapterChannel(adapter), meta.LeadAgentID)
	}
	return adapter.AddMembers(ctx, AddMembersRequest{
		Room: RoomRef{
			Channel: adapter.Channel(),
			RoomID:  roomID,
		},
		InviterParticipantID: leadParticipantID,
		MemberParticipantIDs: memberParticipantIDs,
	})
}

func (s *Service) taskExecutionRoomMemberParticipantIDs(adapter TeamChannelAdapter, directory ExecutionRoomDirectory, meta TeamMeta, parent TeamTask) ([]string, error) {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	leadParticipantID := participantIDForAgentID(adapter, meta.LeadAgentID)
	add := func(id string) {
		id, err := requireCanonicalParticipantID("participant_id", id)
		if err != nil || id == "" || ParticipantIDsMatch(id, leadParticipantID) {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, agentID := range meta.MemberAgentIDs {
		participantID := participantIDForAgentID(adapter, agentID)
		if participantID == "" {
			return nil, fmt.Errorf("channel %q participant not found for team member agent %q", adapterChannel(adapter), agentID)
		}
		add(participantID)
	}
	for _, id := range s.parentTaskAssigneeParticipantIDs(directory, meta.ID, parent.ID) {
		add(id)
	}
	return out, nil
}

func (s *Service) parentTaskAssigneeParticipantIDs(directory ExecutionRoomDirectory, teamID, parentID string) []string {
	teamID = strings.TrimSpace(teamID)
	parentID = strings.TrimSpace(parentID)
	if teamID == "" || parentID == "" || s == nil {
		return nil
	}
	out := make([]string, 0)
	for _, task := range s.ListTasks(teamID) {
		if strings.TrimSpace(task.ParentID) != parentID {
			continue
		}
		assignee := strings.TrimSpace(task.AssignedTo)
		if assignee == "" {
			continue
		}
		assignee, err := requireCanonicalParticipantID("assigned_to", assignee)
		if err != nil || assignee == "" {
			continue
		}
		out = append(out, assignee)
	}
	return out
}

func adapterChannel(adapter TeamChannelAdapter) string {
	if adapter == nil {
		return ""
	}
	return strings.TrimSpace(adapter.Channel())
}

func isExecutionRoomAgentMember(member MemberProfile, leadParticipantID string) bool {
	participantID := strings.TrimSpace(member.ID)
	if participantID == "" || ParticipantIDsMatch(participantID, leadParticipantID) {
		return false
	}
	role := strings.ToLower(strings.TrimSpace(member.Role))
	switch role {
	case agent.RoleWorker, agent.RoleAgent:
		return true
	}
	return false
}
