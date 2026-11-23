package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/mcpschema"
	skill "csgclaw/internal/skill/state"
)

type agentResourceEnabledRequest struct {
	Enabled         *bool  `json:"enabled"`
	ResourceVersion string `json:"resource_version"`
}

type agentResourceEnabledResponse struct {
	AgentID         string                 `json:"agent_id"`
	ResourceVersion string                 `json:"resource_version"`
	Name            string                 `json:"name"`
	Enabled         bool                   `json:"enabled"`
	RuntimeKind     string                 `json:"runtime_kind"`
	RuntimeState    agentengine.AgentState `json:"runtime_state"`
	RestartRequired bool                   `json:"restart_required"`
}

func (h *Handler) handleAgentSkillEnabled(w http.ResponseWriter, r *http.Request) {
	h.handleAgentResourceEnabled(w, r, true)
}

func (h *Handler) handleAgentMCPServerEnabled(w http.ResponseWriter, r *http.Request) {
	h.handleAgentResourceEnabled(w, r, false)
}

func (h *Handler) handleAgentResourceEnabled(w http.ResponseWriter, r *http.Request, isSkill bool) {
	if h.agentEngine == nil {
		writeCodedAPIError(w, http.StatusServiceUnavailable, "agent_unavailable", "Agent service is unavailable")
		return
	}
	var req agentResourceEnabledRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || req.Enabled == nil || strings.TrimSpace(req.ResourceVersion) == "" {
		writeCodedAPIError(w, http.StatusBadRequest, "invalid_request", "enabled and resource_version are required")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeCodedAPIError(w, http.StatusBadRequest, "invalid_request", "request must contain one JSON object")
		return
	}
	id, name := pathValue(r, "id"), pathValue(r, "name")
	agents := h.agentEngine.Agents()
	current, err := agents.Get(r.Context(), id, agentengine.AgentGetOptions{AdoptMCPServers: !isSkill})
	if err != nil {
		writeAgentGetError(w, err)
		return
	}
	if current.Status.RuntimeKind != agent.RuntimeKindCodex && current.Status.RuntimeKind != agent.RuntimeKindDSH {
		writeCodedAPIError(w, http.StatusUnprocessableEntity, "runtime_capability_unsupported", "Runtime does not support resource enablement")
		return
	}
	field := "mcp_servers"
	if isSkill {
		if !slices.Contains(current.Spec.Skills, name) {
			writeCodedAPIError(w, http.StatusNotFound, "skill_not_found", "Skill is not installed")
			return
		}
		if current.Spec.SkillStates == nil {
			current.Spec.SkillStates = make(map[string]skill.State)
		}
		if skill.Enabled(current.Spec.SkillStates, name) != *req.Enabled {
			current.Spec.SkillStates[name] = skill.State{Enabled: *req.Enabled}
		}
		field = "skill_states"
	} else {
		server, ok := current.Spec.MCPServers[name]
		if !ok {
			writeCodedAPIError(w, http.StatusNotFound, "mcp_server_not_found", "MCP server is not installed")
			return
		}
		if mcpschema.ServerEnabled(server) != *req.Enabled {
			server["enabled"] = *req.Enabled
		}
	}
	updated, err := agents.Update(r.Context(), current.ID, agentengine.AgentUpdateRequest{Spec: current.Spec, FieldMask: []string{field}, ResourceVersion: req.ResourceVersion})
	if err != nil {
		switch {
		case errors.Is(err, agent.ErrAgentResourceVersionConflict):
			writeCodedAPIError(w, http.StatusConflict, "resource_version_conflict", err.Error())
		case errors.Is(err, agent.ErrSkillEnablementUnsupported):
			writeCodedAPIError(w, http.StatusUnprocessableEntity, "runtime_capability_unsupported", err.Error())
		case errors.Is(err, agent.ErrAgentSkillInvalid):
			writeCodedAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
		default:
			writeCodedAPIError(w, http.StatusInternalServerError, "runtime_config_apply_failed", err.Error())
		}
		return
	}
	enabled := skill.Enabled(updated.Spec.SkillStates, name)
	if !isSkill {
		enabled = mcpschema.ServerEnabled(updated.Spec.MCPServers[name])
	}
	h.publishUpdatedAgentUser(serviceAgentFromEngine(updated))
	w.Header().Set("ETag", strconv.Quote(updated.ResourceVersion))
	writeJSON(w, http.StatusOK, agentResourceEnabledResponse{
		AgentID: updated.ID, ResourceVersion: updated.ResourceVersion, Name: name, Enabled: enabled,
		RuntimeKind: updated.Status.RuntimeKind, RuntimeState: updated.Status.State, RestartRequired: updated.Status.Model.EnvRestartRequired,
	})
}
