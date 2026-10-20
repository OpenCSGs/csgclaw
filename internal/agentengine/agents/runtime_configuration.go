package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
)

func (s *Controller) SetRuntimeConfigurationGuard(guard func(context.Context, string) error) {
	s.mu.Lock()
	s.runtimeConfigurationGuard = guard
	s.mu.Unlock()
}

func (s *Controller) checkRuntimeConfiguration(ctx context.Context, id string) error {
	s.mu.Lock()
	guard := s.runtimeConfigurationGuard
	s.mu.Unlock()
	if guard != nil {
		return guard(ctx, id)
	}
	return nil
}

func (s *Controller) UpdateRuntimeConfiguration(ctx context.Context, id string, resume bool, apply func(context.Context) error) error {
	ctx, release, err := s.acquireAgentLifecycle(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	selected, ok := s.Agent(id)
	if !ok {
		return fmt.Errorf("agent %q not found", id)
	}
	rt, err := s.runtimeForKind(selected.RuntimeKind)
	if err != nil {
		return err
	}
	handle := runtimeHandleForAgent(selected)
	// A connector may be configured or removed before an Agent has ever
	// started. An absent runtime is already stopped; configuration must settle.
	if _, err := rt.Stop(ctx, handle); err != nil && !sandbox.IsNotFound(err) && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	selected, err = s.updateRuntimeState(id, agentruntime.Info{HandleID: handle.HandleID, State: agentruntime.StateStopped})
	if err != nil {
		return err
	}
	if err := s.observeStoppedExtensions(ctx, id); err != nil {
		return err
	}
	if apply != nil {
		if err := apply(ctx); err != nil {
			return err
		}
	}
	// A user may change the desired state while external authorization is pending.
	if !resume || selected.DesiredState == DesiredStateStopped {
		return nil
	}
	_, err = s.reloadRuntimeLocked(ctx, selected)
	if err == nil && s.extensions != nil {
		err = s.extensions.RuntimeReady(id)
	}
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_, stopErr := rt.Stop(cleanup, handle)
		if stopErr != nil {
			return errors.Join(err, stopErr)
		}
		_, saveErr := s.updateRuntimeState(id, agentruntime.Info{HandleID: handle.HandleID, State: agentruntime.StateStopped})
		return errors.Join(err, saveErr, s.observeStoppedExtensions(cleanup, id))
	}
	return nil
}

func (s *Controller) RuntimeConfigurationReady(id string) error {
	return s.checkRuntimeConfiguration(context.Background(), id)
}

// RuntimeConfigurationProjections reads private migration input before startup.
func (s *Controller) RuntimeConfigurationProjections(id string) ([]agentruntime.ExtensionProjection, error) {
	selected, ok := s.Agent(id)
	if !ok {
		return nil, fmt.Errorf("agent %q not found", id)
	}
	rt, err := s.runtimeForKind(selected.RuntimeKind)
	if err != nil {
		return nil, err
	}
	host, ok := rt.(interface {
		ExtensionProjections(string) ([]agentruntime.ExtensionProjection, error)
	})
	if !ok {
		return nil, agentruntime.ErrExtensionUnsupported
	}
	return host.ExtensionProjections(id)
}
