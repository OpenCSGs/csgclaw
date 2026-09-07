package agents

import (
	context "context"
	config "csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	hub "csgclaw/internal/template"
	errors "errors"
	fmt "fmt"
	io "io"
	os "os"
	strings "strings"
)

func (s *WorkspaceService) agentHomeDir(agentID string) (string, error) {
	root := ""
	if s != nil {
		root = strings.TrimSpace(s.agentsRoot)
	}
	if root == "" {
		var err error
		root, err = config.DefaultAgentsDir()
		if err != nil {
			return "", err
		}
	}
	return agentHomeDirInRoot(root, agentID)
}

func (s *WorkspaceService) WorkspaceRoot(agentName string) (string, error) {
	got, ok := s.agentSnapshotByName(agentName)
	if !ok {
		return "", fmt.Errorf("agent %q not found", strings.TrimSpace(agentName))
	}
	return s.agentWorkspaceRoot(got.ID, got.RuntimeKind)
}

func (s *WorkspaceService) WorkspaceRootByID(agentID string) (string, error) {
	got, ok := s.agentSnapshot(agentID)
	if !ok {
		return "", fmt.Errorf("agent %q not found", strings.TrimSpace(agentID))
	}
	return s.agentWorkspaceRoot(got.ID, got.RuntimeKind)
}

func (s *WorkspaceService) SkillsRoot(agentName string) (string, error) {
	got, ok := s.agentSnapshotByName(agentName)
	if !ok {
		return "", fmt.Errorf("agent %q not found", strings.TrimSpace(agentName))
	}
	return s.agentSkillsRoot(got.ID, got.RuntimeKind)
}

func (s *WorkspaceService) HubPublishSpec(agentID string, includeMemory bool) (hub.PublishSpec, error) {
	if s == nil {
		return hub.PublishSpec{}, fmt.Errorf("agent service is required")
	}
	got, ok := s.agentSnapshot(agentID)
	if !ok {
		return hub.PublishSpec{}, fmt.Errorf("agent %q not found", strings.TrimSpace(agentID))
	}
	layout, err := s.agentLayout(got.ID, got.RuntimeKind)
	if err != nil {
		return hub.PublishSpec{}, err
	}
	runtimeOptions, err := templateSafeRuntimeOptions(got)
	if err != nil {
		return hub.PublishSpec{}, err
	}
	memoryPath := ""
	if includeMemory && runtimeOptions[templateMemoryModeKey] != templateMemoryModeDisabled {
		memoryPath = codexMemoryPath(layout, got.RuntimeKind)
	}
	if memoryPath != "" {
		if _, statErr := os.Stat(memoryPath); errors.Is(statErr, os.ErrNotExist) {
			memoryPath = ""
		} else if statErr != nil {
			return hub.PublishSpec{}, fmt.Errorf("stat codex memory summary: %w", statErr)
		}
	}
	return hub.PublishSpec{
		ID:             got.Name,
		IncludeMemory:  includeMemory,
		Name:           got.Name,
		Description:    got.Description,
		RuntimeKind:    got.RuntimeConfig().Kind(),
		Image:          got.Image,
		RuntimeOptions: runtimeOptions,
		WorkspaceRef: hub.WorkspaceRef{
			Kind:             hub.WorkspaceKindDir,
			Path:             layout.WorkspaceRoot,
			InstructionsPath: layout.InstructionsPath,
			SkillsPath:       layout.SkillsRoot,
			MemoryPath:       memoryPath,
		},
		MCPServers: templateSafeMCPServers(got.MCPServers),
	}, nil
}

func (s *WorkspaceService) StreamLogs(ctx context.Context, id string, follow bool, lines int, w io.Writer) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("agent id is required")
	}
	if w == nil {
		return fmt.Errorf("log writer is required")
	}
	if lines <= 0 {
		lines = 20
	}

	got, ok := s.agentSnapshot(id)
	if !ok {
		return fmt.Errorf("agent %q not found", id)
	}
	runtimeImpl, err := s.registry.Get(strings.TrimSpace(got.RuntimeKind))
	if err != nil {
		return err
	}
	streamer, ok := runtimeImpl.(agentruntime.LogStreamer)
	if !ok {
		return fmt.Errorf("runtime %q does not support log streaming", runtimeImpl.Kind())
	}
	return streamer.StreamLogs(ctx, runtimeHandleForAgent(got), agentruntime.LogOptions{
		Follow: follow,
		Tail:   lines,
		Writer: w,
	})
}

func (s *WorkspaceService) agentWorkspaceRoot(agentID, runtimeKind string) (string, error) {
	layout, err := s.agentLayout(agentID, runtimeKind)
	if err != nil {
		return "", err
	}
	return layout.WorkspaceRoot, nil
}

func (s *WorkspaceService) agentSkillsRoot(agentID, runtimeKind string) (string, error) {
	layout, err := s.agentLayout(agentID, runtimeKind)
	if err != nil {
		return "", err
	}
	return layout.SkillsRoot, nil
}

func (s *WorkspaceService) AgentLayout(agentID string) (agentruntime.Layout, error) {
	got, ok := s.agentSnapshot(agentID)
	if !ok {
		return agentruntime.Layout{}, fmt.Errorf("agent %q not found", canonicalAgentID(agentID))
	}
	return s.agentLayout(got.ID, got.RuntimeKind)
}

func (s *WorkspaceService) agentLayout(agentID, runtimeKind string) (agentruntime.Layout, error) {
	agentHome, err := s.agentHomeDir(agentID)
	if err != nil {
		return agentruntime.Layout{}, err
	}
	rt, err := s.registry.Get(strings.TrimSpace(runtimeKind))
	if err != nil {
		return agentruntime.Layout{}, err
	}
	layout := rt.Layout(agentHome)
	if strings.TrimSpace(layout.WorkspaceRoot) == "" {
		return agentruntime.Layout{}, fmt.Errorf("runtime %q returned empty workspace root", rt.Kind())
	}
	if strings.TrimSpace(layout.SkillsRoot) == "" {
		return agentruntime.Layout{}, fmt.Errorf("runtime %q returned empty skills root", rt.Kind())
	}
	return layout, nil
}
