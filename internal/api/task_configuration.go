package api

import (
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/llm"
	"csgclaw/internal/taskconfig"
	"fmt"
	"net/http"
	"strings"
)

type taskModelService interface {
	ValidateTaskConfiguration(string, taskconfig.Config) (taskconfig.Config, error)
	ResolvedTaskProfile(string, taskconfig.Config) (agent.AgentProfile, error)
}

func (h *Handler) taskAgent(members []string) (agent.Agent, error) {
	if h.svc != nil {
		for _, id := range members {
			target := h.participantBridgeTargetForRoomMember(id)
			for _, alias := range target.aliases {
				if got, ok := h.svc.Agent(alias); ok {
					return got, nil
				}
			}
			if got, ok := h.svc.Agent(id); ok {
				return got, nil
			}
		}
	}
	return agent.Agent{}, fmt.Errorf("task agent is unavailable")
}

func (h *Handler) validateTaskConfiguration(room apitypes.Room, configuration *taskconfig.Config) (*taskconfig.Config, error) {
	if configuration == nil {
		return nil, nil
	}
	if !room.IsDirect {
		return validateCollaborationConfiguration(room, configuration)
	}
	models, ok := h.agentModels.(taskModelService)
	if !ok {
		return nil, fmt.Errorf("session configuration is unavailable")
	}
	got, err := h.taskAgent(room.Members)
	if err != nil {
		return nil, err
	}
	cfg := *configuration
	if room.Configuration != nil && cfg.WorkspaceDir != room.Configuration.WorkspaceDir {
		return nil, fmt.Errorf("working directory is locked; start a new task")
	}
	if cfg.WorkspaceDir == "" && room.Configuration == nil && h.workspace != nil {
		cfg.WorkspaceDir, _ = got.RuntimeOptions["local_workspace_dir"].(string)
		if cfg.WorkspaceDir == "" {
			cfg.WorkspaceDir, err = h.workspace.WorkspaceRootByID(got.ID)
		}
		if err != nil {
			return nil, err
		}
	}
	cfg, err = models.ValidateTaskConfiguration(got.ID, cfg)
	return &cfg, err
}

func (h *Handler) taskLLMContext(w http.ResponseWriter, r *http.Request, agentID string) (*http.Request, bool) {
	roomID := strings.TrimSpace(pathValue(r, "task_id"))
	if roomID == "" {
		return r, true
	}
	if h.im == nil {
		http.NotFound(w, r)
		return r, false
	}
	room, ok := h.im.Room(roomID)
	if !ok || room.Configuration == nil {
		http.NotFound(w, r)
		return r, false
	}
	authorized := false
	for _, member := range room.Members {
		if h.participantBridgeTargetForRoomMember(member).matches(agentID) {
			authorized = true
			break
		}
	}
	if !authorized {
		http.Error(w, "agent is not a member of this task", http.StatusForbidden)
		return r, false
	}
	models, ok := h.agentModels.(taskModelService)
	if !ok {
		http.Error(w, "session configuration is unavailable", http.StatusServiceUnavailable)
		return r, false
	}
	profile, err := models.ResolvedTaskProfile(agentID, *room.Configuration)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return r, false
	}
	return r.WithContext(llm.WithTaskProfile(r.Context(), profile)), true
}
