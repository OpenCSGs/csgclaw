package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentruntime "csgclaw/internal/runtime"
)

// restartCodexRuntime restarts the in-process Codex app-server without deleting
// its runtime directory. In particular, persisted Codex threads and conversation
// mappings survive profile changes such as reasoning effort updates.
func (s *Service) restartCodexRuntime(ctx context.Context, id string) (Agent, error) {
	ctx, release, err := s.acquireAgentLifecycle(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	defer release()
	return s.restartCodexRuntimeLocked(ctx, id)
}

func (s *Service) restartCodexRuntimeLocked(ctx context.Context, id string) (Agent, error) {
	got, ok := s.Agent(id)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", strings.TrimSpace(id))
	}
	if !strings.EqualFold(strings.TrimSpace(got.RuntimeKind), RuntimeKindCodex) {
		return Agent{}, fmt.Errorf("agent %q is not a codex agent", got.ID)
	}

	runtimeImpl, err := s.runtimeForKind(RuntimeKindCodex)
	if err != nil {
		return Agent{}, err
	}
	handle := runtimeHandleForAgent(got)
	s.stopLifecycleAgent(got.ID)
	if _, err := runtimeImpl.Stop(ctx, handle); err != nil {
		return Agent{}, fmt.Errorf("stop codex agent for restart: %w", err)
	}
	if _, err := s.updateRuntimeState(got.ID, agentruntime.Info{
		HandleID: strings.TrimSpace(got.BoxID),
		State:    agentruntime.StateStopped,
	}); err != nil {
		return Agent{}, fmt.Errorf("save stopped codex agent state: %w", err)
	}
	if err := s.provisionRuntimeForAgent(ctx, runtimeImpl, got, ""); err != nil {
		return Agent{}, fmt.Errorf("provision codex agent for restart: %w", err)
	}
	state, err := runtimeImpl.Start(ctx, handle)
	if err != nil {
		return Agent{}, fmt.Errorf("start codex agent after settings update: %w", err)
	}
	info, err := s.runtimeInfo(ctx, runtimeImpl, handle)
	if err != nil {
		return Agent{}, fmt.Errorf("read codex agent state after restart: %w", err)
	}
	if info.State == "" {
		info.State = state
	}

	s.mu.Lock()
	current, key, ok := s.agentByIDLocked(id)
	if !ok {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("agent %q not found", strings.TrimSpace(id))
	}
	if handleID := strings.TrimSpace(info.HandleID); handleID != "" {
		current.BoxID = handleID
	}
	current.Status = string(info.State)
	current.AgentProfile.EnvRestartRequired = false
	current.UpdatedAt = time.Now().UTC()
	s.agents[key] = current
	s.syncRuntimeRecordLocked(current)
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return Agent{}, err
	}

	restarted, ok := s.Agent(id)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", strings.TrimSpace(id))
	}
	if err := s.syncLifecycleForAgent(ctx, restarted); err != nil {
		return Agent{}, err
	}
	return restarted, nil
}

type LifecycleObserver interface {
	EnsureAgent(context.Context, Agent) error
	StopAgent(string)
}

type BindingActivator interface {
	RefreshAgentChannel(context.Context, Agent, string) error
}

type ExternalBindingActivation string

const (
	ExternalBindingActivationChannelRefreshed ExternalBindingActivation = "channel_refreshed"
	ExternalBindingActivationRuntimeRecreated ExternalBindingActivation = "runtime_recreated"
)

func (s *Service) lifecycleObserver() LifecycleObserver {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lifecycle
}

func (s *Service) bindingActivator() BindingActivator {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bindingActivation
}

func (s *Service) syncLifecycleForAgent(ctx context.Context, a Agent) error {
	observer := s.lifecycleObserver()
	if observer == nil {
		return nil
	}
	if shouldEnsureLifecycle(a) {
		return observer.EnsureAgent(ctx, a)
	}
	observer.StopAgent(a.ID)
	return nil
}

// ApplyExternalBinding activates an updated external binding using the
// lifecycle required by the agent's runtime.
func (s *Service) ApplyExternalBinding(ctx context.Context, id, channel string) (Agent, ExternalBindingActivation, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, "", fmt.Errorf("agent id is required")
	}
	channel = strings.ToLower(strings.TrimSpace(channel))
	if channel == "" {
		return Agent{}, "", fmt.Errorf("channel is required")
	}
	got, ok := s.Agent(id)
	if !ok {
		return Agent{}, "", fmt.Errorf("agent %q not found", id)
	}
	if strings.EqualFold(strings.TrimSpace(got.RuntimeKind), RuntimeKindCodex) {
		if !shouldEnsureLifecycle(got) {
			return Agent{}, "", fmt.Errorf("agent %q must be running with a complete profile to refresh external bindings", got.ID)
		}
		activator := s.bindingActivator()
		if activator == nil {
			return Agent{}, "", fmt.Errorf("agent binding activator is not configured")
		}
		if err := activator.RefreshAgentChannel(ctx, got, channel); err != nil {
			return Agent{}, "", err
		}
		return got, ExternalBindingActivationChannelRefreshed, nil
	}
	recreated, err := s.Recreate(ctx, got.ID)
	return recreated, ExternalBindingActivationRuntimeRecreated, err
}

// DeactivateExternalBinding refreshes runtime-side channel state after an
// external participant binding has been removed.
func (s *Service) DeactivateExternalBinding(ctx context.Context, id, channel string) (Agent, ExternalBindingActivation, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, "", fmt.Errorf("agent id is required")
	}
	channel = strings.ToLower(strings.TrimSpace(channel))
	if channel == "" {
		return Agent{}, "", fmt.Errorf("channel is required")
	}
	got, ok := s.Agent(id)
	if !ok {
		return Agent{}, "", fmt.Errorf("agent %q not found", id)
	}
	if strings.EqualFold(strings.TrimSpace(got.RuntimeKind), RuntimeKindCodex) {
		activator := s.bindingActivator()
		if activator == nil {
			return Agent{}, "", fmt.Errorf("agent binding activator is not configured")
		}
		if err := activator.RefreshAgentChannel(ctx, got, channel); err != nil {
			return Agent{}, "", err
		}
		return got, ExternalBindingActivationChannelRefreshed, nil
	}
	recreated, err := s.Recreate(ctx, got.ID)
	return recreated, ExternalBindingActivationRuntimeRecreated, err
}

func (s *Service) stopLifecycleAgent(agentID string) {
	observer := s.lifecycleObserver()
	if observer == nil {
		return
	}
	observer.StopAgent(strings.TrimSpace(agentID))
}

func shouldEnsureLifecycle(a Agent) bool {
	return isAgentProfileComplete(a) &&
		strings.EqualFold(strings.TrimSpace(a.Status), string(agentruntime.StateRunning))
}
