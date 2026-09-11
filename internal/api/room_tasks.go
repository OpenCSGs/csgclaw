package api

import (
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/roomtask"
	"csgclaw/internal/taskcore"
	"csgclaw/internal/taskmeta"
	"encoding/json"
	"net/http"
	"strings"
)

func (h *Handler) SetRoomTaskCore(core *taskcore.Service) {
	if h.participantBridge != nil {
		h.participantBridge.SetRoomContextProvider(h.roomExecutionContext)
	}
	if core == nil {
		h.roomTaskSvc = nil
		return
	}
	h.roomTaskSvc = roomtask.NewService(core, h.roomSchedulingContext, func(p roomtask.Projection) error {
		if p.Kind == "task_reported" || p.Kind == "task_planned" || p.Kind == "task_plan_updated" {
			_, err := h.im.DeliverMessage(im.DeliverMessageRequest{RoomID: p.RoomID, SenderID: h.resolveCSGClawParticipantUserID(p.SenderID), MessageID: p.ID, Content: p.Content, Metadata: taskmeta.Set(map[string]any{"csgclaw": map[string]any{"delivery_kind": p.Kind}}, p.TaskID, p.Attempt)})
			return err
		}
		_, err := h.im.DeliverEvent(im.DeliverEventRequest{RoomID: p.RoomID, SenderID: h.resolveCSGClawParticipantUserID(p.SenderID), MentionID: h.resolveCSGClawParticipantUserID(p.TargetID), MessageID: p.ID, Content: p.Content, Metadata: taskmeta.Set(nil, p.TaskID, p.Attempt), Event: &im.EventPayload{Key: p.Kind, ActorID: p.SenderID, Title: p.Title, TargetIDs: []string{p.TargetID}}})
		return err
	})
}

func (h *Handler) handleRoomTaskMessage(w http.ResponseWriter, r *http.Request) {
	if !h.requireRoomTasks(w, r) {
		return
	}
	var req apitypes.RoomTaskMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	roomID, id := pathValue(r, "id"), pathValue(r, "task_id")
	actor, ok := h.roomTaskRuntimeCaller(w, r)
	if !ok {
		return
	}
	roster, ok := h.roomSchedulingContext(roomID)
	if !ok {
		http.Error(w, "room unavailable", http.StatusConflict)
		return
	}
	room, _ := h.im.Room(roomID)
	member := false
	for _, m := range room.Members {
		member = member || actor.matches(m)
	}
	if !member {
		http.Error(w, "sender is not a room member", http.StatusForbidden)
		return
	}
	task, ok := h.roomTaskSvc.Get(roomID, id)
	if !ok {
		http.Error(w, "room task not found", http.StatusNotFound)
		return
	}
	if !actor.matches(roster.ManagerID) {
		if user, ok := h.im.User(h.resolveCSGClawParticipantUserID(actor.bridgeID)); ok && (strings.EqualFold(user.Role, "worker") || strings.EqualFold(user.Role, "agent")) {
			if !actor.matches(task.AssignedTo) {
				http.Error(w, "workers coordinate their own task with the manager", http.StatusForbidden)
				return
			}
		}
	}
	effectiveTarget := h.participantBridgeTargetForRoomMember(roster.ManagerID)
	if actor.matches(roster.ManagerID) {
		effectiveTarget = h.participantBridgeTargetForRoomMember(task.AssignedTo)
	}
	targetMember := false
	for _, m := range room.Members {
		targetMember = targetMember || effectiveTarget.matches(m)
	}
	if !targetMember {
		http.Error(w, "task counterpart is not a room member", http.StatusConflict)
		return
	}
	if _, ok := h.roomTaskSvc.ResolveMention(roomID, id, effectiveTarget.bridgeID); !ok {
		http.Error(w, "target is not the manager or this task's active assignee", http.StatusConflict)
		return
	}
	if strings.TrimSpace(req.Content) == "" || strings.TrimSpace(req.MessageID) == "" {
		http.Error(w, "content and stable message_id are required", http.StatusBadRequest)
		return
	}
	message, err := h.im.DeliverMessage(im.DeliverMessageRequest{RoomID: roomID, SenderID: h.resolveCSGClawParticipantUserID(actor.bridgeID), MentionID: h.resolveCSGClawParticipantUserID(effectiveTarget.bridgeID), Content: req.Content, MessageID: req.MessageID, Metadata: taskmeta.Set(nil, id, task.Attempt)})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, message)
}
func (h *Handler) roomSchedulingContext(roomID string) (roomtask.Roster, bool) {
	if h.im == nil {
		return roomtask.Roster{}, false
	}
	room, ok := h.im.Room(roomID)
	if !ok || room.IsDirect || !room.IsOnDemand() {
		return roomtask.Roster{}, false
	}
	r := roomtask.Roster{RoomID: room.ID}
	for _, id := range room.Members {
		u, found := h.im.User(id)
		if !found {
			continue
		}
		p := h.participantBridgeTargetForRoomMember(id)
		if p.matches(room.ManagerID) {
			if !strings.EqualFold(u.Role, "manager") {
				return roomtask.Roster{}, false
			}
			r.ManagerID = p.bridgeID
			continue
		}
		if strings.EqualFold(u.Role, "worker") || strings.EqualFold(u.Role, "agent") {
			r.WorkerIDs = append(r.WorkerIDs, p.bridgeID)
		}
	}
	return r, r.ManagerID != ""
}
func (h *Handler) requireRoomTasks(w http.ResponseWriter, r *http.Request) bool {
	if h.roomTaskSvc == nil {
		http.Error(w, "room task service is not configured", http.StatusServiceUnavailable)
		return false
	}
	if _, ok := h.roomSchedulingContext(pathValue(r, "id")); !ok {
		http.Error(w, "collaboration room or manager unavailable", http.StatusConflict)
		return false
	}
	return true
}
func (h *Handler) roomTasksResponse(room string) []apitypes.RoomTask {
	items := []apitypes.RoomTask{}
	for _, t := range h.roomTaskSvc.List(room) {
		items = append(items, h.apiRoomTask(t))
	}
	return items
}
func roomTaskError(w http.ResponseWriter, err error) { http.Error(w, err.Error(), http.StatusConflict) }
func (h *Handler) handleListRoomTasks(w http.ResponseWriter, r *http.Request) {
	if !h.requireRoomTasks(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, h.roomTasksResponse(pathValue(r, "id")))
}
func (h *Handler) handleCreateRoomTask(w http.ResponseWriter, r *http.Request) {
	if !h.requireRoomManager(w, r) {
		return
	}
	var req apitypes.CreateRoomTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	room, _ := h.im.Room(pathValue(r, "id"))
	roster, _ := h.roomSchedulingContext(room.ID)
	requester := ""
	for _, msg := range room.Messages {
		if msg.ID == strings.TrimSpace(req.SourceMessageID) {
			if u, ok := h.im.User(msg.SenderID); ok && !strings.EqualFold(u.Role, "worker") && !strings.EqualFold(u.Role, "agent") && !strings.EqualFold(u.Role, "manager") {
				requester = h.participantBridgeTargetForRoomMember(msg.SenderID).bridgeID
			}
			break
		}
	}
	if requester == "" {
		caller := h.participantBridgeTargetForRoomMember(r.Header.Get("X-CSGClaw-Caller-Agent"))
		if strings.TrimSpace(req.SourceMessageID) != "" && caller.matches(roster.ManagerID) {
			requester = roster.ManagerID
		}
	}
	if requester == "" {
		http.Error(w, "source_message_id must identify the requesting user message or a stable Manager request", http.StatusBadRequest)
		return
	}
	t, err := h.roomTaskSvc.Create(room.ID, strings.TrimSpace(req.SourceMessageID), requester, req.Title, req.Body)
	if err != nil {
		roomTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.apiRoomTask(t))
}
func (h *Handler) handlePlanRoomTask(w http.ResponseWriter, r *http.Request) {
	if !h.requireRoomManager(w, r) {
		return
	}
	var req apitypes.PlanRoomTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	items := make([]roomtask.PlanItem, 0, len(req.Tasks))
	for _, t := range req.Tasks {
		items = append(items, roomtask.PlanItem{IDRef: t.IDRef, Title: t.Title, Body: t.Body, AssignedTo: h.participantBridgeTargetForRoomMember(t.AssignedTo).bridgeID, DependsOnRefs: t.DependsOnRefs})
	}
	room, id := pathValue(r, "id"), pathValue(r, "task_id")
	var err error
	if req.Append {
		err = h.roomTaskSvc.AppendPlan(room, id, req.RequestID, req.Summary, items)
	} else {
		err = h.roomTaskSvc.Plan(room, id, req.Summary, items, req.AutoStart)
	}
	if err != nil {
		roomTaskError(w, err)
		return
	}
	t, _ := h.roomTaskSvc.Get(room, id)
	resp := apitypes.PlanRoomTaskResponse{Task: h.apiRoomTask(t), Started: t.Status == taskcore.StatusInProgress, CreatedTasks: []apitypes.RoomTask{}}
	for _, child := range h.roomTasksResponse(room) {
		if child.ParentID == id {
			resp.CreatedTasks = append(resp.CreatedTasks, child)
			if child.DispatchedAt != nil {
				resp.ScheduledTasks++
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
func (h *Handler) handleStartRoomTask(w http.ResponseWriter, r *http.Request) {
	if !h.requireRoomManager(w, r) {
		return
	}
	room, id := pathValue(r, "id"), pathValue(r, "task_id")
	if err := h.roomTaskSvc.Start(room, id); err != nil {
		roomTaskError(w, err)
		return
	}
	t, _ := h.roomTaskSvc.Get(room, id)
	writeJSON(w, http.StatusOK, apitypes.StartRoomTaskResponse{Task: h.apiRoomTask(t)})
}
func (h *Handler) handleClaimRoomTask(w http.ResponseWriter, r *http.Request) {
	if !h.requireRoomTasks(w, r) {
		return
	}
	var req apitypes.ClaimRoomTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.updateRoomTask(w, r, taskcore.StatusInProgress, "", "", req.Attempt)
}
func (h *Handler) handleUpdateRoomTask(w http.ResponseWriter, r *http.Request) {
	if !h.requireRoomTasks(w, r) {
		return
	}
	var req apitypes.UpdateRoomTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reason := req.Error
	if reason == "" {
		reason = req.Reason
	}
	h.updateRoomTask(w, r, req.Status, req.Result, reason, req.Attempt)
}
func (h *Handler) updateRoomTask(w http.ResponseWriter, r *http.Request, status, result, reason string, attempt int) {
	actor, ok := h.roomTaskRuntimeCaller(w, r)
	if !ok {
		return
	}
	t, err := h.roomTaskSvc.UpdateExecution(pathValue(r, "id"), pathValue(r, "task_id"), actor.bridgeID, status, result, reason, attempt)
	if err != nil {
		roomTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.apiRoomTask(t))
}

func (h *Handler) roomTaskRuntimeCaller(w http.ResponseWriter, r *http.Request) (participantBridgeTarget, bool) {
	caller := h.participantBridgeTargetForRoomMember(r.Header.Get("X-CSGClaw-Caller-Agent"))
	if strings.TrimSpace(caller.bridgeID) == "" {
		http.Error(w, "runtime caller identity is required", http.StatusForbidden)
		return participantBridgeTarget{}, false
	}
	return caller, true
}
func (h *Handler) handleReportRoomTask(w http.ResponseWriter, r *http.Request) {
	if !h.requireRoomManager(w, r) {
		return
	}
	var req struct {
		Outcome string `json:"outcome"`
		Summary string `json:"summary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	t, err := h.roomTaskSvc.Report(pathValue(r, "id"), pathValue(r, "task_id"), req.Outcome, req.Summary)
	if err != nil {
		roomTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.apiRoomTask(t))
}
func (h *Handler) handleRetryRoomDelivery(w http.ResponseWriter, r *http.Request) {
	if !h.requireRoomTasks(w, r) {
		return
	}
	if err := h.roomTaskSvc.RetryDelivery(pathValue(r, "id")); err != nil {
		roomTaskError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, h.roomTasksResponse(pathValue(r, "id")))
}

func (h *Handler) apiRoomTask(t taskcore.Task) apitypes.RoomTask {
	out := apitypes.RoomTask{Task: t}
	if user, ok := h.im.User(h.resolveCSGClawParticipantUserID(t.AssignedTo)); ok {
		out.AssignedToAgentName = user.Name
	}
	return out
}
func (h *Handler) requireRoomManager(w http.ResponseWriter, r *http.Request) bool {
	if !h.requireRoomTasks(w, r) {
		return false
	}
	roster, _ := h.roomSchedulingContext(pathValue(r, "id"))
	if caller := strings.TrimSpace(r.Header.Get("X-CSGClaw-Caller-Agent")); caller != "" && !h.participantBridgeTargetForRoomMember(caller).matches(roster.ManagerID) {
		http.Error(w, "only the room Manager coordinates parent tasks and dispatch/review", http.StatusForbidden)
		return false
	}
	return true
}
func (h *Handler) handleDispatchRoomTask(w http.ResponseWriter, r *http.Request) {
	if !h.requireRoomManager(w, r) {
		return
	}
	var req struct {
		AssignedTo string `json:"assigned_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	assignee := ""
	if req.AssignedTo != "" {
		assignee = h.participantBridgeTargetForRoomMember(req.AssignedTo).bridgeID
	}
	if err := h.roomTaskSvc.Dispatch(pathValue(r, "id"), pathValue(r, "task_id"), assignee); err != nil {
		roomTaskError(w, err)
		return
	}
	t, _ := h.roomTaskSvc.Get(pathValue(r, "id"), pathValue(r, "task_id"))
	writeJSON(w, http.StatusOK, h.apiRoomTask(t))
}
func (h *Handler) handleReviewRoomTask(w http.ResponseWriter, r *http.Request) {
	if !h.requireRoomManager(w, r) {
		return
	}
	var req apitypes.ReviewRoomTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.roomTaskSvc.Review(pathValue(r, "id"), pathValue(r, "task_id"), req.Attempt, req.Accept, req.Summary); err != nil {
		roomTaskError(w, err)
		return
	}
	t, _ := h.roomTaskSvc.Get(pathValue(r, "id"), pathValue(r, "task_id"))
	writeJSON(w, http.StatusOK, h.apiRoomTask(t))
}
