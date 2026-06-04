package api

import (
	"context"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/im"
	"csgclaw/internal/team"
)

func (h *Handler) ensureTaskExecutionRoom(ctx context.Context, adapter team.TeamChannelAdapter, meta team.TeamMeta, parent team.TeamTask) (string, error) {
	if team.ExecutionRoomBound(parent, meta) {
		roomID := strings.TrimSpace(parent.RoomID)
		if err := h.syncTaskExecutionRoomMembers(ctx, adapter, meta, parent, roomID); err != nil {
			return "", err
		}
		return roomID, nil
	}

	memberBotIDs := h.taskExecutionRoomMemberBotIDs(meta, parent)

	roomRef, err := adapter.EnsureRoom(ctx, team.EnsureRoomRequest{
		Title:        team.TaskExecutionRoomTitle(parent),
		LeadBotID:    meta.LeadBotID,
		CreatorBotID: meta.LeadBotID,
		MemberBotIDs: memberBotIDs,
	})
	if err != nil {
		return "", err
	}
	return roomRef.RoomID, nil
}

func (h *Handler) syncTaskExecutionRoomMembers(ctx context.Context, adapter team.TeamChannelAdapter, meta team.TeamMeta, parent team.TeamTask, roomID string) error {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" || adapter == nil {
		return nil
	}
	memberBotIDs := h.taskExecutionRoomMemberBotIDs(meta, parent)
	if h != nil && h.im != nil {
		if members, err := h.im.ListMembers(roomID); err == nil {
			existing := make(map[string]struct{}, len(members))
			for _, member := range members {
				existing[strings.TrimSpace(member.ID)] = struct{}{}
			}
			filtered := make([]string, 0, len(memberBotIDs))
			for _, memberID := range memberBotIDs {
				resolvedID := h.im.ResolveUserID(memberID)
				if _, ok := existing[strings.TrimSpace(resolvedID)]; ok {
					continue
				}
				filtered = append(filtered, memberID)
			}
			memberBotIDs = filtered
		}
	}
	if len(memberBotIDs) == 0 {
		return nil
	}
	return adapter.AddMembers(ctx, team.AddMembersRequest{
		Room: team.RoomRef{
			Channel: firstNonEmpty(meta.Channel, adapter.Channel()),
			RoomID:  roomID,
		},
		InviterBotID: meta.LeadBotID,
		MemberBotIDs: memberBotIDs,
	})
}

func (h *Handler) taskExecutionRoomMemberBotIDs(meta team.TeamMeta, parent team.TeamTask) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || id == meta.LeadBotID {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range h.teamRoomMemberBotIDs(meta.RoomID, meta.LeadBotID) {
		add(id)
	}
	for _, id := range h.parentTaskAssigneeBotIDs(meta.ID, parent.ID) {
		add(id)
	}
	return out
}

func (h *Handler) parentTaskAssigneeBotIDs(teamID, parentID string) []string {
	teamID = strings.TrimSpace(teamID)
	parentID = strings.TrimSpace(parentID)
	if teamID == "" || parentID == "" || h.teamSvc == nil {
		return nil
	}
	out := make([]string, 0)
	for _, task := range h.teamSvc.ListTasks(teamID) {
		if strings.TrimSpace(task.ParentID) != parentID {
			continue
		}
		assignee := strings.TrimSpace(task.AssignedTo)
		if assignee == "" {
			continue
		}
		if h.im != nil {
			assignee = h.im.ResolveUserID(assignee)
		}
		out = append(out, assignee)
	}
	return out
}

func (h *Handler) teamRoomMemberBotIDs(teamRoomID, leadBotID string) []string {
	teamRoomID = strings.TrimSpace(teamRoomID)
	if teamRoomID == "" || h.im == nil {
		return nil
	}
	members, err := h.im.ListMembers(teamRoomID)
	if err != nil {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(members))
	for _, member := range members {
		if !isExecutionRoomAgentMember(member, leadBotID) {
			continue
		}
		userID := strings.TrimSpace(member.ID)
		if _, dup := seen[userID]; dup {
			continue
		}
		seen[userID] = struct{}{}
		out = append(out, userID)
	}
	return out
}

func isExecutionRoomAgentMember(member im.User, leadBotID string) bool {
	userID := strings.TrimSpace(member.ID)
	if userID == "" || userID == leadBotID {
		return false
	}
	role := strings.ToLower(strings.TrimSpace(member.Role))
	switch role {
	case agent.RoleWorker, agent.RoleAgent:
		return true
	}
	return false
}
