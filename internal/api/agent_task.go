package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"csgclaw/internal/agenttask"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/taskcore"
)

func (h *Handler) requireAgentTaskService(w http.ResponseWriter) (*agenttask.Service, bool) {
	if h == nil || h.agentTaskSvc == nil {
		http.Error(w, "agent task service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return h.agentTaskSvc, true
}

func (h *Handler) handleListAgentTasks(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireAgentTaskService(w)
	if !ok {
		return
	}
	presenter := h.newTeamIdentityPresenter()
	tasks := svc.List()
	resp := make([]apitypes.TeamTask, 0, len(tasks))
	for _, task := range tasks {
		if agentID := appRequestAgentID(r); agentID != "" {
			if _, err := h.appDirectTask(agentID, task.ID, false); err != nil {
				continue
			}
		}
		resp = append(resp, apiCoreTask(task, presenter))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleCreateAgentTask(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireAgentTaskService(w)
	if !ok {
		return
	}
	var req apitypes.CreateAgentTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	task, err := svc.CreateAgentTask(r.Context(), agenttask.CreateInput{
		AgentID:   strings.TrimSpace(req.AgentID),
		Title:     strings.TrimSpace(req.Title),
		Body:      strings.TrimSpace(req.Body),
		CreatedBy: strings.TrimSpace(req.CreatedBy),
	})
	if err != nil {
		writeTaskCoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, apiCoreTask(task, h.newTeamIdentityPresenter()))
}

func (h *Handler) handleClaimAgentTask(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireAgentTaskService(w)
	if !ok {
		return
	}
	var req apitypes.ClaimAgentTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	task, err := svc.Claim(agenttask.ClaimInput{
		TaskID:        pathValue(r, "task_id"),
		ParticipantID: strings.TrimSpace(req.ParticipantID),
	})
	if err != nil {
		writeTaskCoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiCoreTask(task, h.newTeamIdentityPresenter()))
}

func (h *Handler) handleUpdateAgentTask(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireAgentTaskService(w)
	if !ok {
		return
	}
	var req apitypes.PatchAgentTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	task, err := svc.Update(agenttask.UpdateInput{
		TaskID:  pathValue(r, "task_id"),
		ActorID: strings.TrimSpace(req.ActorID),
		Status:  strings.TrimSpace(req.Status),
		Result:  strings.TrimSpace(req.Result),
		Error:   strings.TrimSpace(req.Error),
		Reason:  strings.TrimSpace(req.Reason),
	})
	if err != nil {
		writeTaskCoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiCoreTask(task, h.newTeamIdentityPresenter()))
}

func writeTaskCoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, taskcore.ErrTaskNotFound), errors.Is(err, taskcore.ErrApprovalNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, taskcore.ErrTransitionInvalid), errors.Is(err, taskcore.ErrApprovalAlreadyClosed):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
