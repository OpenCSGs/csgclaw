package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/scheduledtask"
)

func (h *Handler) requireScheduledTaskService(w http.ResponseWriter) (*scheduledtask.Service, bool) {
	if h == nil || h.scheduledTaskSvc == nil {
		http.Error(w, "scheduled task service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return h.scheduledTaskSvc, true
}

func (h *Handler) handleListScheduledTasks(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireScheduledTaskService(w)
	if !ok {
		return
	}
	items := svc.List()
	resp := make([]apitypes.ScheduledTask, 0, len(items))
	for _, item := range items {
		resp = append(resp, apiScheduledTask(item))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleCreateScheduledTask(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireScheduledTaskService(w)
	if !ok {
		return
	}
	var req apitypes.CreateScheduledTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	item, err := svc.Create(scheduledtask.CreateInput{
		Title:      strings.TrimSpace(req.Title),
		AgentID:    strings.TrimSpace(req.AgentID),
		Prompt:     strings.TrimSpace(req.Prompt),
		Recurrence: strings.TrimSpace(req.Recurrence),
		FirstRunAt: req.FirstRunAt.In(time.Local),
		ExpiresAt:  localTimePtr(req.ExpiresAt),
		Enabled:    enabled,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, apiScheduledTask(item))
}

func (h *Handler) handleUpdateScheduledTask(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireScheduledTaskService(w)
	if !ok {
		return
	}
	var req patchScheduledTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	item, err := svc.Update(scheduledtask.UpdateInput{
		ID:         pathValue(r, "scheduled_task_id"),
		Title:      req.Title,
		AgentID:    req.AgentID,
		Prompt:     req.Prompt,
		Recurrence: req.Recurrence,
		NextRunAt:  localTimeValuePtr(req.NextRunAt),
		ExpiresAt:  localOptionalTimeDoublePtr(req.ExpiresAt),
		Enabled:    req.Enabled,
	})
	if err != nil {
		writeScheduledTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiScheduledTask(item))
}

func (h *Handler) handleDeleteScheduledTask(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireScheduledTaskService(w)
	if !ok {
		return
	}
	if err := svc.Delete(pathValue(r, "scheduled_task_id")); err != nil {
		writeScheduledTaskError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleListScheduledTaskRuns(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireScheduledTaskService(w)
	if !ok {
		return
	}
	runs := svc.Runs(pathValue(r, "scheduled_task_id"))
	resp := make([]apitypes.ScheduledTaskRun, 0, len(runs))
	for _, run := range runs {
		resp = append(resp, apiScheduledTaskRun(run))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleRunScheduledTaskNow(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.requireScheduledTaskService(w)
	if !ok {
		return
	}
	run, err := svc.RunNow(r.Context(), pathValue(r, "scheduled_task_id"))
	if err != nil {
		writeScheduledTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, apiScheduledTaskRun(run))
}

func writeScheduledTaskError(w http.ResponseWriter, err error) {
	if errors.Is(err, scheduledtask.ErrActiveTask) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if strings.Contains(strings.ToLower(err.Error()), "not found") {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}

func apiScheduledTask(item scheduledtask.Task) apitypes.ScheduledTask {
	return apitypes.ScheduledTask{
		ID:         item.ID,
		Title:      item.Title,
		AgentID:    item.AgentID,
		Prompt:     item.Prompt,
		Recurrence: item.Recurrence,
		Enabled:    item.Enabled,
		NextRunAt:  item.NextRunAt,
		LastRunAt:  item.LastRunAt,
		ExpiresAt:  item.ExpiresAt,
		CreatedAt:  item.CreatedAt,
		UpdatedAt:  item.UpdatedAt,
	}
}

func apiScheduledTaskRun(run scheduledtask.Run) apitypes.ScheduledTaskRun {
	return apitypes.ScheduledTaskRun{
		ID:              run.ID,
		ScheduledTaskID: run.ScheduledTaskID,
		TriggeredAt:     run.TriggeredAt,
		Status:          run.Status,
		TaskID:          run.TaskID,
		Error:           run.Error,
	}
}

func localTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	next := value.In(time.Local)
	return &next
}

func localTimeValuePtr(value *time.Time) *time.Time {
	return localTimePtr(value)
}

func localTimeDoublePtr(value *time.Time) **time.Time {
	if value == nil {
		return nil
	}
	next := localTimePtr(value)
	return &next
}

type patchScheduledTaskRequest struct {
	Title      *string      `json:"title,omitempty"`
	AgentID    *string      `json:"agent_id,omitempty"`
	Prompt     *string      `json:"prompt,omitempty"`
	Recurrence *string      `json:"recurrence,omitempty"`
	NextRunAt  *time.Time   `json:"next_run_at,omitempty"`
	ExpiresAt  optionalTime `json:"expires_at,omitempty"`
	Enabled    *bool        `json:"enabled,omitempty"`
}

type optionalTime struct {
	set   bool
	value *time.Time
}

func (t *optionalTime) UnmarshalJSON(data []byte) error {
	t.set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		t.value = nil
		return nil
	}
	var value time.Time
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	t.value = &value
	return nil
}

func localOptionalTimeDoublePtr(value optionalTime) **time.Time {
	if !value.set {
		return nil
	}
	if value.value == nil {
		var cleared *time.Time
		return &cleared
	}
	return localTimeDoublePtr(value.value)
}
