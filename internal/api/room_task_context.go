package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/roomtask"
	"csgclaw/internal/taskcore"
)

// roomExecutionContext is read after the per-conversation ingress queue admits
// a turn. Policy, room scope, task state, and the current event are separated so
// the channel can project only changed sections into continuing conversations.
func (h *Handler) roomExecutionContext(request roomtask.TurnContextRequest) (roomtask.PrivateTurnContext, error) {
	roomID, participantID := request.RoomID, request.ParticipantID
	sourceID, taskID := request.SourceID, request.TaskID
	room, ok := h.im.Room(roomID)
	if !ok || !room.IsOnDemand() {
		return roomtask.PrivateTurnContext{}, nil
	}
	roster, ok := h.roomSchedulingContext(roomID)
	if !ok || h.roomTaskSvc == nil {
		return roomtask.PrivateTurnContext{}, fmt.Errorf("collaboration context unavailable")
	}
	actor := h.participantBridgeTargetForRoomMember(participantID)
	manager := actor.matches(roster.ManagerID)
	role := roomtask.TurnRoleWorker
	scope := map[string]any{"room_id": roomID, "room_type": apitypes.RoomTypeOnDemand, "manager_id": roster.ManagerID,
		"participant_id": actor.bridgeID, "role": "worker"}
	snapshot := map[string]any{}
	turn := map[string]any{"source_message_id": sourceID, "related_task_id": taskID}
	if manager {
		role = roomtask.TurnRoleManager
		scope = h.roomContextFacts(roster)
		scope["role"], scope["participant_id"] = "manager", actor.bridgeID
		scope["room_type"] = apitypes.RoomTypeOnDemand
		for _, message := range room.Messages {
			if message.ID == sourceID {
				turn["source_actor_id"] = h.participantBridgeTargetForRoomMember(message.SenderID).bridgeID
				break
			}
		}
		all := h.roomTaskSvc.List(roomID)
		selected := map[string]bool{}
		for _, task := range all {
			if task.ParentID == "" && (!roomtask.Terminal(task.Status) || task.ReportStatus != "delivered") {
				selected[task.ID] = true
			}
		}
		byID := map[string]taskcore.Task{}
		for _, task := range all {
			byID[task.ID] = task
		}
		for id := taskID; id != ""; id = byID[id].ParentID {
			selected[id] = true
		}
		tasks := []map[string]any{}
		for _, task := range all {
			if !selected[task.ID] && !selected[task.ParentID] {
				continue
			}
			item := map[string]any{"id": task.ID, "parent_id": task.ParentID, "title": task.Title,
				"status": task.Status, "assigned_to": task.AssignedTo, "attempt": task.Attempt, "depends_on": task.DependsOn,
				"waiting_on_task_id": task.WaitingOnTaskID, "recovery_required": task.RecoveryRequired,
				"report_status": task.ReportStatus, "goal_outcome": task.GoalOutcome, "updated_at": task.UpdatedAt, "created_at": task.CreatedAt, "source_message_id": task.SourceMessageID}
			// No shell transcripts or full implementation details in routine wakes.
			if task.ID == taskID || task.Status == taskcore.StatusReview {
				item["result_excerpt"] = contextExcerpt(task.Result, 2000)
				item["error_excerpt"] = contextExcerpt(task.Error, 1000)
				item["review_excerpt"] = contextExcerpt(task.Review, 1000)
			}
			tasks = append(tasks, item)
		}
		snapshot["tasks"], snapshot["has_tasks"] = tasks, len(tasks) > 0
		snapshot["completed_history_omitted"] = true
	} else {
		task, found := h.roomTaskSvc.Get(roomID, taskID)
		if !found || task.ParentID == "" || !actor.matches(task.AssignedTo) {
			return roomtask.PrivateTurnContext{}, fmt.Errorf("worker task context does not match the assignment")
		}
		member := false
		for _, id := range roster.WorkerIDs {
			member = member || actor.matches(id)
		}
		if !member {
			return roomtask.PrivateTurnContext{}, fmt.Errorf("worker is no longer a room member")
		}
		if attempt := h.roomTaskAttemptForSource(roomID, sourceID); attempt > 0 && attempt != task.Attempt {
			return roomtask.PrivateTurnContext{}, fmt.Errorf("task execution attempt is stale")
		}
		snapshot["task"] = task
		predecessors := []map[string]any{}
		for _, id := range task.DependsOn {
			if dependency, found := h.roomTaskSvc.Get(roomID, id); found {
				predecessors = append(predecessors, map[string]any{"id": id, "title": dependency.Title, "status": dependency.Status, "result": dependency.Result})
			}
		}
		snapshot["predecessors"] = predecessors
	}
	if attempt := h.roomTaskAttemptForSource(roomID, sourceID); attempt > 0 {
		turn["task_attempt"] = attempt
	}
	policyID := roomtask.TurnPolicyID(role)
	if policyID == "" {
		return roomtask.PrivateTurnContext{}, fmt.Errorf("collaboration instructions unavailable")
	}
	scopeJSON, err := json.Marshal(scope)
	if err != nil {
		return roomtask.PrivateTurnContext{}, err
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return roomtask.PrivateTurnContext{}, err
	}
	turnJSON, err := json.Marshal(turn)
	if err != nil {
		return roomtask.PrivateTurnContext{}, err
	}
	return roomtask.PrivateTurnContext{
		Role: role, PolicyID: policyID,
		ScopeJSON: string(scopeJSON), SnapshotJSON: string(snapshotJSON), TurnJSON: string(turnJSON),
	}, nil
}

func contextExcerpt(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "… [truncated; task get returns full details]"
}

func (h *Handler) handleGetRoomTask(w http.ResponseWriter, r *http.Request) {
	if !h.requireRoomTasks(w, r) {
		return
	}
	task, ok := h.roomTaskSvc.Get(pathValue(r, "id"), pathValue(r, "task_id"))
	if !ok {
		http.Error(w, "room task not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, h.apiRoomTask(task))
}
