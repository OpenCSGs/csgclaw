package api

import (
	"errors"
	"net/http"
	"strings"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/llm"
	"csgclaw/internal/team"
)

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

func (h *Handler) teamDirectory() *team.CSGClawTeamDirectory {
	if h == nil {
		return nil
	}
	return team.NewCSGClawTeamDirectory(h.im, h.svc, h.participant)
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
		errors.Is(err, team.ErrTaskNoSubtasks),
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

func writeTeamPlanError(w http.ResponseWriter, err error) {
	var llmErr *llm.HTTPError
	if errors.Is(err, team.ErrManagerPlannerUnavailable) ||
		errors.Is(err, team.ErrManagerPlannerFailed) ||
		errors.As(err, &llmErr) {
		writeTeamPlannerError(w, err)
		return
	}
	writeTeamError(w, err)
}

func writeTeamPlannerError(w http.ResponseWriter, err error) {
	if errors.Is(err, team.ErrManagerPlannerUnavailable) {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	var llmErr *llm.HTTPError
	if errors.As(err, &llmErr) {
		writeLLMError(w, llmErr)
		return
	}
	http.Error(w, err.Error(), http.StatusBadGateway)
}

func apiTeam(item team.TeamMeta) apitypes.Team {
	return apitypes.Team{
		ID:          item.ID,
		RoomID:      item.RoomID,
		Channel:     item.Channel,
		Title:       item.Title,
		LeadAgentID: item.LeadAgentID,
		Status:      item.Status,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}

func apiTeams(items []team.TeamMeta) []apitypes.Team {
	resp := make([]apitypes.Team, 0, len(items))
	for _, item := range items {
		resp = append(resp, apiTeam(item))
	}
	return resp
}

func apiTask(item team.TeamTask) apitypes.TeamTask {
	return apitypes.TeamTask{
		ID:           item.ID,
		TeamID:       item.TeamID,
		RoomID:       item.RoomID,
		ParentID:     item.ParentID,
		Title:        item.Title,
		Body:         item.Body,
		Status:       item.Status,
		CreatedBy:    item.CreatedBy,
		AssignedTo:   item.AssignedTo,
		ClaimedBy:    item.ClaimedBy,
		DependsOn:    append([]string(nil), item.DependsOn...),
		Priority:     item.Priority,
		PlanSummary:  item.PlanSummary,
		DispatchedAt: item.DispatchedAt,
		DeadlineAt:   item.DeadlineAt,
		TimeoutAt:    item.TimeoutAt,
		Result:       item.Result,
		Error:        item.Error,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
		CompletedAt:  item.CompletedAt,
	}
}

func apiTasks(items []team.TeamTask) []apitypes.TeamTask {
	resp := make([]apitypes.TeamTask, 0, len(items))
	for _, item := range items {
		resp = append(resp, apiTask(item))
	}
	return resp
}

func apiGlobalTask(item team.GlobalTaskView) apitypes.GlobalTask {
	return apitypes.GlobalTask{
		TeamTask:  apiTask(item.Task),
		TeamTitle: item.TeamTitle,
		RoomTitle: item.RoomTitle,
	}
}

func apiGlobalTasks(items []team.GlobalTaskView) []apitypes.GlobalTask {
	resp := make([]apitypes.GlobalTask, 0, len(items))
	for _, item := range items {
		resp = append(resp, apiGlobalTask(item))
	}
	return resp
}

func apiPlanTaskWorkflowResponse(result team.PlanTaskWorkflowResult) apitypes.PlanTeamTaskResponse {
	return apitypes.PlanTeamTaskResponse{
		Task:           apiTask(result.Parent),
		CreatedTasks:   apiTasks(result.Tasks),
		AlreadyPlanned: result.AlreadyPlanned,
		Started:        result.Started,
		ScheduledTasks: result.ScheduledCount,
	}
}

func teamCreateTaskBatchInput(teamID string, req apitypes.CreateTeamTasksBatchRequest) team.CreateTaskBatchInput {
	input := team.CreateTaskBatchInput{
		TeamID:    teamID,
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
	return input
}

func apiCreateTasksBatchResponse(result team.CreateTasksResult) apitypes.CreateTeamTasksBatchResponse {
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
	return resp
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

func apiApprovals(items []team.TeamApproval) []apitypes.TeamApproval {
	resp := make([]apitypes.TeamApproval, 0, len(items))
	for _, item := range items {
		resp = append(resp, apiApproval(item))
	}
	return resp
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

func apiEvents(items []team.TeamEvent) []apitypes.TeamEvent {
	resp := make([]apitypes.TeamEvent, 0, len(items))
	for _, item := range items {
		resp = append(resp, apiEvent(item))
	}
	return resp
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
