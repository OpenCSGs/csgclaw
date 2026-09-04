package api

import (
	"context"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/extensionstate"
	larkextension "csgclaw/internal/runtimeextension/larkcli"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func apiExtensionStore(agentID string) (*extensionstate.Store, error) {
	root := os.Getenv("CSGCLAW_TEST_AGENT_ROOT")
	if root == "" {
		var err error
		root, err = config.DefaultAgentsDir()
		if err != nil {
			return nil, err
		}
	}
	return extensionstate.New(filepath.Join(root, agent.CanonicalID(agentID), ".codex", "home", "runtime-extensions"))
}
func (fakeCompatRuntime) ExtensionProjections(agentID string) ([]agentruntime.ExtensionProjection, error) {
	store, err := apiExtensionStore(agentID)
	if err != nil {
		return nil, err
	}
	return store.List()
}
func (fakeCompatRuntime) RenderExtensions(context.Context, string, []agentruntime.ExtensionProjection) error {
	return nil
}
func (fakeCompatRuntime) PrepareExtensionDelete(_ context.Context, agentID, name string) (agentruntime.PreparedExtension, error) {
	store, err := apiExtensionStore(agentID)
	if err != nil {
		return nil, err
	}
	return store.Delete(name)
}
func (fakeLarkCLIExtensionDriver) PrepareExtension(ctx context.Context, agentID string, desired agentruntime.ExtensionDesired) (agentruntime.PreparedExtension, agentruntime.ExtensionResult, error) {
	result := agentruntime.ExtensionResult{State: agentruntime.ExtensionStateConfigured, CheckedAt: time.Now().UTC()}
	path, err := testLarkCLIPath(ctx)
	if err != nil {
		return nil, agentruntime.ExtensionResult{State: agentruntime.ExtensionStateUnavailable, Reason: "executable_unavailable", Message: err.Error(), CheckedAt: time.Now().UTC()}, nil
	}
	store, err := apiExtensionStore(agentID)
	if err != nil {
		return nil, result, err
	}
	if current, found, err := store.Load(desired.Name); err != nil {
		return nil, result, err
	} else if found && current.SourceRevision == desired.SourceRevision {
		current.Generation = desired.Generation
		change, err := store.Revise(current)
		return change, result, err
	}
	payload, err := larkextension.Decode(desired.Payload)
	if err != nil {
		return nil, result, err
	}
	change, err := store.Stage(desired.Name)
	if err != nil {
		return nil, result, err
	}
	configDir := filepath.Join(change.Directory(), "config")
	sourcePath := filepath.Join(change.Directory(), "source", "config.json")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o700); err != nil {
		return nil, result, err
	}
	data, _ := json.Marshal(map[string]any{"accounts": map[string]any{"app": map[string]any{"id": payload.AppID}}})
	if err := os.WriteFile(sourcePath, data, 0o600); err != nil {
		return nil, result, err
	}
	cmd := larkCLICommandContext(ctx, path, "config", "bind", "--source", "lark-channel", "--identity", "bot-only", "--force", "--lang", "zh")
	cmd.Env = append(os.Environ(), "LARKSUITE_CLI_CONFIG_DIR="+configDir, "LARK_CHANNEL=1", "LARK_CHANNEL_HOME="+filepath.Dir(filepath.Dir(change.Directory())), "LARK_CHANNEL_PROFILE="+agent.CanonicalID(agentID), "LARK_CHANNEL_CONFIG="+sourcePath)
	if _, err := cmd.CombinedOutput(); err != nil {
		_ = change.Cleanup(context.Background())
		return nil, agentruntime.ExtensionResult{State: agentruntime.ExtensionStateError, Reason: "bind_failed", Message: "lark-cli config bind failed"}, err
	}
	change.SetProjection(agentruntime.ExtensionProjection{Name: desired.Name, Kind: desired.Kind, Generation: desired.Generation, SourceRevision: desired.SourceRevision, Environment: map[string]string{"LARKSUITE_CLI_CONFIG_DIR": configDir, "LARK_CHANNEL_CONFIG": sourcePath}})
	return change, result, nil
}
