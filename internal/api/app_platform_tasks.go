package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/taskcore"
	"csgclaw/internal/team"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func taskToolFields(names ...string) map[string]any {
	fields := map[string]any{}
	for _, name := range names {
		fields[name] = map[string]any{"type": "string"}
	}
	return fields
}

func taskToolText(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func taskToolPayload(args, fields map[string]any) map[string]any {
	out := map[string]any{}
	for name := range fields {
		if name == "room_id" || name == "task_id" || name == "team_id" || name == "approval_id" {
			continue
		}
		if value, ok := args[name]; ok {
			out[name] = value
		}
	}
	return out
}

func (h *Handler) registerAppPlatformTaskTools(agentID string) {
	h.registerAppRoomTaskTools(agentID)
	h.registerAppDirectTaskTools(agentID)
	h.registerAppTeamTaskTools(agentID)
}

func (h *Handler) registerAppRoomTaskTools(agentID string) {
	type operation struct {
		name, description, method string
		fields                    map[string]any
		required                  []string
		manager, readOnly         bool
		handler                   http.HandlerFunc
	}
	plan := taskToolFields("summary", "request_id")
	plan["append"] = map[string]any{"type": "boolean"}
	plan["auto_start"] = map[string]any{"type": "boolean"}
	planItem := taskToolFields("id_ref", "title", "body", "assigned_to")
	planItem["depends_on_refs"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	plan["tasks"] = map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": planItem, "required": []string{"id_ref", "title", "assigned_to"}, "additionalProperties": false}}
	claim := map[string]any{"attempt": map[string]any{"type": "integer", "minimum": 1}}
	update := taskToolFields("status", "result", "error", "reason")
	update["status"] = map[string]any{"type": "string", "enum": []string{"in_progress", "completed", "blocked", "failed"}}
	update["attempt"] = claim["attempt"]
	review := taskToolFields("summary")
	review["attempt"] = claim["attempt"]
	review["accept"] = map[string]any{"type": "boolean"}
	recoverFields := taskToolFields("assessment")
	recoverFields["attempt"] = claim["attempt"]
	report := taskToolFields("outcome", "summary")
	report["outcome"] = map[string]any{"type": "string", "enum": []string{"succeeded", "issues", "failed", "stopped"}}
	ops := []operation{
		{"room_tasks_list", "List tasks in a collaboration room you belong to.", http.MethodGet, nil, nil, false, true, h.handleListRoomTasks},
		{"room_task_get", "Read a task in a collaboration room you belong to.", http.MethodGet, nil, []string{"task_id"}, false, true, h.handleGetRoomTask},
		{"room_task_create", "Create a parent task from a user message or stable Manager request; room Manager only.", http.MethodPost, taskToolFields("source_message_id", "title", "body"), []string{"source_message_id", "title"}, true, false, h.handleCreateRoomTask},
		{"room_task_plan", "Plan or append assigned subtasks and their dependencies; room Manager only.", http.MethodPost, plan, []string{"task_id", "tasks"}, true, false, h.handlePlanRoomTask},
		{"room_task_start", "Activate a planned parent task; dispatch ready child tasks explicitly using room_task_dispatch. Room Manager only.", http.MethodPost, nil, []string{"task_id"}, true, false, h.handleStartRoomTask},
		{"room_task_claim", "Claim your assigned task for its current execution attempt.", http.MethodPost, claim, []string{"task_id", "attempt"}, false, false, h.handleClaimRoomTask},
		{"room_task_update", "Report progress or outcome for your assigned task and execution attempt.", http.MethodPatch, update, []string{"task_id", "attempt", "status"}, false, false, h.handleUpdateRoomTask},
		{"room_task_dispatch", "Dispatch a task to its assigned worker or select an eligible room worker; room Manager only.", http.MethodPost, taskToolFields("assigned_to"), []string{"task_id"}, true, false, h.handleDispatchRoomTask},
		{"room_task_review", "Review a worker's completed attempt; room Manager only.", http.MethodPost, review, []string{"task_id", "attempt", "accept", "summary"}, true, false, h.handleReviewRoomTask},
		{"room_task_report", "Publish the final outcome and summary of a parent task; room Manager only.", http.MethodPost, report, []string{"task_id", "outcome", "summary"}, true, false, h.handleReportRoomTask},
		{"room_task_stop", "Stop a parent task through the execution controller; room Manager only.", http.MethodPost, nil, []string{"task_id"}, true, false, h.handleRoomTaskStop},
		{"room_task_recover", "Resolve a stopped or uncertain execution after checking the specified attempt; room Manager only.", http.MethodPost, recoverFields, []string{"task_id", "attempt", "assessment"}, true, false, h.handleRoomTaskRecover},
		{"room_task_message", "Send a task-scoped message to the Manager or current assignee using a stable message ID.", http.MethodPost, taskToolFields("content", "message_id"), []string{"task_id", "content", "message_id"}, false, false, h.handleRoomTaskMessage},
		{"room_task_events", "Read the event history for a task in your collaboration room.", http.MethodGet, nil, []string{"task_id"}, false, true, h.handleRoomTaskEvents},
		{"room_tasks_retry_delivery", "Retry pending task message delivery in your collaboration room; room Manager only.", http.MethodPost, nil, nil, true, false, h.handleRetryRoomDelivery},
	}
	for _, op := range ops {
		fields := op.fields
		if fields == nil {
			fields = map[string]any{}
		}
		fields["room_id"] = map[string]any{"type": "string"}
		for _, key := range op.required {
			if _, ok := fields[key]; !ok {
				fields[key] = map[string]any{"type": "string"}
			}
		}
		required := append([]string{"room_id"}, op.required...)
		h.addAppPlatformTool(agentID, op.name, op.description, fields, required, op.readOnly, func(ctx context.Context, _ *mcp.CallToolRequest, args map[string]any) (any, error) {
			roomID := taskToolText(args, "room_id")
			if err := h.requirePlatformRoom(agentID, roomID); err != nil {
				return nil, err
			}
			if op.manager {
				if err := h.requireAppTaskRoomManager(agentID, roomID); err != nil {
					return nil, err
				}
			}
			return h.invokeAppPlatformHandler(ctx, agentID, op.method, map[string]string{"id": roomID, "task_id": taskToolText(args, "task_id")}, nil, taskToolPayload(args, fields), op.handler)
		})
	}
}

func (h *Handler) requireAppTaskRoomManager(agentID, roomID string) error {
	roster, ok := h.roomSchedulingContext(roomID)
	if !ok || !h.participantBridgeTargetForRoomMember(h.agentPlatformParticipant(agentID)).matches(roster.ManagerID) {
		return fmt.Errorf("only this room's Manager may coordinate tasks")
	}
	return h.requirePlatformManager(agentID)
}

func (h *Handler) appDirectTask(agentID, id string, execution bool) (taskcore.Task, error) {
	if h.agentTaskSvc == nil {
		return taskcore.Task{}, fmt.Errorf("Agent task service is unavailable")
	}
	actor := h.participantBridgeTargetForRoomMember(h.agentPlatformParticipant(agentID))
	for _, task := range h.agentTaskSvc.List() {
		if task.ID != id {
			continue
		}
		if actor.matches(task.AssignedTo) || (!execution && actor.matches(task.CreatedBy)) {
			return task, nil
		}
		return taskcore.Task{}, fmt.Errorf("task is not assigned to this Agent")
	}
	return taskcore.Task{}, fmt.Errorf("Agent task not found")
}

func (h *Handler) registerAppDirectTaskTools(agentID string) {
	h.addAppPlatformTool(agentID, "agent_tasks_list", "List tasks assigned to you or created by you.", map[string]any{}, nil, true, func(ctx context.Context, _ *mcp.CallToolRequest, _ map[string]any) (any, error) {
		if h.agentTaskSvc == nil {
			return nil, fmt.Errorf("Agent task service is unavailable")
		}
		items := []apitypes.TeamTask{}
		actor := h.participantBridgeTargetForRoomMember(h.agentPlatformParticipant(agentID))
		for _, task := range h.agentTaskSvc.List() {
			if actor.matches(task.AssignedTo) || actor.matches(task.CreatedBy) {
				items = append(items, apiCoreTask(task, h.newTeamIdentityPresenter()))
			}
		}
		return items, nil
	})
	h.addAppPlatformTool(agentID, "agent_task_get", "Read a task assigned to you or created by you.", taskToolFields("task_id"), []string{"task_id"}, true, func(ctx context.Context, _ *mcp.CallToolRequest, args map[string]any) (any, error) {
		task, err := h.appDirectTask(agentID, taskToolText(args, "task_id"), false)
		if err != nil {
			return nil, err
		}
		return apiCoreTask(task, h.newTeamIdentityPresenter()), nil
	})
	create := taskToolFields("agent_id", "title", "body")
	h.addAppPlatformTool(agentID, "agent_task_create", "Assign a task to a worker Agent and send its task notification; Manager only.", create, []string{"agent_id", "title"}, false, func(ctx context.Context, _ *mcp.CallToolRequest, args map[string]any) (any, error) {
		if err := h.requirePlatformManager(agentID); err != nil {
			return nil, err
		}
		target, ok := h.svc.Agent(taskToolText(args, "agent_id"))
		if !ok || target.Role != agent.RoleWorker {
			return nil, fmt.Errorf("target must be a worker Agent")
		}
		payload := taskToolPayload(args, create)
		payload["created_by"] = h.agentPlatformParticipant(agentID)
		return h.invokeAppPlatformHandler(ctx, agentID, http.MethodPost, nil, nil, payload, h.handleCreateAgentTask)
	})
	for _, operation := range []struct {
		name, description, method string
		fields                    map[string]any
		required                  []string
		handler                   http.HandlerFunc
	}{
		{"agent_task_claim", "Claim a task assigned to you.", http.MethodPost, taskToolFields("task_id"), []string{"task_id"}, h.handleClaimAgentTask},
		{"agent_task_update", "Complete, fail or block a task assigned to you.", http.MethodPatch, taskToolFields("task_id", "status", "result", "error", "reason"), []string{"task_id", "status"}, h.handleUpdateAgentTask},
	} {
		h.addAppPlatformTool(agentID, operation.name, operation.description, operation.fields, operation.required, false, func(ctx context.Context, _ *mcp.CallToolRequest, args map[string]any) (any, error) {
			id := taskToolText(args, "task_id")
			if _, err := h.appDirectTask(agentID, id, true); err != nil {
				return nil, err
			}
			payload := taskToolPayload(args, operation.fields)
			payload["participant_id"] = h.agentPlatformParticipant(agentID)
			payload["actor_id"] = h.agentPlatformParticipant(agentID)
			return h.invokeAppPlatformHandler(ctx, agentID, operation.method, map[string]string{"task_id": id}, nil, payload, operation.handler)
		})
	}
}

func (h *Handler) appTaskTeam(agentID, teamID string, leadOnly bool) (team.TeamMeta, error) {
	if h.teamSvc == nil {
		return team.TeamMeta{}, fmt.Errorf("team service is unavailable")
	}
	meta, ok := h.teamSvc.GetTeam(teamID)
	if !ok {
		return team.TeamMeta{}, fmt.Errorf("team not found")
	}
	caller := agent.CanonicalID(agentID)
	if agent.CanonicalID(meta.LeadAgentID) == caller {
		return meta, nil
	}
	if !leadOnly {
		for _, id := range meta.MemberAgentIDs {
			if agent.CanonicalID(id) == caller {
				return meta, nil
			}
		}
	}
	return team.TeamMeta{}, fmt.Errorf("Agent is not authorized for this team")
}

func (h *Handler) registerAppTeamTaskTools(agentID string) {
	h.addAppPlatformTool(agentID, "teams_list", "List teams you lead or belong to.", map[string]any{}, nil, true, func(context.Context, *mcp.CallToolRequest, map[string]any) (any, error) {
		if h.teamSvc == nil {
			return []apitypes.Team{}, nil
		}
		items := []apitypes.Team{}
		for _, meta := range h.teamSvc.ListTeams() {
			if _, err := h.appTaskTeam(agentID, meta.ID, false); err == nil {
				items = append(items, apiTeamWithPresenter(meta, h.newTeamIdentityPresenter()))
			}
		}
		return items, nil
	})
	type operation struct {
		name, description, method string
		fields                    map[string]any
		required                  []string
		lead, readOnly            bool
		handler                   http.HandlerFunc
	}
	batch := taskToolFields("execution_channel")
	batchFields := taskToolFields("id_ref", "parent_id", "parent_ref", "title", "body", "assign_to", "deadline_at", "timeout_at")
	batchFields["depends_on_refs"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	batchFields["priority"] = map[string]any{"type": "integer"}
	batch["tasks"] = map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": batchFields, "required": []string{"title"}, "additionalProperties": false}}
	plan := map[string]any{"auto_start": map[string]any{"type": "boolean"}}
	ops := []operation{
		{"team_get", "Read a team you lead or belong to.", http.MethodGet, nil, nil, false, true, h.handleGetTeam},
		{"team_tasks_list", "List tasks in a team you belong to.", http.MethodGet, nil, nil, false, true, h.handleListTeamTasks},
		{"team_tasks_create", "Create a batch of tasks with execution rooms and notifications; team lead only.", http.MethodPost, batch, []string{"tasks"}, true, false, h.handleCreateTeamTasksBatch},
		{"team_task_plan", "Generate a task plan and optionally start it; team lead only.", http.MethodPost, plan, []string{"task_id"}, true, false, h.handlePlanTeamTask},
		{"team_task_start", "Start a planned task and dispatch its execution rooms; team lead only.", http.MethodPost, nil, []string{"task_id"}, true, false, h.handleStartTeamTask},
		{"team_task_assign", "Assign an eligible team member to a task; team lead only.", http.MethodPost, taskToolFields("assigned_to"), []string{"task_id", "assigned_to"}, true, false, h.handleAssignTeamTask},
		{"team_task_claim", "Claim a team task as the current Agent.", http.MethodPost, nil, []string{"task_id"}, false, false, h.handleClaimTeamTask},
		{"team_task_claim_next", "Claim the next eligible task for the current Agent in this team.", http.MethodPost, nil, nil, false, false, h.handleClaimNextTask},
		{"team_task_update", "Report progress or an outcome for your own team task.", http.MethodPatch, taskToolFields("status", "result", "error", "reason"), []string{"task_id", "status"}, false, false, h.handleUpdateTeamTask},
		{"team_approvals_list", "List approvals in your team.", http.MethodGet, nil, nil, false, true, h.handleListTeamApprovals},
		{"team_approval_request", "Request approval for your team task.", http.MethodPost, taskToolFields("task_id", "approver_id", "kind", "summary", "payload"), []string{"task_id", "kind", "summary"}, false, false, h.handleCreateTeamApproval},
		{"team_approval_resolve", "Resolve an approval addressed to the current Agent.", http.MethodPost, taskToolFields("status", "reason"), []string{"approval_id", "status"}, false, false, h.handleResolveTeamApproval},
		{"team_task_events", "Read your team's task event history.", http.MethodGet, nil, nil, false, true, h.handleListTeamEvents},
	}
	for _, op := range ops {
		fields := op.fields
		if fields == nil {
			fields = map[string]any{}
		}
		fields["team_id"] = map[string]any{"type": "string"}
		for _, key := range op.required {
			if _, ok := fields[key]; !ok {
				fields[key] = map[string]any{"type": "string"}
			}
		}
		h.addAppPlatformTool(agentID, op.name, op.description, fields, append([]string{"team_id"}, op.required...), op.readOnly, func(ctx context.Context, _ *mcp.CallToolRequest, args map[string]any) (any, error) {
			if op.lead {
				if err := h.requirePlatformManager(agentID); err != nil {
					return nil, err
				}
			}
			teamID := taskToolText(args, "team_id")
			meta, err := h.appTaskTeam(agentID, teamID, op.lead)
			if err != nil {
				return nil, err
			}
			actor := h.agentPlatformParticipant(agentID)
			target := h.participantBridgeTargetForRoomMember(actor)
			id := taskToolText(args, "task_id")
			if op.name == "team_task_update" || op.name == "team_approval_request" {
				task, found := h.teamSvc.GetTask(teamID, id)
				if !found {
					return nil, fmt.Errorf("team task not found")
				}
				if !target.matches(task.AssignedTo) && agent.CanonicalID(meta.LeadAgentID) != agent.CanonicalID(agentID) {
					return nil, fmt.Errorf("task is not assigned to this Agent")
				}
			}
			if op.name == "team_approval_resolve" {
				if err := h.authorizeAppApprovalResolution(agentID, teamID, taskToolText(args, "approval_id")); err != nil {
					return nil, err
				}
			}
			payload := taskToolPayload(args, fields)
			payload["actor_id"] = actor
			payload["participant_id"] = actor
			payload["created_by"] = actor
			payload["requested_by"] = actor
			if op.name == "team_task_assign" {
				payload["participant_id"] = taskToolText(args, "assigned_to")
			}
			if op.name == "team_approval_request" {
				approver, err := h.appApprovalApprover(agentID, teamID, id, taskToolText(args, "approver_id"))
				if err != nil {
					return nil, err
				}
				payload["approver_id"] = approver
				payload["task_id"] = id
			}
			if op.name == "team_approval_resolve" {
				payload["approver_id"] = actor
			}
			return h.invokeAppPlatformHandler(ctx, agentID, op.method, map[string]string{"team_id": teamID, "task_id": id, "approval_id": taskToolText(args, "approval_id")}, nil, payload, op.handler)
		})
	}
}
