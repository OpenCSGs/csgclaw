package codex

import (
	"bytes"
	"context"
	agentruntime "csgclaw/internal/runtime"
	larkextension "csgclaw/internal/runtimeextension/larkcli"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	larkCLIWorkspaceName         = "lark-channel"
	larkCLIConfigFileName        = "config.json"
	larkCLISourceProviderName    = "csgclaw-pt"
	larkCLIAppSecretExecID       = "app_secret"
	larkCLIIdentityPreset        = "bot-only"
	larkCLIExecProviderTimeoutMS = 10_000
	larkCLIExecProviderMaxBytes  = 64 * 1024
	larkCLIBindTimeout           = 90 * time.Second
	larkCLIProbeTimeout          = 5 * time.Second
)

var (
	larkCLILookPath       = exec.LookPath
	larkCLICommandContext = exec.CommandContext
)

type larkCLIExtensionDriver struct {
	runtime *Runtime
}

var _ agentruntime.ExtensionDriverProvider = (*Runtime)(nil)
var _ agentruntime.ExtensionDriver = (*larkCLIExtensionDriver)(nil)

func (r *Runtime) RuntimeExtensionDriver(kind string) (agentruntime.ExtensionDriver, bool) {
	if r == nil || strings.TrimSpace(kind) != larkextension.Kind {
		return nil, false
	}
	return &larkCLIExtensionDriver{runtime: r}, true
}

func (d *larkCLIExtensionDriver) PrepareExtension(ctx context.Context, agentID string, desired agentruntime.ExtensionDesired) (agentruntime.PreparedExtension, agentruntime.ExtensionResult, error) {
	checked := time.Now().UTC()
	fail := func(reason, message string) (agentruntime.PreparedExtension, agentruntime.ExtensionResult, error) {
		err := errors.New(message)
		return nil, larkCLIErrorResult(reason, err, checked), err
	}
	payload, err := decodeLarkCLIPayload(agentID, desired.Payload)
	if err != nil {
		return fail("invalid_source", "The lark-cli source configuration is invalid")
	}
	home, err := d.runtime.resolveCodexHomeDir(agentID)
	if err != nil {
		return fail("runtime_layout_unavailable", "The Runtime home is unavailable")
	}
	store, err := extensionStore(home)
	if err != nil {
		return fail("runtime_layout_unavailable", "The Runtime extension root is unavailable")
	}
	current, found, err := store.Load(desired.Name)
	if err != nil {
		return fail("projection_invalid", "The managed lark-cli projection is invalid; remove it and configure again")
	}
	executable, err := ensureLarkCLI(ctx)
	if err != nil {
		return nil, agentruntime.ExtensionResult{State: agentruntime.ExtensionStateUnavailable, Reason: "executable_unavailable", Message: "lark-cli is not installed, not on PATH, or cannot start. Install it for the CSGClaw account and retry.", CheckedAt: checked}, nil
	}
	result := agentruntime.ExtensionResult{State: agentruntime.ExtensionStateConfigured, Reason: "configured", CheckedAt: checked}
	if found && current.Kind == desired.Kind && current.SourceRevision == desired.SourceRevision {
		dir, dirErr := store.Directory(current)
		if dirErr == nil && validLarkProjection(dir, payload) && maps.Equal(current.Environment, larkEnvironment(home, dir, agentID)) && current.Instructions == feishuLarkCLIManagedInstructions {
			current.Generation = desired.Generation
			change, err := store.Revise(current)
			if err != nil {
				return fail("staging_failed", "Could not prepare the lark-cli configuration")
			}
			return change, result, nil
		}
	}
	change, err := store.Stage(desired.Name)
	if err != nil {
		return fail("staging_failed", "Could not create private lark-cli staging")
	}
	keep := false
	defer func() {
		if !keep {
			_ = change.Cleanup(context.WithoutCancel(ctx))
		}
	}()
	configDir := filepath.Join(change.Directory(), "config")
	sourcePath := filepath.Join(change.Directory(), "source", "config.json")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fail("staging_failed", "Could not prepare private lark-cli configuration")
	}
	if err := writeLarkChannelSourceConfig(sourcePath, payload); err != nil {
		return fail("source_write_failed", "Could not write the private lark-cli source configuration")
	}
	if err := runLarkCLIConfigBind(ctx, executable, configDir, sourcePath, home, agentID); err != nil {
		return fail("bind_failed", "lark-cli config bind failed. Check the installed CLI version and Feishu Bot permissions, then retry.")
	}
	if appID, ok := readLarkCLIConfigAppID(filepath.Join(configDir, larkCLIWorkspaceName, larkCLIConfigFileName)); !ok || appID != payload.AppID {
		return fail("bind_invalid", "lark-cli did not generate the requested Bot configuration")
	}
	change.SetProjection(agentruntime.ExtensionProjection{
		Name: desired.Name, Kind: desired.Kind, Generation: desired.Generation, SourceRevision: desired.SourceRevision,
		Environment: larkEnvironment(home, change.Directory(), agentID), Instructions: feishuLarkCLIManagedInstructions,
	})
	keep = true
	return change, result, nil
}

func larkEnvironment(home, dir, agentID string) map[string]string {
	return map[string]string{
		"LARKSUITE_CLI_CONFIG_DIR": filepath.Join(dir, "config"),
		"LARK_CHANNEL":             "1",
		"LARK_CHANNEL_HOME":        home,
		"LARK_CHANNEL_PROFILE":     canonicalRuntimeAgentID(agentID),
		"LARK_CHANNEL_CONFIG":      filepath.Join(dir, "source", "config.json"),
	}
}

func validLarkProjection(dir string, payload larkextension.Payload) bool {
	appID, ok := readLarkCLIConfigAppID(filepath.Join(dir, "config", larkCLIWorkspaceName, larkCLIConfigFileName))
	if !ok || appID != payload.AppID {
		return false
	}
	actual, err := os.ReadFile(filepath.Join(dir, "source", "config.json"))
	if err != nil {
		return false
	}
	expected, err := larkSourceConfig(payload)
	return err == nil && bytes.Equal(actual, expected)
}

func (d *larkCLIExtensionDriver) ObserveExtension(ctx context.Context, agentID string, desired agentruntime.ExtensionDesired) (agentruntime.ExtensionResult, error) {
	checked := time.Now().UTC()
	payload, err := decodeLarkCLIPayload(agentID, desired.Payload)
	if err != nil {
		return larkCLIErrorResult("invalid_source", errors.New("The lark-cli source configuration is invalid"), checked), nil
	}
	if _, err := ensureLarkCLI(ctx); err != nil {
		return agentruntime.ExtensionResult{State: agentruntime.ExtensionStateUnavailable, Reason: "executable_unavailable", Message: "lark-cli is unavailable; install or repair it and retry.", CheckedAt: checked}, nil
	}
	home, err := d.runtime.resolveCodexHomeDir(agentID)
	if err != nil {
		return agentruntime.ExtensionResult{}, err
	}
	store, err := extensionStore(home)
	if err != nil {
		return agentruntime.ExtensionResult{}, err
	}
	projection, found, err := store.Load(desired.Name)
	if err != nil || !found {
		return larkCLIErrorResult("binding_missing", errors.New("The managed lark-cli projection is missing"), checked), nil
	}
	dir, err := store.Directory(projection)
	if err != nil || projection.SourceRevision != desired.SourceRevision || projection.Generation != desired.Generation || !validLarkProjection(dir, payload) {
		return larkCLIErrorResult("binding_mismatch", errors.New("The lark-cli projection does not match its desired configuration"), checked), nil
	}
	loaded := false
	if session, err := d.runtime.SessionManager().LiveSession(SessionHandle{RuntimeID: "rt-" + canonicalRuntimeAgentID(agentID)}); err == nil && session != nil && session.ProcessID > 0 {
		loaded = session.ExtensionDigests[desired.Name] == projection.Digest
	}
	return agentruntime.ExtensionResult{State: agentruntime.ExtensionStateConfigured, Reason: "configured", RuntimeLoaded: loaded, CheckedAt: checked}, nil
}

func decodeLarkCLIPayload(agentID string, raw json.RawMessage) (larkextension.Payload, error) {
	payload, err := larkextension.Decode(raw)
	if err != nil {
		return larkextension.Payload{}, fmt.Errorf("decode lark-cli extension source: %w", err)
	}
	payload.AgentID = canonicalRuntimeAgentID(payload.AgentID)
	if payload.AgentID == "" || payload.AgentID != canonicalRuntimeAgentID(agentID) || strings.TrimSpace(payload.ParticipantID) == "" || strings.TrimSpace(payload.AppID) == "" || strings.TrimSpace(payload.BaseURL) == "" || strings.TrimSpace(payload.AccessToken) == "" || strings.TrimSpace(payload.HelperPath) == "" {
		return larkextension.Payload{}, fmt.Errorf("lark-cli extension source is incomplete or belongs to a different agent")
	}
	payload.ParticipantID = strings.TrimSpace(payload.ParticipantID)
	payload.AppID = strings.TrimSpace(payload.AppID)
	payload.BaseURL = strings.TrimRight(strings.TrimSpace(payload.BaseURL), "/")
	payload.AccessToken = strings.TrimSpace(payload.AccessToken)
	payload.HelperPath = strings.TrimSpace(payload.HelperPath)
	return payload, nil
}

func ensureLarkCLI(ctx context.Context) (string, error) {
	path, err := larkCLILookPath("lark-cli")
	path = strings.TrimSpace(path)
	if err != nil || path == "" {
		return "", fmt.Errorf("lark-cli is not installed or not on PATH; install @larksuite/cli or an official native binary, restart CSGClaw if PATH changed, then retry")
	}
	probeCtx, cancel := context.WithTimeout(ctx, larkCLIProbeTimeout)
	defer cancel()
	cmd := larkCLICommandContext(probeCtx, path, "-v")
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return path, fmt.Errorf("lark-cli cannot start; check executable permissions, platform architecture, and runtime dependencies: %w", err)
	}
	return path, nil
}

func runLarkCLIConfigBind(ctx context.Context, path, configDir, sourcePath, channelHome, profile string) error {
	bindCtx, cancel := context.WithTimeout(ctx, larkCLIBindTimeout)
	defer cancel()
	cmd := larkCLICommandContext(bindCtx, path, "config", "bind", "--source", larkCLIWorkspaceName, "--identity", larkCLIIdentityPreset, "--force", "--lang", "zh")
	cmd.Dir = configDir
	cmd.Env = mergeCommandEnv(os.Environ(), map[string]string{
		"LARKSUITE_CLI_CONFIG_DIR": configDir,
		"LARK_CHANNEL":             "1",
		"LARK_CHANNEL_HOME":        channelHome,
		"LARK_CHANNEL_PROFILE":     canonicalRuntimeAgentID(profile),
		"LARK_CHANNEL_CONFIG":      sourcePath,
	})
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("lark-cli config bind failed: %w", err)
	}
	return nil
}

func larkSourceConfig(payload larkextension.Payload) ([]byte, error) {
	value := map[string]any{
		"accounts": map[string]any{"app": map[string]any{
			"id":     payload.AppID,
			"secret": map[string]any{"source": "exec", "provider": larkCLISourceProviderName, "id": larkCLIAppSecretExecID},
			"tenant": "feishu",
		}},
		"secrets": map[string]any{"providers": map[string]any{larkCLISourceProviderName: map[string]any{
			"source": "exec", "command": payload.HelperPath,
			"args":        []string{"pt", "app-info", "--channel", "feishu", "--agent-id", payload.AgentID, "--exec-provider"},
			"env":         map[string]string{"CSGCLAW_BASE_URL": payload.BaseURL, "CSGCLAW_ACCESS_TOKEN": payload.AccessToken},
			"trustedDirs": trustedLarkCLIPaths(payload.HelperPath), "allowInsecurePath": true, "allowSymlinkCommand": true,
			"noOutputTimeoutMs": larkCLIExecProviderTimeoutMS, "maxOutputBytes": larkCLIExecProviderMaxBytes,
		}}},
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func writeLarkChannelSourceConfig(path string, payload larkextension.Payload) error {
	data, err := larkSourceConfig(payload)
	if err != nil {
		return err
	}
	return writeLarkCLIFile(path, data)
}

func writeLarkCLIFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".runtime-extension-file-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func readLarkCLIConfigAppID(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var payload struct {
		Apps []struct {
			AppID string `json:"appId"`
		} `json:"apps"`
	}
	if json.Unmarshal(data, &payload) != nil || len(payload.Apps) == 0 {
		return "", false
	}
	id := strings.TrimSpace(payload.Apps[0].AppID)
	return id, id != ""
}

func trustedLarkCLIPaths(helperPath string) []string {
	if runtime.GOOS == "windows" {
		return []string{helperPath}
	}
	return []string{filepath.Dir(helperPath)}
}

func mergeCommandEnv(base []string, overrides map[string]string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	return out
}

func larkCLIErrorResult(reason string, err error, checkedAt time.Time) agentruntime.ExtensionResult {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return agentruntime.ExtensionResult{State: agentruntime.ExtensionStateError, Reason: reason, Message: message, CheckedAt: checkedAt}
}
