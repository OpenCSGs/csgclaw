package api

import (
	"fmt"
	"strings"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/participant"
)

// App approval creation and resolution share the same policy across MCP and
// scoped runtime REST. Human management APIs retain their existing workflow.
func (h *Handler) appApprovalApprover(agentID, teamID, taskID, requested string) (string, error) {
	meta, err := h.appTaskTeam(agentID, teamID, false)
	if err != nil {
		return "", err
	}
	actor := h.participantBridgeTargetForRoomMember(h.agentPlatformParticipant(agentID))
	task, ok := h.teamSvc.GetTask(teamID, taskID)
	if !ok {
		return "", fmt.Errorf("team task not found")
	}
	if !actor.matches(task.AssignedTo) && agent.CanonicalID(meta.LeadAgentID) != agent.CanonicalID(agentID) {
		return "", fmt.Errorf("task is not assigned to this Agent")
	}
	requested = strings.TrimSpace(requested)
	if !h.agentPlatformManager(agentID) {
		lead, ok := h.svc.Agent(meta.LeadAgentID)
		if !ok || lead.Role != agent.RoleManager {
			return "", fmt.Errorf("team has no Manager approver")
		}
		approver := h.participantBridgeTargetForRoomMember(h.agentPlatformParticipant(lead.ID))
		if actor.matches(approver.bridgeID) || (requested != "" && !approver.matches(requested)) {
			return "", fmt.Errorf("Workers must request approval from their team Manager")
		}
		return approver.bridgeID, nil
	}
	if requested == "" || actor.matches(requested) {
		return "", fmt.Errorf("Manager approval requests must explicitly select an existing human")
	}
	if h.participant != nil {
		for _, person := range h.participant.List(participant.ListOptions{}) {
			if person.Type != participant.TypeHuman || person.AgentID != "" {
				continue
			}
			target := h.participantBridgeTargetForRoomMember(person.ID)
			if target.matches(requested) {
				return target.bridgeID, nil
			}
		}
	}
	if h.im != nil {
		for _, person := range h.im.ListUsers() {
			switch strings.ToLower(strings.TrimSpace(person.Role)) {
			case "admin", "human", "user":
			default:
				continue
			}
			target := h.participantBridgeTargetForRoomMember(person.ID)
			if target.matches(requested) {
				return target.bridgeID, nil
			}
		}
	}
	return "", fmt.Errorf("approval recipient must be an existing human or administrator")
}

func (h *Handler) authorizeAppApprovalResolution(agentID, teamID, approvalID string) error {
	if err := h.requirePlatformManager(agentID); err != nil {
		return err
	}
	if _, err := h.appTaskTeam(agentID, teamID, false); err != nil {
		return err
	}
	actor := h.participantBridgeTargetForRoomMember(h.agentPlatformParticipant(agentID))
	for _, approval := range h.teamSvc.ListApprovals(teamID) {
		if approval.ID != approvalID {
			continue
		}
		if actor.matches(approval.RequestedBy) {
			return fmt.Errorf("an approval requester cannot approve their own request")
		}
		if !actor.matches(approval.ApproverID) {
			return fmt.Errorf("approval is not addressed to this Manager")
		}
		return nil
	}
	return fmt.Errorf("approval not found")
}
