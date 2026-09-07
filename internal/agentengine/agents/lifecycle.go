package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentruntime "csgclaw/internal/runtime"
)

// restartRuntime restarts the in-process Codex app-server without deleting
// its runtime directory. In particular, persisted Codex threads and conversation
// mappings survive profile changes such as reasoning effort updates.
func (s *Controller) restartRuntime(ctx context.Context, id string) (Agent, error) {
	ctx, release, err := s.acquireAgentLifecycle(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	defer release()
	return s.restartRuntimeLocked(ctx, id)
}

// RestartRuntimeIfRunning refreshes a live Codex app-server without
// forcing stopped workers to start.
func (s *Controller) RestartRuntimeIfRunning(ctx context.Context, id string) (Agent, bool, error) {
	if s == nil {
		return Agent{}, false, fmt.Errorf("agent service is required")
	}
	ctx, release, err := s.acquireAgentLifecycle(ctx, id)
	if err != nil {
		return Agent{}, false, err
	}
	defer release()

	got, ok := s.Agent(id)
	if !ok {
		return Agent{}, false, fmt.Errorf("agent %q not found", strings.TrimSpace(id))
	}
	if !isRuntimeRunning(got) {
		return got, false, nil
	}
	restarted, err := s.restartRuntimeLocked(ctx, id)
	if err != nil {
		return Agent{}, false, err
	}
	return restarted, true, nil
}

func (s *Controller) restartRuntimeLocked(ctx context.Context, id string) (Agent, error) {
	got, ok := s.Agent(id)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", strings.TrimSpace(id))
	}

	runtimeImpl, err := s.runtimeForKind(got.RuntimeKind)
	if err != nil {
		return Agent{}, err
	}
	handle := runtimeHandleForAgent(got)
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
	if err := s.observeStoppedExtensions(ctx, id); err != nil {
		return Agent{}, err
	}
	if err := s.prepareExtensions(ctx, id); err != nil {
		return Agent{}, err
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
	s.putAgentLocked(key, current)
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
	if err := s.observeStartedExtensions(ctx, id); err != nil {
		return Agent{}, err
	}
	return restarted, nil
}
