package agents

import (
	"context"
	"csgclaw/internal/agentengine/contract"
	agentruntime "csgclaw/internal/runtime"
	skilllocal "csgclaw/internal/skill/local"
	skill "csgclaw/internal/skill/state"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type deferResourceRestartKey struct{}

var ErrSkillEnablementUnsupported = errors.New("runtime does not support skill enablement")
var ErrAgentResourceVersionConflict = &contract.TurnError{Code: contract.ErrorInvalidRequest, Message: "agent resource version is stale"}

func (s *Controller) ensureSkillEnabled(ctx context.Context, id, name string) error {
	if clean, err := skilllocal.NormalizeName(name); err != nil || clean != name || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) {
		return ErrAgentSkillInvalid
	}
	id = canonicalAgentID(id)
	return s.LifecycleCoordinator().TryUpdateFiles(ctx, id, func() error {
		a, ok := s.agentSnapshot(id)
		if !ok || a.ArchivedAt != nil {
			return os.ErrNotExist
		}
		if a.Role != RoleWorker || !isHostRuntimeKind(a.RuntimeKind) {
			return ErrSkillEnablementUnsupported
		}
		root, err := s.agentSkillsRoot(id, a.RuntimeKind)
		if err != nil {
			return err
		}
		dir, err := os.Lstat(filepath.Join(root, name))
		if err != nil {
			return err
		}
		if !dir.IsDir() {
			return ErrAgentSkillInvalid
		}
		if _, err := skilllocal.ResolveDir(root, name); err != nil {
			return err
		}
		rt, err := s.runtimeForKind(a.RuntimeKind)
		if err != nil {
			return err
		}
		reconciler, ok := rt.(agentruntime.SkillsReconciler)
		if !ok {
			return ErrSkillEnablementUnsupported
		}
		current, err := s.enableInstalledSkill(id, name)
		if err != nil {
			return err
		}
		// Even a default-enabled skill may have new files after a reinstall.
		return reconciler.ReconcileSkills(ctx, runtimeHandleForAgent(current), current.SkillStates)
	})
}

func (s *Controller) enableInstalledSkill(id, name string) (Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, key, ok := s.agentByIDLocked(id)
	if !ok {
		return Agent{}, os.ErrNotExist
	}
	if skill.Enabled(current.SkillStates, name) {
		return current, nil
	}
	previous := current
	current.SkillStates = maps.Clone(current.SkillStates)
	delete(current.SkillStates, name) // absence is the default enabled state
	current.UpdatedAt = time.Now().UTC()
	s.putAgentLocked(key, current)
	if err := s.saveLocked(); err != nil {
		s.putAgentLocked(key, previous)
		return Agent{}, err
	}
	return current, nil
}

func validateSkillStates(kind string, states map[string]skill.State) error {
	if len(states) == 0 {
		return nil
	}
	if !isHostRuntimeKind(kind) {
		return ErrSkillEnablementUnsupported
	}
	for name := range states {
		normalized, err := skilllocal.NormalizeName(name)
		if err != nil || normalized != name {
			return fmt.Errorf("%w: %s", ErrAgentSkillInvalid, name)
		}
	}
	return nil
}

func (s *Controller) markResourceRestart(id string) (Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, key, ok := s.agentByIDLocked(id)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	previous := current
	current.AgentProfile.EnvRestartRequired = true
	current.UpdatedAt = time.Now().UTC()
	s.putAgentLocked(key, current)
	if err := s.saveLocked(); err != nil {
		s.putAgentLocked(key, previous)
		return Agent{}, err
	}
	return current, nil
}

func (s *Controller) reconcileSkillStates(ctx context.Context, current Agent) error {
	rt, err := s.runtimeForKind(current.RuntimeKind)
	if err != nil {
		return err
	}
	reconciler, ok := rt.(agentruntime.SkillsReconciler)
	if !ok {
		if len(current.SkillStates) == 0 {
			return nil
		}
		return ErrSkillEnablementUnsupported
	}
	return reconciler.ReconcileSkills(ctx, runtimeHandleForAgent(current), current.SkillStates)
}
