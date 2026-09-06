package api

import (
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"encoding/json"
	"net/http"
)

func (h *Handler) getAgentMemory(w http.ResponseWriter, r *http.Request) {
	if h.agentEngine == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	current, err := h.agentEngine.Agents().Get(r.Context(), pathValue(r, "id"), agentengine.AgentGetOptions{IncludeDocuments: true})
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if current.Status.Memory == nil || current.Status.Memory.Error != "" {
		message := "agent memory is unavailable"
		if current.Status.Memory != nil {
			message = current.Status.Memory.Error
		}
		http.Error(w, message, http.StatusNotFound)
		return
	}
	document := memoryDocumentFromEngine(current.Status.Memory)
	writeJSON(w, http.StatusOK, document)
}

func (h *Handler) putAgentMemory(w http.ResponseWriter, r *http.Request) {
	if h.agentEngine == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	var request struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if request.Enabled == nil {
		http.Error(w, "enabled is required", http.StatusBadRequest)
		return
	}
	agents := h.agentEngine.Agents()
	current, err := agents.Get(r.Context(), pathValue(r, "id"), agentengine.AgentGetOptions{IncludeDocuments: true})
	if err == nil {
		current.Spec.Memory = &agentengine.MemorySpec{Enabled: *request.Enabled}
		_, err = agents.Update(r.Context(), current.ID, agentengine.AgentUpdateRequest{
			Spec: current.Spec, FieldMask: []string{"memory"}, ResourceVersion: current.ResourceVersion,
		})
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	current, err = agents.Get(r.Context(), current.ID, agentengine.AgentGetOptions{IncludeDocuments: true})
	if err != nil || current.Status.Memory == nil || current.Status.Memory.Error != "" {
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else if current.Status.Memory != nil {
			http.Error(w, current.Status.Memory.Error, http.StatusBadRequest)
		} else {
			http.Error(w, "agent memory is unavailable", http.StatusBadRequest)
		}
		return
	}
	document := memoryDocumentFromEngine(current.Status.Memory)
	writeJSON(w, http.StatusOK, document)
}

func memoryDocumentFromEngine(status *agentengine.MemoryStatus) agent.MemoryDocument {
	return agent.MemoryDocument{
		Enabled: status.Enabled, Ready: status.Ready, Name: status.Name,
		Location: status.Location, Content: status.Content,
	}
}
