package team

import (
	"context"
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

	memberParticipantIDs := s.taskExecutionRoomMemberParticipantIDs(directory, meta, parent)
	leadParticipantID := participantIDForAgentID(adapter, meta.LeadAgentID)
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
	memberParticipantIDs := s.taskExecutionRoomMemberParticipantIDs(directory, meta, parent)
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
	return adapter.AddMembers(ctx, AddMembersRequest{
		Room: RoomRef{
			Channel: firstNonEmpty(meta.Channel, adapter.Channel()),
			RoomID:  roomID,
		},
		InviterParticipantID: leadParticipantID,
		MemberParticipantIDs: memberParticipantIDs,
	})
}

func (s *Service) taskExecutionRoomMemberParticipantIDs(directory ExecutionRoomDirectory, meta TeamMeta, parent TeamTask) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	leadParticipantID := participantIDForAgentID(directory, meta.LeadAgentID)
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
	for _, id := range teamRoomMemberParticipantIDs(directory, meta.RoomID, leadParticipantID) {
		add(id)
	}
	for _, id := range s.parentTaskAssigneeParticipantIDs(directory, meta.ID, parent.ID) {
		add(id)
	}
	return out
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

func teamRoomMemberParticipantIDs(directory ExecutionRoomDirectory, teamRoomID, leadParticipantID string) []string {
	teamRoomID = strings.TrimSpace(teamRoomID)
	if teamRoomID == "" || directory == nil {
		return nil
	}
	members, err := directory.ListMembers(teamRoomID)
	if err != nil {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(members))
	for _, member := range members {
		member.ID = cleanParticipantID(member.ID)
		if member.ID == "" || !isExecutionRoomAgentMember(member, leadParticipantID) {
			continue
		}
		participantID := strings.TrimSpace(member.ID)
		if _, dup := seen[participantID]; dup {
			continue
		}
		seen[participantID] = struct{}{}
		out = append(out, participantID)
	}
	return out
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
