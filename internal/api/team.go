package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/participant"
	"csgclaw/internal/team"
)

func (h *Handler) handleListTeams(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, apiTeams(svc.ListTeams()))
}

func (h *Handler) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	svc, adapter, ok := h.requireTeamComponents(w)
	if !ok {
		return
	}

	var req apitypes.CreateTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	leadAgentID, memberAgentIDs, err := h.resolveCreateTeamAgents(req)
	if err != nil {
		writeTeamError(w, err)
		return
	}
	created, err := svc.CreateTeamWithRoom(r.Context(), adapter, team.CreateTeamWithRoomInput{
		RoomID:         strings.TrimSpace(req.RoomID),
		Channel:        strings.TrimSpace(req.Channel),
		Title:          strings.TrimSpace(req.Title),
		LeadAgentID:    leadAgentID,
		MemberAgentIDs: memberAgentIDs,
	})
	if err != nil {
		writeTeamError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, apiTeam(created))
}

func (h *Handler) resolveCreateTeamAgents(req apitypes.CreateTeamRequest) (string, []string, error) {
	leadAgentID := strings.TrimSpace(req.LeadAgentID)
	if leadAgentID == "" && strings.TrimSpace(req.LeadParticipantID) != "" {
		resolved, err := h.resolveCreateTeamParticipantID("lead_participant_id", req.LeadParticipantID)
		if err != nil {
			return "", nil, err
		}
		leadAgentID = resolved
	}

	memberAgentIDs := req.MemberAgentIDs
	if len(req.MemberAgentIDs) > 0 {
		memberAgentIDs = req.MemberAgentIDs
	} else if len(req.MemberParticipantIDs) > 0 {
		memberAgentIDs = make([]string, 0, len(req.MemberParticipantIDs))
		for _, participantID := range req.MemberParticipantIDs {
			resolved, err := h.resolveCreateTeamParticipantID("member_participant_ids", participantID)
			if err != nil {
				return "", nil, err
			}
			if strings.TrimSpace(resolved) == "" {
				continue
			}
			memberAgentIDs = append(memberAgentIDs, resolved)
		}
	}

	return leadAgentID, memberAgentIDs, nil
}

func (h *Handler) resolveCreateTeamParticipantID(field, id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", nil
	}
	if id == agent.ManagerParticipantID {
		return agent.ManagerUserID, nil
	}
	if h != nil && h.participant != nil {
		if item, ok := h.participant.Get(participant.ChannelCSGClaw, id); ok {
			if agentID := strings.TrimSpace(item.AgentID); agentID != "" {
				return agentID, nil
			}
		}
	}
	if strings.HasPrefix(id, "u-") {
		return id, nil
	}
	return "u-" + id, nil
}

func (h *Handler) handleGetTeam(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) handleListTeamTasks(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	if !h.ensureTeamExists(w, svc, pathValue(r, "team_id")) {
		return
	}
	writeJSON(w, http.StatusOK, apiTasks(svc.ListTasks(pathValue(r, "team_id")), h.newTeamIdentityPresenter()))
}

func (h *Handler) handleListGlobalTasks(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, apiGlobalTasks(svc.ListGlobalTaskViews(h.teamDirectory()), h.newTeamIdentityPresenter()))
}

func (h *Handler) handleCreateTeamTasksBatch(w http.ResponseWriter, r *http.Request) {
	svc, adapter, ok := h.requireTeamComponents(w)
	if !ok {
		return
	}
	var req apitypes.CreateTeamTasksBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	teamID := pathValue(r, "team_id")
	result, err := svc.CreateTasksWithExecutionRoom(r.Context(), teamCreateTaskBatchInput(teamID, req), adapter, h.teamDirectory())
	if err != nil {
		writeTeamError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, apiCreateTasksBatchResponse(result, h.newTeamIdentityPresenter()))
}

func (h *Handler) handleClaimNextTask(w http.ResponseWriter, r *http.Request) {
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
	item, err := svc.ClaimNext(teamID, strings.TrimSpace(req.ParticipantID))
	if err != nil {
		writeTeamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiTask(item, h.newTeamIdentityPresenter()))
}

func (h *Handler) handleClaimTeamTask(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	var req apitypes.ClaimTeamTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	item, err := svc.ClaimTask(team.ClaimTaskInput{
		TeamID:        pathValue(r, "team_id"),
		TaskID:        pathValue(r, "task_id"),
		ParticipantID: strings.TrimSpace(req.ParticipantID),
	})
	if err != nil {
		writeTeamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiTask(item, h.newTeamIdentityPresenter()))
}

func (h *Handler) handleUpdateTeamTask(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, apiTask(item, h.newTeamIdentityPresenter()))
}

func (h *Handler) handlePlanTeamTask(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	var req struct {
		apitypes.PlanTeamTaskRequest
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	var adapter team.TeamChannelAdapter
	if req.AutoStart {
		var ok bool
		_, adapter, ok = h.requireTeamComponents(w)
		if !ok {
			return
		}
	}
	directory := h.teamDirectory()
	result, err := svc.PlanTaskWithOptionalStart(r.Context(), team.PlanTaskWorkflowInput{
		TeamID:    pathValue(r, "team_id"),
		TaskID:    pathValue(r, "task_id"),
		ActorID:   strings.TrimSpace(req.ActorID),
		AutoStart: req.AutoStart,
	}, adapter, directory, team.NewManagerPlanner(h.llm, directory))
	if err != nil {
		writeTeamPlanError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiPlanTaskWorkflowResponse(result, h.newTeamIdentityPresenter()))
}

func (h *Handler) handleStartTeamTask(w http.ResponseWriter, r *http.Request) {
	svc, adapter, ok := h.requireTeamComponents(w)
	if !ok {
		return
	}
	var req struct {
		apitypes.StartTeamTaskRequest
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	item, err := svc.StartTaskWithExecutionRoom(r.Context(), team.StartTaskWithExecutionRoomInput{
		TeamID:  pathValue(r, "team_id"),
		TaskID:  pathValue(r, "task_id"),
		ActorID: strings.TrimSpace(req.ActorID),
	}, adapter, h.teamDirectory())
	if err != nil {
		writeTeamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apitypes.StartTeamTaskResponse{
		Task:           apiTask(item.Parent, h.newTeamIdentityPresenter()),
		ScheduledTasks: item.ScheduledCount,
	})
}

func (h *Handler) handleAssignTeamTask(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	var req struct {
		ParticipantID string `json:"participant_id"`
		ActorID       string `json:"actor_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	item, err := svc.AssignTask(team.AssignTaskInput{
		TeamID:     pathValue(r, "team_id"),
		TaskID:     pathValue(r, "task_id"),
		AssignedTo: strings.TrimSpace(req.ParticipantID),
		ActorID:    strings.TrimSpace(req.ActorID),
	})
	if err != nil {
		writeTeamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiTask(item, h.newTeamIdentityPresenter()))
}

func (h *Handler) handleListTeamApprovals(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	if !h.ensureTeamExists(w, svc, pathValue(r, "team_id")) {
		return
	}
	writeJSON(w, http.StatusOK, apiApprovals(svc.ListApprovals(pathValue(r, "team_id"))))
}

func (h *Handler) handleCreateTeamApproval(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) handleResolveTeamApproval(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) handleListTeamEvents(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireTeamService(w)
	if !ok {
		return
	}
	if !h.ensureTeamExists(w, svc, pathValue(r, "team_id")) {
		return
	}
	writeJSON(w, http.StatusOK, apiEvents(svc.ListEvents(pathValue(r, "team_id")), h.newTeamIdentityPresenter()))
}
