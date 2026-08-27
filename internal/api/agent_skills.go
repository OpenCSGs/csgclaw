package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/agentengine"
)

type batchAddAgentSkillsRequest struct {
	Names []string `json:"names"`
}

func (h *Handler) handleAgentSkillsBatchAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.agentEngine == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimSpace(pathValue(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	var req batchAddAgentSkillsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode request: "+err.Error(), http.StatusBadRequest)
		return
	}
	agents := h.agentEngine.Agents()
	current, err := agents.Get(r.Context(), id, agentengine.AgentGetOptions{})
	if err != nil {
		writeAgentSkillsMutationError(w, err)
		return
	}
	for _, rawName := range req.Names {
		name := strings.TrimSpace(rawName)
		if slices.Contains(current.Spec.Skills, name) {
			writeAgentSkillsMutationError(w, fmt.Errorf("%w: %s", agent.ErrAgentSkillAlreadyExists, name))
			return
		}
		current.Spec.Skills = append(current.Spec.Skills, name)
	}
	if _, err := agents.Update(r.Context(), id, agentengine.AgentUpdateRequest{Spec: current.Spec, FieldMask: []string{"skills"}, ResourceVersion: current.ResourceVersion}); err != nil {
		writeAgentSkillsMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleAgentSkillDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.agentEngine == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimSpace(pathValue(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}
	agents := h.agentEngine.Agents()
	current, err := agents.Get(r.Context(), id, agentengine.AgentGetOptions{})
	if err != nil {
		writeAgentSkillsMutationError(w, err)
		return
	}
	name := strings.TrimSpace(pathValue(r, "name"))
	index := slices.Index(current.Spec.Skills, name)
	if index < 0 {
		writeAgentSkillsMutationError(w, fmt.Errorf("%w: %s", os.ErrNotExist, name))
		return
	}
	current.Spec.Skills = slices.Delete(current.Spec.Skills, index, index+1)
	if _, err := agents.Update(r.Context(), id, agentengine.AgentUpdateRequest{Spec: current.Spec, FieldMask: []string{"skills"}, ResourceVersion: current.ResourceVersion}); err != nil {
		writeAgentSkillsMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeAgentSkillsMutationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, os.ErrNotExist), errors.Is(err, errAgentWorkspaceNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, agent.ErrAgentSkillAlreadyExists):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, agent.ErrAgentSkillInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
