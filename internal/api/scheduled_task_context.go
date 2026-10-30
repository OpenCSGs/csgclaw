package api

import (
	"csgclaw/internal/agentengine"
	"net/http"
	"time"
)

func (h *Handler) handleScheduledTaskContext(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireScheduledTaskService(w); !ok {
		return
	}
	type executor struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Role string `json:"role"`
	}
	agents := make([]executor, 0)
	if h.agentEngine != nil {
		items, err := h.agentEngine.Agents().List(r.Context(), agentengine.AgentListOptions{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, item := range items {
			agents = append(agents, executor{ID: item.ID, Name: item.Spec.Name, Role: string(item.Spec.Role)})
		}
	}
	now := time.Now()
	writeJSON(w, http.StatusOK, map[string]any{"now": now, "timezone": time.Local.String(), "recurrences": []string{"once", "daily", "weekly", "monthly"}, "agents": agents})
}
