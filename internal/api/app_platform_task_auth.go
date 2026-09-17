package api

import (
	"net/http"
	"strings"

	agent "csgclaw/internal/agentengine/agents"
)

// Runtime CLI task notifications remain valid with Agent credentials. Each
// admitted route resolves task/team ownership and replaces identity arguments.
func (h *Handler) authorizeAgentTaskRoute(r *http.Request, agentID string, parts []string) bool {
	if r.Method != http.MethodGet && h.appReadOnlyAgent(agentID) {
		return false
	}
	actor := h.agentPlatformParticipant(agentID)
	target := h.participantBridgeTargetForRoomMember(actor)
	if parts[0] == "agent-tasks" {
		if len(parts) == 1 && r.Method == http.MethodGet {
			return true
		}
		payload, ok := platformRequestBody(r)
		if !ok {
			return false
		}
		if len(parts) == 1 && r.Method == http.MethodPost {
			if !h.agentPlatformManager(agentID) {
				return false
			}
			worker, found := h.svc.Agent(platformText(payload, "agent_id"))
			if !found || worker.Role != agent.RoleWorker {
				return false
			}
			if !h.bindTaskActor(payload, actor, "created_by") {
				return false
			}
			setPlatformRequestBody(r, payload)
			return true
		}
		claim := len(parts) == 3 && parts[2] == "claim" && r.Method == http.MethodPost
		update := len(parts) == 2 && r.Method == http.MethodPatch
		if !claim && !update {
			return false
		}
		if _, err := h.appDirectTask(agentID, parts[1], true); err != nil {
			return false
		}
		if !h.bindTaskActor(payload, actor, "participant_id", "actor_id") {
			return false
		}
		setPlatformRequestBody(r, payload)
		return true
	}
	if parts[0] != "teams" {
		return false
	}
	if len(parts) == 1 {
		return r.Method == http.MethodGet
	}
	// The old global claim-next API must still name a team: the underlying
	// cross-team scan has no Agent membership filter.
	if len(parts) == 3 && parts[1] == "tasks" && parts[2] == "claim-next" && r.Method == http.MethodPost {
		payload, ok := platformRequestBody(r)
		if !ok {
			return false
		}
		teamID := platformText(payload, "team_id")
		if teamID == "" {
			return false
		}
		if _, err := h.appTaskTeam(agentID, teamID, false); err != nil {
			return false
		}
		if !h.bindTaskActor(payload, actor, "participant_id") {
			return false
		}
		setPlatformRequestBody(r, payload)
		return true
	}
	meta, err := h.appTaskTeam(agentID, parts[1], false)
	if err != nil {
		return false
	}
	if r.Method == http.MethodGet {
		return len(parts) == 2 || (len(parts) == 3 && (parts[2] == "tasks" || parts[2] == "approvals" || parts[2] == "events"))
	}
	if len(parts) < 3 {
		return false
	}
	payload, ok := platformRequestBody(r)
	if !ok {
		return false
	}
	manager := h.agentPlatformManager(agentID) && agent.CanonicalID(meta.LeadAgentID) == agent.CanonicalID(agentID)
	if parts[2] == "tasks" {
		if len(parts) == 4 && parts[3] == "batch" && r.Method == http.MethodPost {
			if !manager || !h.bindTaskActor(payload, actor, "created_by") {
				return false
			}
			items, _ := payload["tasks"].([]any)
			for _, item := range items {
				task, _ := item.(map[string]any)
				if assignee := platformText(task, "assign_to"); assignee != "" && !h.appTeamAssignee(meta.ID, assignee) {
					return false
				}
			}
			setPlatformRequestBody(r, payload)
			return true
		}
		if len(parts) == 4 && parts[3] == "claim-next" && r.Method == http.MethodPost {
			if !h.bindTaskActor(payload, actor, "participant_id") {
				return false
			}
			setPlatformRequestBody(r, payload)
			return true
		}
		if len(parts) < 4 {
			return false
		}
		task, found := h.teamSvc.GetTask(meta.ID, parts[3])
		if !found {
			return false
		}
		if len(parts) == 5 && r.Method == http.MethodPost {
			switch parts[4] {
			case "plan", "start":
				if !manager || !h.bindTaskActor(payload, actor, "actor_id") {
					return false
				}
			case "assign":
				if !manager || !h.appTeamAssignee(meta.ID, platformText(payload, "participant_id")) {
					return false
				}
				if !h.bindTaskActor(payload, actor, "actor_id") {
					return false
				}
			case "claim":
				if !target.matches(task.AssignedTo) || !h.bindTaskActor(payload, actor, "participant_id") {
					return false
				}
			default:
				return false
			}
		} else if len(parts) == 4 && r.Method == http.MethodPatch {
			if !target.matches(task.AssignedTo) || !h.bindTaskActor(payload, actor, "actor_id") {
				return false
			}
		} else {
			return false
		}
		setPlatformRequestBody(r, payload)
		return true
	}
	if parts[2] == "approvals" && r.Method == http.MethodPost {
		if len(parts) == 3 {
			approver, err := h.appApprovalApprover(agentID, meta.ID, platformText(payload, "task_id"), platformText(payload, "approver_id"))
			if err != nil {
				return false
			}
			payload["approver_id"] = approver
			if !h.bindTaskActor(payload, actor, "requested_by") {
				return false
			}
		} else if len(parts) == 5 && parts[4] == "resolve" {
			if err := h.authorizeAppApprovalResolution(agentID, meta.ID, parts[3]); err != nil {
				return false
			}
			if !h.bindTaskActor(payload, actor, "approver_id") {
				return false
			}
		} else {
			return false
		}
		setPlatformRequestBody(r, payload)
		return true
	}
	return false
}

func (h *Handler) bindTaskActor(payload map[string]any, actor string, fields ...string) bool {
	target := h.participantBridgeTargetForRoomMember(actor)
	for _, field := range fields {
		if raw, ok := payload[field]; ok {
			value, isString := raw.(string)
			if !isString || (strings.TrimSpace(value) != "" && !target.matches(value)) {
				return false
			}
		}
		payload[field] = actor
	}
	return true
}

func (h *Handler) appTeamAssignee(teamID, participantID string) bool {
	if strings.TrimSpace(participantID) == "" {
		return false
	}
	id := h.runtimeAgentIDForBridgeID(participantID)
	_, err := h.appTaskTeam(id, teamID, false)
	return err == nil
}
