package im

import (
	"csgclaw/internal/taskconfig"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func requestedWorkspaceDir(cfg *taskconfig.Config) string {
	if cfg == nil || strings.TrimSpace(cfg.WorkspaceDir) == "" {
		return ""
	}
	return filepath.Clean(strings.TrimSpace(cfg.WorkspaceDir))
}

func sameCollaborationCreation(room Room, req CreateRoomRequest) bool {
	if room.SourceTeamID != req.SourceTeamID {
		return false
	}
	if !room.IsOnDemand() {
		return true
	}
	requested := requestedWorkspaceDir(req.Configuration)
	if room.CreationWorkspaceDir != nil {
		return *room.CreationWorkspaceDir == requested
	}
	// Older rooms did not record automatic vs explicit selection. Preserve
	// empty retries, but never silently accept a different explicit directory.
	return requested == "" || requested == requestedWorkspaceDir(room.Configuration)
}

// RoomForCreationRequest finds a saved result without consulting a mutable
// source Team. CreateRoom still validates the rest of the retry request.
func (s *Service) RoomForCreationRequest(creatorID, requestID string) (Room, bool) {
	if requestID == "" {
		return Room{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	creatorID = s.resolveRoomUserIDLocked(creatorID)
	for _, room := range s.rooms {
		if room.CreatedBy == creatorID && room.ClientRequestID == requestID {
			return cloneRoom(*room), true
		}
	}
	return Room{}, false
}

// Shared outputs belong to the room, not to any member's private workspace.
// Keep these files when deleting the conversation or removing a member.
// Only provision directories at creation; loading legacy rooms must preserve
// their existing per-agent workspaces and saved configuration.
func (s *Service) ensureRoomWorkspace(room *Room) error {
	if !room.IsOnDemand() || s.statePath == "" {
		return nil
	}
	if room.Configuration != nil && room.Configuration.WorkspaceDir != "" {
		return nil
	}
	if filepath.Base(room.ID) != room.ID || room.ID == "." || room.ID == ".." {
		return fmt.Errorf("invalid room ID")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(s.statePath), "workspaces", room.ID))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return fmt.Errorf("create collaboration workspace: %w", err)
	}
	room.Configuration = &taskconfig.Config{WorkspaceDir: root}
	return nil
}
