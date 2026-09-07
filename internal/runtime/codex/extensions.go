package codex

import (
	"context"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/extensionstate"
	"path/filepath"
)

func extensionStore(home string) (*extensionstate.Store, error) {
	return extensionstate.New(filepath.Join(home, "runtime-extensions"))
}

func managedExtensionInstructions(home string) ([]string, error) {
	store, err := extensionStore(home)
	if err != nil {
		return nil, err
	}
	items, err := store.List()
	if err != nil {
		return nil, err
	}
	var fragments []string
	for _, item := range items {
		if item.Instructions != "" {
			fragments = append(fragments, item.Instructions)
		}
	}
	return fragments, nil
}

func (r *Runtime) ExtensionProjections(agentID string) ([]agentruntime.ExtensionProjection, error) {
	home, err := r.resolveCodexHomeDir(agentID)
	if err != nil {
		return nil, err
	}
	store, err := extensionStore(home)
	if err != nil {
		return nil, err
	}
	return store.List()
}

func (r *Runtime) RenderExtensions(ctx context.Context, agentID string, projections []agentruntime.ExtensionProjection) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	home, err := r.resolveCodexHomeDir(agentID)
	if err != nil {
		return err
	}
	var fragments []string
	for _, projection := range projections {
		if projection.Instructions != "" {
			fragments = append(fragments, projection.Instructions)
		}
	}
	return r.refreshCodexHomeAgentsFileWithFragments(agentruntime.Handle{RuntimeID: "rt-" + canonicalRuntimeAgentID(agentID)}, home, fragments)
}

func (r *Runtime) PrepareExtensionDelete(ctx context.Context, agentID, name string) (agentruntime.PreparedExtension, error) {
	home, err := r.resolveCodexHomeDir(agentID)
	if err != nil {
		return nil, err
	}
	store, err := extensionStore(home)
	if err != nil {
		return nil, err
	}
	return store.Delete(name)
}
