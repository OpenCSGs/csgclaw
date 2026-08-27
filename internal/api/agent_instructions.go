package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/agentengine"
)

func (h *Handler) getAgentInstructions(w http.ResponseWriter, r *http.Request) {
	if h.agentEngine == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	current, err := h.agentEngine.Agents().Get(r.Context(), pathValue(r, "id"), agentengine.AgentGetOptions{IncludeDocuments: true})
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if current.Status.Instructions == nil || current.Status.Instructions.Error != "" {
		message := "agent instructions are unavailable"
		if current.Status.Instructions != nil {
			message = current.Status.Instructions.Error
		}
		http.Error(w, message, http.StatusNotFound)
		return
	}
	document := agent.InstructionsDocument{Instructions: current.Spec.Instructions, Effective: current.Status.Instructions.Effective}
	writeJSON(w, http.StatusOK, document)
}

func (h *Handler) putAgentInstructions(w http.ResponseWriter, r *http.Request) {
	if h.agentEngine == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	var request struct {
		Effective string `json:"effective"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(request.Effective) == "" {
		http.Error(w, "effective instructions are required", http.StatusBadRequest)
		return
	}
	agents := h.agentEngine.Agents()
	current, err := agents.Get(r.Context(), pathValue(r, "id"), agentengine.AgentGetOptions{IncludeDocuments: true})
	if err == nil {
		current.Spec.Instructions = agent.ExtractUserInstructionsFromAgentsDocument(request.Effective)
		_, err = agents.Update(r.Context(), current.ID, agentengine.AgentUpdateRequest{
			Spec: current.Spec, FieldMask: []string{"instructions"}, ResourceVersion: current.ResourceVersion,
		})
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	current, err = agents.Get(r.Context(), current.ID, agentengine.AgentGetOptions{IncludeDocuments: true})
	if err != nil || current.Status.Instructions == nil || current.Status.Instructions.Error != "" {
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else if current.Status.Instructions != nil {
			http.Error(w, current.Status.Instructions.Error, http.StatusBadRequest)
		} else {
			http.Error(w, "agent instructions are unavailable", http.StatusBadRequest)
		}
		return
	}
	document := agent.InstructionsDocument{Instructions: current.Spec.Instructions, Effective: current.Status.Instructions.Effective}
	writeJSON(w, http.StatusOK, document)
}
