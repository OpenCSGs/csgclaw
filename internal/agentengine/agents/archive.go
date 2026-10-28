package agents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"csgclaw/internal/agentengine/contract"
)

func archivedAgentError() error {
	return &contract.TurnError{Code: contract.ErrorAgentUnavailable, Message: "agent is archived; restore it before use"}
}

// SetArchived retains configuration and relationships. The execution gate closes
// before cancellation, so queued turns cannot slip through while archiving.
func (s *Controller) SetArchived(ctx context.Context, id string, archived bool) (contract.Agent, error) {
	id = canonicalAgentID(id)
	selected, ok := s.Agent(id)
	if !ok {
		return contract.Agent{}, fmt.Errorf("agent %q not found", id)
	}
	if isManagerAgent(selected) {
		return contract.Agent{}, fmt.Errorf("manager cannot be archived")
	}
	interrupt := func() {
		if archived && s.interruptAgent != nil {
			s.interruptAgent(id)
		}
	}
	ctx, release, err := s.LifecycleCoordinator().MutationInterrupt(ctx, id, interrupt)
	if err != nil {
		return contract.Agent{}, err
	}
	defer release()
	selected, ok = s.Agent(id)
	if !ok {
		return contract.Agent{}, fmt.Errorf("agent %q not found", id)
	}
	if archived {
		if err := s.persistArchive(id, true); err != nil {
			return contract.Agent{}, err
		}
		// Keep the durable disabled flag on a stop failure; retrying archive repairs
		// the stop, and a restart must never revive the archived runtime.
		if _, err := s.Stop(ctx, id); err != nil {
			return contract.Agent{}, err
		}
	} else if selected.ArchivedAt != nil {
		if err := s.persistArchive(id, false); err != nil {
			return contract.Agent{}, err
		}
		if _, err := s.Start(ctx, id); err != nil {
			if rollbackErr := s.persistArchive(id, true); rollbackErr != nil {
				return contract.Agent{}, fmt.Errorf("restore: %v; retain archive: %w", err, rollbackErr)
			}
			return contract.Agent{}, err
		}
		if _, err := s.SetDesiredState(id, DesiredStateRunning, false); err != nil {
			return contract.Agent{}, err
		}
	}
	selected, _ = s.Agent(id)
	return s.convert(selected)
}

func (s *Controller) persistArchive(id string, archived bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, key, ok := s.agentByIDLocked(id)
	if !ok {
		return fmt.Errorf("agent %q not found", id)
	}
	previous := current
	now := time.Now().UTC()
	if archived {
		if current.ArchivedAt == nil {
			current.ArchivedAt = &now
		}
		current.DesiredState = DesiredStateStopped
	} else {
		current.ArchivedAt = nil
	}
	current.UpdatedAt = now
	s.putAgentLocked(key, current)
	if err := s.saveLocked(); err != nil {
		s.putAgentLocked(key, previous)
		return err
	}
	return nil
}

// preserveDeletedWorkspace moves only user working files out of the runtime
// deletion path. Credentials, private memory and runtime state are not retained.
func (s *Controller) preserveDeletedWorkspace(selected Agent) (func() error, error) {
	root, err := s.WorkspaceRootByID(selected.ID)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return func() error { return nil }, nil
	} else if err != nil {
		return nil, err
	}
	home, err := s.agentHomeDir(selected.ID)
	if err != nil {
		return nil, err
	}
	backup, err := os.MkdirTemp(filepath.Dir(home), ".deleted-agent-workspace-")
	if err != nil {
		return nil, err
	}
	saved := filepath.Join(backup, "workspace")
	if err := os.Rename(root, saved); err != nil {
		_ = os.Remove(backup)
		return nil, err
	}
	return func() error {
		if err := os.MkdirAll(filepath.Dir(root), 0700); err != nil {
			return fmt.Errorf("workspace retained at %s: %w", saved, err)
		}
		// Runtime deletion may recreate an empty default workspace. Never remove a
		// nonempty directory when restoring the user's files.
		if _, err := os.Lstat(root); err == nil {
			if err := os.Remove(root); err != nil {
				return fmt.Errorf("workspace retained at %s: %w", saved, err)
			}
		}
		if err := os.Rename(saved, root); err != nil {
			return fmt.Errorf("workspace retained at %s: %w", saved, err)
		}
		return os.Remove(backup)
	}, nil
}
