package api

import (
	"csgclaw/internal/apitypes"
	"csgclaw/internal/taskconfig"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateCollaborationConfiguration(room apitypes.Room, configuration *taskconfig.Config) (*taskconfig.Config, error) {
	if !room.IsOnDemand() {
		return nil, fmt.Errorf("workspace configuration requires a collaboration room")
	}
	cfg := *configuration
	if cfg.ModelID != "" || cfg.ModelProviderID != "" || cfg.ReasoningEffort != "" || cfg.ExecutionMode != "" {
		return nil, fmt.Errorf("collaboration rooms share a working directory; each agent keeps its own model and permissions")
	}
	cfg.WorkspaceDir = strings.TrimSpace(cfg.WorkspaceDir)
	if cfg.WorkspaceDir != "" {
		if !filepath.IsAbs(cfg.WorkspaceDir) {
			return nil, fmt.Errorf("working directory must be absolute")
		}
		info, err := os.Stat(cfg.WorkspaceDir)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("working directory is unavailable")
		}
		cfg.WorkspaceDir = filepath.Clean(cfg.WorkspaceDir)
	}
	if room.Configuration != nil && cfg.WorkspaceDir != room.Configuration.WorkspaceDir {
		return nil, fmt.Errorf("working directory is locked; start a new collaboration")
	}
	return &cfg, nil
}

func roomWorkspaceDir(room apitypes.Room) string {
	if room.Configuration == nil {
		return ""
	}
	return room.Configuration.WorkspaceDir
}

func (h *Handler) applyRoomTeamGuide(req *apitypes.CreateRoomRequest) error {
	req.SourceTeamID = strings.TrimSpace(req.SourceTeamID)
	if req.SourceTeamID == "" {
		return nil
	}
	if req.IsDirect || apitypes.NormalizeRoomType(req.Type) != apitypes.RoomTypeOnDemand {
		return fmt.Errorf("team guides require a collaboration room")
	}
	if h.im != nil {
		if saved, ok := h.im.RoomForCreationRequest(req.CreatorID, req.ClientRequestID); ok {
			if saved.SourceTeamID != req.SourceTeamID {
				return fmt.Errorf("client_request_id already belongs to another conversation")
			}
			req.CollaborationGuide = saved.CollaborationGuide
			return nil
		}
	}
	if h.teamSvc == nil {
		return fmt.Errorf("team service is unavailable")
	}
	meta, ok := h.teamSvc.GetTeam(req.SourceTeamID)
	if !ok {
		return fmt.Errorf("team not found")
	}
	req.CollaborationGuide = meta.CollaborationGuide
	return nil
}
