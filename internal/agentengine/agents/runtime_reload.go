package agents

import (
	"context"
	agentruntime "csgclaw/internal/runtime"
	"fmt"
)

// ReloadRuntimeIfRunning loads already-activated incremental projections. Unlike
// base provisioning, it never reapplies Credentials or runs the user's InitShell.
func (s *Controller) ReloadRuntimeIfRunning(ctx context.Context, id string) (Agent, bool, error) {
	ctx, release, err := s.acquireAgentLifecycle(ctx, id)
	if err != nil {
		return Agent{}, false, err
	}
	defer release()
	selected, ok := s.Agent(id)
	if !ok {
		return Agent{}, false, fmt.Errorf("agent %q not found", id)
	}
	if !isRuntimeRunning(selected) {
		return selected, false, nil
	}
	rt, err := s.runtimeForKind(selected.RuntimeKind)
	if err != nil {
		return Agent{}, false, err
	}
	handle := runtimeHandleForAgent(selected)
	if _, err := rt.Stop(ctx, handle); err != nil {
		return Agent{}, false, err
	}
	if _, err := s.updateRuntimeState(id, agentruntime.Info{HandleID: handle.HandleID, State: agentruntime.StateStopped}); err != nil {
		return Agent{}, false, err
	}
	if err := s.observeStoppedExtensions(ctx, id); err != nil {
		return Agent{}, false, err
	}
	state, err := rt.Start(ctx, handle)
	if err != nil {
		return Agent{}, false, err
	}
	info, err := rt.Info(ctx, handle)
	if err != nil {
		return Agent{}, false, err
	}
	if info.State == "" {
		info.State = state
	}
	item, err := s.updateRuntimeState(id, info)
	if err != nil {
		return Agent{}, false, err
	}
	if err := s.observeStartedExtensions(ctx, id); err != nil {
		return Agent{}, false, err
	}
	return item, true, nil
}
