package agents

import (
	"context"
	"fmt"
	"strings"

	agentruntime "csgclaw/internal/runtime"
)

func (s *Controller) SetManagedMemory(cleanup func(string) error) {
	s.mu.Lock()
	s.memoryCleanup = cleanup
	s.mu.Unlock()
}

func (s *Controller) memoryManaged() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.memoryCleanup != nil
}

type MemoryDocument struct {
	Enabled  bool   `json:"enabled"`
	Ready    bool   `json:"ready"`
	Name     string `json:"name"`
	Location string `json:"location,omitempty"`
	Content  string `json:"content"`
}

func (s *Controller) SupportsMemory(runtimeKind string) bool {
	if s.memoryManaged() {
		return runtimeKind == agentruntime.NameCodex || runtimeKind == agentruntime.NameDSH
	}
	rt, err := s.runtimeForKind(strings.TrimSpace(runtimeKind))
	if err != nil {
		return false
	}
	_, ok := rt.(agentruntime.MemoryController)
	return ok
}

func (s *Controller) MemoryDocument(ctx context.Context, id string) (MemoryDocument, error) {
	if s.memoryManaged() {
		return MemoryDocument{}, fmt.Errorf("memory is scoped to a local user")
	}
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
	if s.memoryManaged() {
		return MemoryDocument{}, fmt.Errorf("use the user-scoped memory endpoint")
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
