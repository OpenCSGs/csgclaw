package api

import (
	"context"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/taskcore"
	"csgclaw/internal/worklease"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type roomWorkReader interface {
	ActiveWork(string) []apitypes.ParticipantWorkUpdate
}

func (h *Handler) RecoverRoomTasks() error {
	if h.roomTaskSvc == nil {
		return nil
	}
	return h.roomTaskSvc.Recover()
}
func (h *Handler) handleRoomTaskEvents(w http.ResponseWriter, r *http.Request) {
	if !h.requireRoomTasks(w, r) {
		return
	}
	room, id := pathValue(r, "id"), pathValue(r, "task_id")
	if _, ok := h.roomTaskSvc.Get(room, id); !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, h.roomTaskSvc.Events(room, id))
}
func (h *Handler) handleRoomTaskRecover(w http.ResponseWriter, r *http.Request) {
	if !h.requireRoomManager(w, r) {
		return
	}
	var req struct {
		Attempt    int    `json:"attempt"`
		Assessment string `json:"assessment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	room, id := pathValue(r, "id"), pathValue(r, "task_id")
	if leases, ok := h.participantWork.(roomWorkReader); ok {
		for _, l := range leases.ActiveWork(room) {
			if h.roomTaskIDForSource(room, l.RequestID) == id {
				http.Error(w, "previous task still has an active execution; stop it first", http.StatusConflict)
				return
			}
		}
	}
	if err := h.roomTaskSvc.ResolveRecovery(room, id, req.Attempt, req.Assessment); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	t, _ := h.roomTaskSvc.Get(room, id)
	writeJSON(w, http.StatusOK, h.apiRoomTask(t))
}
func (h *Handler) handleRoomTaskStop(w http.ResponseWriter, r *http.Request) {
	if !h.requireRoomManager(w, r) {
		return
	}
	room, id := pathValue(r, "id"), pathValue(r, "task_id")
	if err := h.roomTaskSvc.RequestStop(room, id); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	reader, readable := h.participantWork.(roomWorkReader)
	controller, controllable := h.participantWork.(worklease.ParticipantWorkController)
	problems := []string{}
	for _, t := range h.roomTaskSvc.List(room) {
		if t.ParentID != id || t.Status != taskcore.StatusInProgress {
			continue
		}
		if !readable || !controllable {
			problems = append(problems, "execution controller unavailable for "+t.ID)
			continue
		}
		found := false
		for _, l := range reader.ActiveWork(room) {
			if h.roomTaskIDForSource(room, l.RequestID) != t.ID || h.roomTaskAttemptForSource(room, l.RequestID) != t.Attempt {
				continue
			}
			found = true
			ctx, cancel := context.WithTimeout(r.Context(), participantTurnStopTimeout)
			_, err := controller.RequestStop(ctx, l.ParticipantID, apitypes.ParticipantWorkStopRequest{RoomID: room, LeaseID: l.LeaseID, RequestID: l.RequestID})
			cancel()
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", t.ID, err))
			}
		}
		if !found {
			problems = append(problems, t.ID+": awaiting execution confirmation; inspect the current turn before retrying")
		}
	}
	if len(problems) > 0 {
		http.Error(w, "stop saved; "+strings.Join(problems, "; "), http.StatusConflict)
		return
	}
	t, _ := h.roomTaskSvc.Get(room, id)
	writeJSON(w, http.StatusOK, h.apiRoomTask(t))
}
