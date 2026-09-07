package agentengine_test

import (
	"context"
	"csgclaw/internal/runtime"
	"csgclaw/internal/runtime/extensionstate"
	"path/filepath"
	"time"
)

func (r *contractRuntime) extensionStore(agentID string) *extensionstate.Store {
	store, err := extensionstate.New(filepath.Join(r.workspace, "extensions", agentID))
	if err != nil {
		panic(err)
	}
	return store
}
func (r *contractRuntime) PrepareExtension(ctx context.Context, agentID string, desired runtime.ExtensionDesired) (runtime.PreparedExtension, runtime.ExtensionResult, error) {
	change, err := r.extensionStore(agentID).Stage(desired.Name)
	if err != nil {
		return nil, runtime.ExtensionResult{}, err
	}
	change.SetProjection(runtime.ExtensionProjection{Name: desired.Name, Kind: desired.Kind, Generation: desired.Generation, SourceRevision: desired.SourceRevision})
	return change, runtime.ExtensionResult{State: runtime.ExtensionStateConfigured, CheckedAt: time.Now().UTC()}, nil
}
func (r *contractRuntime) ExtensionProjections(agentID string) ([]runtime.ExtensionProjection, error) {
	return r.extensionStore(agentID).List()
}
func (r *contractRuntime) RenderExtensions(context.Context, string, []runtime.ExtensionProjection) error {
	return nil
}
func (r *contractRuntime) PrepareExtensionDelete(_ context.Context, agentID, name string) (runtime.PreparedExtension, error) {
	return r.extensionStore(agentID).Delete(name)
}
