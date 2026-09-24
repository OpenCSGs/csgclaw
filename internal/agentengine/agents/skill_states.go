package agents

import (
	"context"
	"csgclaw/internal/agentengine/contract"
	agentruntime "csgclaw/internal/runtime"
	skilllocal "csgclaw/internal/skill/local"
	skill "csgclaw/internal/skill/state"
	"errors"
	"fmt"
	"time"
)

type deferResourceRestartKey struct{}

var ErrSkillEnablementUnsupported = errors.New("runtime does not support skill enablement")
var ErrAgentResourceVersionConflict = &contract.TurnError{Code: contract.ErrorInvalidRequest, Message: "agent resource version is stale"}

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
