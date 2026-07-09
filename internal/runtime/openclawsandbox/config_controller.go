package openclawsandbox

import (
	"context"
	"path/filepath"

	agentruntime "csgclaw/internal/runtime"
)

var _ agentruntime.RuntimeConfigController = (*Runtime)(nil)

func (r *Runtime) ValidateConfig(_ context.Context, _ agentruntime.RuntimeConfigSnapshot) error {
	return nil
}

func (r *Runtime) RestartRequired(agentruntime.RuntimeConfigChange) (bool, error) {
	return false, nil
}

func (r *Runtime) ReconcileConfig(_ context.Context, h agentruntime.Handle, _ agentruntime.RuntimeConfigChange) error {
	agentRef, err := r.ResolveAgentForHandle(h)
	if err != nil {
		return err
	}
	agentHome, err := r.AgentHomeForAgentID(agentRef.ID)
	if err != nil {
		return err
	}
	return refreshWorkspaceAgentsFile(filepath.Join(r.Layout(agentHome).WorkspaceRoot, "AGENTS.md"), agentRef.Instructions)
}
