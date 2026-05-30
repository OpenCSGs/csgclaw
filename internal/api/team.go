package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/team"
)

func (h *Handler) listTeams(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	teams := svc.ListTeams()
	resp := make([]apitypes.Team, 0, len(teams))
	for _, item := range teams {
		resp = append(resp, apiTeam(item))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) createTeam(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, adapter, ok := h.requireTeamComponents(w)
	if !ok {
		return
	}

	var req apitypes.CreateTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	channel := strings.TrimSpace(req.Channel)
	if channel == "" {
		channel = "csgclaw"
	}
	if !strings.EqualFold(channel, adapter.Channel()) {
		http.Error(w, fmt.Sprintf("unsupported team channel %q", channel), http.StatusBadRequest)
		return
	}

	roomRef, err := adapter.EnsureRoom(r.Context(), team.EnsureRoomRequest{
		RoomID:       strings.TrimSpace(req.RoomID),
		Title:        strings.TrimSpace(req.Title),
		LeadBotID:    strings.TrimSpace(req.LeadBotID),
		CreatorBotID: strings.TrimSpace(req.LeadBotID),
		MemberBotIDs: uniqueStrings(req.MemberBotIDs),
	})
	if err != nil {
		writeTeamError(w, err)
		return
	}
	if strings.TrimSpace(req.RoomID) != "" && len(req.MemberBotIDs) > 0 {
		if err := adapter.AddMembers(r.Context(), team.AddMembersRequest{
			Room:         roomRef,
			InviterBotID: strings.TrimSpace(req.LeadBotID),
			MemberBotIDs: uniqueStrings(req.MemberBotIDs),
		}); err != nil {
			writeTeamError(w, err)
			return
		}
	}

	created, err := svc.CreateTeam(team.CreateTeamInput{
		ID:        roomRef.RoomID,
		RoomID:    roomRef.RoomID,
		Channel:   channel,
		Title:     strings.TrimSpace(req.Title),
		LeadBotID: strings.TrimSpace(req.LeadBotID),
	})
	if err != nil {
		writeTeamError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, apiTeam(created))
}

func (h *Handler) getTeam(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	item, found := svc.GetTeam(pathValue(r, "team_id"))
	if !found {
		http.Error(w, team.ErrTeamNotFound.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, apiTeam(item))
}

func (h *Handler) listTeamTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	if !h.ensureTeamExists(w, svc, pathValue(r, "team_id")) {
		return
	}
	tasks := svc.ListTasks(pathValue(r, "team_id"))
	resp := make([]apitypes.TeamTask, 0, len(tasks))
	for _, item := range tasks {
		resp = append(resp, apiTask(item))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) listGlobalTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	tasks := svc.ListAllTasks()
	resp := make([]apitypes.GlobalTask, 0, len(tasks))
	for _, item := range tasks {
		task := apitypes.GlobalTask{TeamTask: apiTask(item)}
		if meta, found := svc.GetTeam(item.TeamID); found {
			task.TeamTitle = meta.Title
		}
		if h.im != nil {
			if room, found := h.im.Room(item.RoomID); found {
				task.RoomTitle = room.Title
			}
		}
		resp = append(resp, task)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) createTeamTasksBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	var req apitypes.CreateTeamTasksBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	input := team.CreateTaskBatchInput{
		TeamID:    pathValue(r, "team_id"),
		CreatedBy: strings.TrimSpace(req.CreatedBy),
		Tasks:     make([]team.CreateTaskBatchItem, 0, len(req.Tasks)),
	}
	for _, item := range req.Tasks {
		input.Tasks = append(input.Tasks, team.CreateTaskBatchItem{
			IDRef:         strings.TrimSpace(item.IDRef),
			ParentID:      strings.TrimSpace(item.ParentID),
			ParentRef:     strings.TrimSpace(item.ParentRef),
			Title:         strings.TrimSpace(item.Title),
			Body:          strings.TrimSpace(item.Body),
			AssignTo:      strings.TrimSpace(item.AssignTo),
			DependsOnRefs: uniqueStrings(item.DependsOnRefs),
			Priority:      item.Priority,
			DeadlineAt:    item.DeadlineAt,
			TimeoutAt:     item.TimeoutAt,
		})
	}
	result, err := svc.CreateTasks(input)
	if err != nil {
		writeTeamError(w, err)
		return
	}
	resp := apitypes.CreateTeamTasksBatchResponse{
		Tasks:  make([]apitypes.TeamTask, 0, len(result.Tasks)),
		IDRefs: make([]apitypes.TeamTaskIDRef, 0, len(result.IDRefs)),
	}
	for _, item := range result.Tasks {
		resp.Tasks = append(resp.Tasks, apiTask(item))
	}
	for _, ref := range result.IDRefs {
		resp.IDRefs = append(resp.IDRefs, apitypes.TeamTaskIDRef{IDRef: ref.IDRef, TaskID: ref.TaskID})
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) claimNextTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	var req apitypes.ClaimNextTeamTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	teamID := pathValue(r, "team_id")
	if teamID == "" {
		teamID = strings.TrimSpace(req.TeamID)
	}
	item, err := svc.ClaimNext(teamID, strings.TrimSpace(req.BotID))
	if err != nil {
		writeTeamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiTask(item))
}

func (h *Handler) updateTeamTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	var req struct {
		apitypes.PatchTeamTaskRequest
		ActorID string `json:"actor_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	item, err := svc.UpdateTaskStatus(team.UpdateTaskStatusInput{
		TeamID:  pathValue(r, "team_id"),
		TaskID:  pathValue(r, "task_id"),
		ActorID: strings.TrimSpace(req.ActorID),
		Status:  strings.TrimSpace(req.Status),
		Result:  strings.TrimSpace(req.Result),
		Error:   strings.TrimSpace(req.Error),
		Reason:  strings.TrimSpace(req.Reason),
	})
	if err != nil {
		writeTeamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiTask(item))
}

func (h *Handler) assignTeamTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	var req struct {
		apitypes.AssignTeamTaskRequest
		ActorID string `json:"actor_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	item, err := svc.AssignTask(team.AssignTaskInput{
		TeamID:     pathValue(r, "team_id"),
		TaskID:     pathValue(r, "task_id"),
		AssignedTo: strings.TrimSpace(req.BotID),
		ActorID:    strings.TrimSpace(req.ActorID),
	})
	if err != nil {
		writeTeamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiTask(item))
}

func (h *Handler) listTeamApprovals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	if !h.ensureTeamExists(w, svc, pathValue(r, "team_id")) {
		return
	}
	approvals := svc.ListApprovals(pathValue(r, "team_id"))
	resp := make([]apitypes.TeamApproval, 0, len(approvals))
	for _, item := range approvals {
		resp = append(resp, apiApproval(item))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) createTeamApproval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	var req apitypes.CreateTeamApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	item, err := svc.RequestApproval(team.RequestApprovalInput{
		TeamID:      pathValue(r, "team_id"),
		TaskID:      strings.TrimSpace(req.TaskID),
		RequestedBy: strings.TrimSpace(req.RequestedBy),
		ApproverID:  strings.TrimSpace(req.ApproverID),
		Kind:        strings.TrimSpace(req.Kind),
		Summary:     strings.TrimSpace(req.Summary),
		Payload:     strings.TrimSpace(req.Payload),
	})
	if err != nil {
		writeTeamError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, apiApproval(item))
}

func (h *Handler) resolveTeamApproval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	var req apitypes.ResolveTeamApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	item, err := svc.ResolveApproval(team.ResolveApprovalInput{
		TeamID:     pathValue(r, "team_id"),
		ApprovalID: pathValue(r, "approval_id"),
		ApproverID: strings.TrimSpace(req.ApproverID),
		Status:     strings.TrimSpace(req.Status),
		Resolution: strings.TrimSpace(req.Reason),
	})
	if err != nil {
		writeTeamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiApproval(item))
}

func (h *Handler) listTeamEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	if !h.ensureTeamExists(w, svc, pathValue(r, "team_id")) {
		return
	}
	events := svc.ListEvents(pathValue(r, "team_id"))
	resp := make([]apitypes.TeamEvent, 0, len(events))
	for _, item := range events {
		resp = append(resp, apiEvent(item))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) requireTeamService(w http.ResponseWriter) (*team.Service, bool) {
	if h == nil || h.teamSvc == nil {
		http.Error(w, "team service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return h.teamSvc, true
}

func (h *Handler) requireTeamComponents(w http.ResponseWriter) (*team.Service, team.TeamChannelAdapter, bool) {
	svc, ok := h.requireTeamService(w)
	if !ok {
		return nil, nil, false
	}
	if h.teamAdapter == nil {
		http.Error(w, "team adapter is not configured", http.StatusServiceUnavailable)
		return nil, nil, false
	}
	return svc, h.teamAdapter, true
}

func (h *Handler) ensureTeamExists(w http.ResponseWriter, svc *team.Service, teamID string) bool {
	if _, found := svc.GetTeam(teamID); !found {
		http.Error(w, team.ErrTeamNotFound.Error(), http.StatusNotFound)
		return false
	}
	return true
}

func writeTeamError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, team.ErrTeamNotFound), errors.Is(err, team.ErrTaskNotFound), errors.Is(err, team.ErrApprovalNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, team.ErrTaskNotClaimable),
		errors.Is(err, team.ErrTaskDependenciesOpen),
		errors.Is(err, team.ErrWorkerAlreadyBusy),
		errors.Is(err, team.ErrTeamSelectionRequired):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, team.ErrTaskTransitionInvalid),
		errors.Is(err, team.ErrApprovalAlreadyHandled):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

func apiTeam(item team.TeamMeta) apitypes.Team {
	return apitypes.Team{
		ID:        item.ID,
		RoomID:    item.RoomID,
		Channel:   item.Channel,
		Title:     item.Title,
		LeadBotID: item.LeadBotID,
		Status:    item.Status,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	}
}

func apiTask(item team.TeamTask) apitypes.TeamTask {
	return apitypes.TeamTask{
		ID:          item.ID,
		TeamID:      item.TeamID,
		RoomID:      item.RoomID,
		ParentID:    item.ParentID,
		Title:       item.Title,
		Body:        item.Body,
		Status:      item.Status,
		CreatedBy:   item.CreatedBy,
		AssignedTo:  item.AssignedTo,
		ClaimedBy:   item.ClaimedBy,
		DependsOn:   append([]string(nil), item.DependsOn...),
		Priority:    item.Priority,
		DeadlineAt:  item.DeadlineAt,
		TimeoutAt:   item.TimeoutAt,
		Result:      item.Result,
		Error:       item.Error,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
		CompletedAt: item.CompletedAt,
	}
}

func apiApproval(item team.TeamApproval) apitypes.TeamApproval {
	return apitypes.TeamApproval{
		ID:          item.ID,
		TeamID:      item.TeamID,
		RoomID:      item.RoomID,
		TaskID:      item.TaskID,
		RequestedBy: item.RequestedBy,
		ApproverID:  item.ApproverID,
		Kind:        item.Kind,
		Summary:     item.Summary,
		Payload:     item.Payload,
		Status:      item.Status,
		Resolution:  item.Resolution,
		CreatedAt:   item.CreatedAt,
		ResolvedAt:  item.ResolvedAt,
	}
}

func apiEvent(item team.TeamEvent) apitypes.TeamEvent {
	return apitypes.TeamEvent{
		Seq:       item.Seq,
		TeamID:    item.TeamID,
		RoomID:    item.RoomID,
		Type:      item.Type,
		ActorID:   item.ActorID,
		TaskID:    item.TaskID,
		TargetID:  item.TargetID,
		Summary:   item.Summary,
		CreatedAt: item.CreatedAt,
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
