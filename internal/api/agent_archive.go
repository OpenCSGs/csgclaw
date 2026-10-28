package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	"csgclaw/internal/scheduledtask"
)

func (h *Handler) agentLifecycleState(id string) string {
	if h == nil || h.agentEngine == nil {
		return ""
	}
	id = h.runtimeAgentIDForBridgeID(id)
	if id == "" {
		return ""
	}
	// The record lookup is in-memory; never probe runtimes under the IM lock.
	if h.svc != nil {
		if item, ok := h.svc.Agent(id); ok {
			if item.ArchivedAt != nil {
				return "archived"
			}
			return ""
		}
	}
	if item, err := h.agentEngine.Agents().Get(context.Background(), id, agentengine.AgentGetOptions{}); err == nil && item.ArchivedAt != nil {
		return "archived"
	}
	return ""
}
func (h *Handler) configureAgentLifecycle() {
	if h.im != nil {
		h.im.SetAgentStateLookup(h.agentLifecycleState)
	}
}
func (h *Handler) availableScheduledAgent(id string) error {
	if h.agentEngine == nil {
		return nil
	}
	item, err := h.agentEngine.Agents().Get(context.Background(), id, agentengine.AgentGetOptions{})
	if err != nil {
		return fmt.Errorf("scheduled agent is deleted or unavailable")
	}
	if item.ArchivedAt != nil {
		return fmt.Errorf("scheduled agent is archived; restore it before enabling this task")
	}
	return nil
}
func (h *Handler) handleArchiveAgent(w http.ResponseWriter, r *http.Request) {
	h.setAgentArchived(w, r, true)
}
func (h *Handler) handleRestoreAgent(w http.ResponseWriter, r *http.Request) {
	h.setAgentArchived(w, r, false)
}
func (h *Handler) setAgentArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	h.agentArchiveMu.Lock()
	defer h.agentArchiveMu.Unlock()
	if h.agentEngine == nil {
		http.Error(w, "agent service unavailable", http.StatusServiceUnavailable)
		return
	}
	id := agent.CanonicalID(pathValue(r, "id"))
	previous, lookupErr := h.agentEngine.Agents().Get(r.Context(), id, agentengine.AgentGetOptions{})
	if lookupErr != nil {
		writeAgentGetError(w, lookupErr)
		return
	}
	item, err := h.agentEngine.Agents().SetArchived(r.Context(), id, archived)
	// Even a runtime stop failure can leave the durable archive flag in place.
	// Schedules and UI must reflect that disabled state, and retries are safe.
	if archived && h.agentLifecycleState(id) == "archived" {
		if h.scheduledTaskSvc != nil {
			if pauseErr := h.scheduledTaskSvc.DisableAgent(id); pauseErr != nil && err == nil {
				err = pauseErr
			}
		}
		if h.agentTaskSvc != nil {
			if cancelErr := h.agentTaskSvc.CancelAgentTasks(id); cancelErr != nil && err == nil {
				err = cancelErr
			}
		}
		if h.roomTaskSvc != nil {
			actors := []string{id, agent.ParticipantIDForAgent("", id)}
			if h.participant != nil {
				for _, p := range h.participant.List(participant.ListOptions{AgentID: id}) {
					actors = append(actors, p.ID, p.ChannelUserRef)
				}
			}
			if cancelErr := h.roomTaskSvc.CancelAgentTasks(actors); cancelErr != nil && err == nil {
				err = cancelErr
			}
		}
	}
	if h.im != nil {
		if archived && h.agentLifecycleState(id) == "archived" {
			current, getErr := h.agentEngine.Agents().Get(context.WithoutCancel(r.Context()), id, agentengine.AgentGetOptions{})
			if getErr == nil && current.ArchivedAt != nil {
				if noticeErr := h.im.RecordAgentLifecycle(id, im.AgentArchivedEventKey, current.ArchivedAt.Format(time.RFC3339Nano)); noticeErr != nil && err == nil {
					err = noticeErr
				}
			}
		} else if !archived && err == nil && previous.ArchivedAt != nil && item.ArchivedAt == nil {
			if noticeErr := h.im.RecordAgentLifecycle(id, im.AgentRestoredEventKey, previous.ArchivedAt.Format(time.RFC3339Nano)); noticeErr != nil {
				err = noticeErr
			}
		}
		h.im.PublishAgentState(id)
	}
	if err != nil {
		writeAgentOperationError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, h.presentEngineAgentForRequest(r, item))
}

type agentArchiveImpact struct {
	Teams          int `json:"teams"`
	Rooms          int `json:"rooms"`
	ScheduledTasks int `json:"scheduled_tasks"`
}

func (h *Handler) handleAgentArchiveImpact(w http.ResponseWriter, r *http.Request) {
	id := agent.CanonicalID(pathValue(r, "id"))
	impact := agentArchiveImpact{}
	if h.teamSvc != nil {
		for _, item := range h.teamSvc.ListTeams() {
			if item.LeadAgentID == id || slices.Contains(item.MemberAgentIDs, id) {
				impact.Teams++
			}
		}
	}
	if h.im != nil {
		impact.Rooms = len(h.im.RoomIDsForMember(id))
	}
	if h.scheduledTaskSvc != nil {
		for _, item := range h.scheduledTaskSvc.List() {
			if item.AgentID == id && item.Enabled {
				impact.ScheduledTasks++
			}
		}
	}
	writeJSON(w, http.StatusOK, impact)
}
func (h *Handler) validateArchivedDelete(r *http.Request, id string) error {
	item, err := h.agentEngine.Agents().Get(r.Context(), id, agentengine.AgentGetOptions{})
	if err != nil {
		return &agentengine.TurnError{Code: agentengine.ErrorAgentNotFound, Message: "agent not found"}
	}
	if item.ArchivedAt == nil {
		return fmt.Errorf("archive the agent before deleting it")
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Name != item.Spec.Name {
		return fmt.Errorf("type the exact agent name to confirm deletion")
	}
	return nil
}

// Read-only configuration/history remains available. All runtime and model
// proxy calls are blocked, including callers using the service access token.
func (h *Handler) guardArchivedAgent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.agentLifecycleState(pathValue(r, "id")) != "archived" {
			next.ServeHTTP(w, r)
			return
		}
		path := strings.TrimSuffix(r.URL.Path, "/")
		if strings.HasSuffix(path, "/archive") || strings.HasSuffix(path, "/restore") || (r.Method == http.MethodDelete && !strings.Contains(strings.TrimPrefix(path, "/api/v1/agents/"), "/")) {
			next.ServeHTTP(w, r)
			return
		}
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && !strings.Contains(path, "/llm/") && !strings.HasSuffix(path, "/mcp") && !strings.Contains(path, "/mcp-file-bridge/") {
			next.ServeHTTP(w, r)
			return
		}
		writeCodedAPIError(w, http.StatusConflict, "agent_archived", "Restore the archived agent before using it")
	})
}

func (h *Handler) reconcileArchivedSchedules(svc *scheduledtask.Service) {
	for _, item := range svc.List() {
		if item.Enabled && h.agentLifecycleState(item.AgentID) == "archived" {
			if err := svc.DisableAgent(item.AgentID); err != nil {
				slog.Warn("pause archived agent schedules", "agent_id", item.AgentID, "error", err)
			}
		}
	}
}
