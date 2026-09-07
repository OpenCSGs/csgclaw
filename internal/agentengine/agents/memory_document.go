package agents

import (
	"context"
	"fmt"
	"strings"

	agentruntime "csgclaw/internal/runtime"
)

type MemoryDocument struct {
	Enabled  bool   `json:"enabled"`
	Ready    bool   `json:"ready"`
	Name     string `json:"name"`
	Location string `json:"location,omitempty"`
	Content  string `json:"content"`
}

func (s *Controller) SupportsMemory(runtimeKind string) bool {
	rt, err := s.runtimeForKind(strings.TrimSpace(runtimeKind))
	if err != nil {
		return false
	}
	_, ok := rt.(agentruntime.MemoryController)
	return ok
}

func (s *Controller) MemoryDocument(ctx context.Context, id string) (MemoryDocument, error) {
	got, ok := s.Agent(id)
	if !ok {
		return MemoryDocument{}, fmt.Errorf("agent %q not found", strings.TrimSpace(id))
	}
	rt, err := s.runtimeForKind(strings.TrimSpace(got.RuntimeKind))
	if err != nil {
		return MemoryDocument{}, err
	}
	controller, ok := rt.(agentruntime.MemoryController)
	if !ok {
		return MemoryDocument{}, fmt.Errorf("runtime %q does not expose memory", got.RuntimeKind)
	}
	agentHome, err := s.agentHomeDir(got.ID)
	if err != nil {
		return MemoryDocument{}, err
	}
	document, err := controller.ReadMemoryDocument(ctx, agentHome, got.RuntimeOptions)
	if err != nil {
		return MemoryDocument{}, err
	}
	return MemoryDocument(document), nil
}

func (s *Controller) UpdateMemoryEnabled(ctx context.Context, id string, enabled bool) (MemoryDocument, error) {
	if s == nil {
		return MemoryDocument{}, fmt.Errorf("agent service is required")
	}
	ctx, release, err := s.acquireAgentLifecycle(ctx, id)
	if err != nil {
		return MemoryDocument{}, err
	}
	defer release()

	got, ok := s.Agent(id)
	if !ok {
		return MemoryDocument{}, fmt.Errorf("agent %q not found", strings.TrimSpace(id))
	}
	rt, err := s.runtimeForKind(strings.TrimSpace(got.RuntimeKind))
	if err != nil {
		return MemoryDocument{}, err
	}
	controller, ok := rt.(agentruntime.MemoryController)
	if !ok {
		return MemoryDocument{}, fmt.Errorf("runtime %q does not expose memory", got.RuntimeKind)
	}
	options, err := controller.ConfigureMemory(got.RuntimeOptions, enabled)
	if err != nil {
		return MemoryDocument{}, err
	}
	if _, err := s.updateWithManagedRuntimeOptions(ctx, got.ID, UpdateRequest{
		RuntimeOptions: &options,
		FieldMask:      []string{"runtime_options"},
	}, true); err != nil {
		return MemoryDocument{}, err
	}
	return s.MemoryDocument(ctx, got.ID)
}
