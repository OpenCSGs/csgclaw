package api

import (
	"errors"
	"net/http"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/llm"
	"csgclaw/internal/participant"
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

func apiTeamWithPresenter(item team.TeamMeta, presenter teamIdentityPresenter) apitypes.Team {
	resp := apiTeam(item)
	resp.LeadAgentName = presenter.displayAgentName(resp.LeadAgentID)
	return resp
}

func apiTeams(items []team.TeamMeta) []apitypes.Team {
	resp := make([]apitypes.Team, 0, len(items))
	for _, item := range items {
		resp = append(resp, apiTeam(item))
	}
	return resp
}

func apiTeamsWithPresenter(items []team.TeamMeta, presenter teamIdentityPresenter) []apitypes.Team {
	resp := make([]apitypes.Team, 0, len(items))
	for _, item := range items {
		resp = append(resp, apiTeamWithPresenter(item, presenter))
	}
	return resp
}

type teamIdentityPresenter struct {
	agents    *agent.Service
	namesByID map[string]string
}

func (h *Handler) newTeamIdentityPresenter() teamIdentityPresenter {
	p := teamIdentityPresenter{namesByID: make(map[string]string)}
	if h != nil {
		p.agents = h.svc
	}
	if h == nil || h.participant == nil {
		return p
	}
	for _, item := range h.participant.List(participant.ListOptions{Channel: participant.ChannelCSGClaw}) {
		name := p.agentDisplayName(item.AgentID)
		if name == "" {
			name = strings.TrimSpace(item.Name)
		}
		if name == "" {
			name = strings.TrimSpace(item.ChannelUserRef)
		}
		if name == "" {
			continue
		}
		p.addName(item.ID, name)
		p.addName(item.ChannelUserRef, name)
		p.addName(item.AgentID, name)
	}
	return p
}

func (p teamIdentityPresenter) displayAgentName(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || p.namesByID == nil {
		return ""
	}
	if name := p.namesByID[id]; name != "" {
		return name
	}
	return p.agentDisplayName(id)
}

func (p teamIdentityPresenter) addName(id string, name string) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if id == "" || name == "" {
		return
	}
	p.namesByID[id] = name
	for _, alias := range teamIdentityAliases(id) {
		p.namesByID[alias] = name
	}
}

func teamIdentityAliases(id string) []string {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	aliases := make([]string, 0, 4)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || value == id {
			return
		}
		for _, existing := range aliases {
			if existing == value {
				return
			}
		}
		aliases = append(aliases, value)
	}
	switch {
	case strings.HasPrefix(id, "pt-"):
		suffix := strings.TrimPrefix(id, "pt-")
		add(suffix)
		add("agent-" + suffix)
		add("u-" + suffix)
	case strings.HasPrefix(id, "agent-"):
		suffix := strings.TrimPrefix(id, "agent-")
		add(suffix)
		add("pt-" + suffix)
		add("u-" + suffix)
	case strings.HasPrefix(id, "u-"):
		suffix := strings.TrimPrefix(id, "u-")
		add(suffix)
		add("pt-" + suffix)
		add("agent-" + suffix)
	default:
		add("pt-" + id)
		add("agent-" + id)
		add("u-" + id)
	}
	return aliases
}

func (p teamIdentityPresenter) agentDisplayName(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || p.agents == nil {
		return ""
	}
	if name, ok := p.agents.AgentDisplayName(id); ok {
		return name
	}
	if strings.HasPrefix(id, "u-") || id == "" {
		return ""
	}
	name, _ := p.agents.AgentDisplayName("u-" + id)
	return name
}

func apiTask(item team.TeamTask, presenter teamIdentityPresenter) apitypes.TeamTask {
	return apitypes.TeamTask{
		ID:                  item.ID,
		TeamID:              item.TeamID,
		RoomID:              item.RoomID,
		ParentID:            item.ParentID,
		Title:               item.Title,
		Body:                item.Body,
		Status:              item.Status,
		CreatedBy:           item.CreatedBy,
		CreatedByAgentName:  presenter.displayAgentName(item.CreatedBy),
		AssignedTo:          item.AssignedTo,
		AssignedToAgentName: presenter.displayAgentName(item.AssignedTo),
		ClaimedBy:           item.ClaimedBy,
		ClaimedByAgentName:  presenter.displayAgentName(item.ClaimedBy),
		DependsOn:           append([]string(nil), item.DependsOn...),
		Priority:            item.Priority,
		PlanSummary:         item.PlanSummary,
		DispatchedAt:        item.DispatchedAt,
		DeadlineAt:          item.DeadlineAt,
		TimeoutAt:           item.TimeoutAt,
		Result:              item.Result,
		Error:               item.Error,
		CreatedAt:           item.CreatedAt,
		UpdatedAt:           item.UpdatedAt,
		CompletedAt:         item.CompletedAt,
	}
}

func apiTasks(items []team.TeamTask, presenter teamIdentityPresenter) []apitypes.TeamTask {
	resp := make([]apitypes.TeamTask, 0, len(items))
	for _, item := range items {
		resp = append(resp, apiTask(item, presenter))
	}
	return resp
}

func apiGlobalTask(item team.GlobalTaskView, presenter teamIdentityPresenter) apitypes.GlobalTask {
	return apitypes.GlobalTask{
		TeamTask:  apiTask(item.Task, presenter),
		TeamTitle: item.TeamTitle,
		RoomTitle: item.RoomTitle,
	}
}

func apiGlobalTasks(items []team.GlobalTaskView, presenter teamIdentityPresenter) []apitypes.GlobalTask {
	resp := make([]apitypes.GlobalTask, 0, len(items))
	for _, item := range items {
		resp = append(resp, apiGlobalTask(item, presenter))
	}
	return resp
}

func apiPlanTaskWorkflowResponse(result team.PlanTaskWorkflowResult, presenter teamIdentityPresenter) apitypes.PlanTeamTaskResponse {
	return apitypes.PlanTeamTaskResponse{
		Task:           apiTask(result.Parent, presenter),
		CreatedTasks:   apiTasks(result.Tasks, presenter),
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

func apiCreateTasksBatchResponse(result team.CreateTasksResult, presenter teamIdentityPresenter) apitypes.CreateTeamTasksBatchResponse {
	resp := apitypes.CreateTeamTasksBatchResponse{
		Tasks:  make([]apitypes.TeamTask, 0, len(result.Tasks)),
		IDRefs: make([]apitypes.TeamTaskIDRef, 0, len(result.IDRefs)),
	}
	for _, item := range result.Tasks {
		resp.Tasks = append(resp.Tasks, apiTask(item, presenter))
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

func apiEvent(item team.TeamEvent, presenter teamIdentityPresenter) apitypes.TeamEvent {
	return apitypes.TeamEvent{
		Seq:             item.Seq,
		TeamID:          item.TeamID,
		RoomID:          item.RoomID,
		Type:            item.Type,
		ActorID:         item.ActorID,
		ActorAgentName:  presenter.displayAgentName(item.ActorID),
		TaskID:          item.TaskID,
		TargetID:        item.TargetID,
		TargetAgentName: presenter.displayAgentName(item.TargetID),
		Summary:         item.Summary,
		CreatedAt:       item.CreatedAt,
	}
}

func apiEvents(items []team.TeamEvent, presenter teamIdentityPresenter) []apitypes.TeamEvent {
	resp := make([]apitypes.TeamEvent, 0, len(items))
	for _, item := range items {
		resp = append(resp, apiEvent(item, presenter))
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
