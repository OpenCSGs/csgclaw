package larkcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"csgclaw/internal/identity"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/extensionstate"
)

const (
	WorkspaceName         = "lark-channel"
	ConfigFileName        = "config.json"
	sourceProviderName    = "csgclaw-pt"
	appSecretExecID       = "app_secret"
	identityPreset        = "bot-only"
	execProviderTimeoutMS = 10_000
	execProviderMaxBytes  = 64 * 1024
	bindTimeout           = 90 * time.Second
	probeTimeout          = 5 * time.Second
)

// CommandContextFunc constructs a lark-cli subprocess.
type CommandContextFunc func(context.Context, string, ...string) *exec.Cmd

// DriverOptions supplies the Runtime-owned layout and process observation.
type DriverOptions struct {
	ResolveHome    func(string) (string, error)
	RuntimeLoaded  func(string, agentruntime.ExtensionProjection) bool
	Instructions   string
	LookPath       func(string) (string, error)
	CommandContext CommandContextFunc
}

// Driver manages one worker-scoped lark-cli projection.
type Driver struct {
	options DriverOptions
}

var _ agentruntime.ExtensionDriver = (*Driver)(nil)

// NewDriver creates a lark-cli Driver for one Runtime Adapter.
func NewDriver(options DriverOptions) *Driver {
	if options.LookPath == nil {
		options.LookPath = exec.LookPath
	}
	if options.CommandContext == nil {
		options.CommandContext = exec.CommandContext
	}
	return &Driver{options: options}
}

func (d *Driver) PrepareExtension(ctx context.Context, agentID string, desired agentruntime.ExtensionDesired) (agentruntime.PreparedExtension, agentruntime.ExtensionResult, error) {
	if payload, err := Decode(desired.Payload); err == nil && payload.Legacy != nil {
		return d.prepareLegacy(ctx, agentID, desired, payload)
	}
	checked := time.Now().UTC()
	fail := func(reason, message string) (agentruntime.PreparedExtension, agentruntime.ExtensionResult, error) {
		err := errors.New(message)
		return nil, errorResult(reason, err, checked), err
	}
	payload, err := decodePayload(agentID, desired.Payload)
	if err != nil {
		return fail("invalid_source", "The lark-cli source configuration is invalid")
	}
	home, store, err := d.store(agentID)
	if err != nil {
		return fail("runtime_layout_unavailable", "The Runtime extension root is unavailable")
	}
	current, found, err := store.Load(desired.Name)
	if err != nil {
		return fail("projection_invalid", "The managed lark-cli projection is invalid; remove it and configure again")
	}
	executable, err := d.ensureExecutable(ctx)
	if err != nil {
		return nil, agentruntime.ExtensionResult{State: agentruntime.ExtensionStateUnavailable, Reason: "executable_unavailable", Message: "lark-cli is not installed, not on PATH, or cannot start. Install it for the CSGClaw account and retry.", CheckedAt: checked}, nil
	}
	result := agentruntime.ExtensionResult{State: agentruntime.ExtensionStateConfigured, Reason: "configured", CheckedAt: checked}
	if found && current.Kind == desired.Kind && current.SourceRevision == desired.SourceRevision {
		dir, dirErr := store.Directory(current)
		if dirErr == nil && validProjection(dir, payload) && current.Executable == executable && maps.Equal(current.Environment, Environment(home, dir, agentID)) && current.Instructions == d.options.Instructions {
			current.Generation = desired.Generation
			change, reviseErr := store.Revise(current)
			if reviseErr != nil {
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
	sourcePath := filepath.Join(change.Directory(), "source", ConfigFileName)
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fail("staging_failed", "Could not prepare private lark-cli configuration")
	}
	if err := writeSourceConfig(sourcePath, payload); err != nil {
		return fail("source_write_failed", "Could not write the private lark-cli source configuration")
	}
	if err := d.runConfigBind(ctx, executable, configDir, sourcePath, home, agentID); err != nil {
		return fail("bind_failed", "lark-cli config bind failed. Check the installed CLI version and Feishu Bot permissions, then retry.")
	}
	if appID, ok := readConfigAppID(filepath.Join(configDir, WorkspaceName, ConfigFileName)); !ok || appID != payload.AppID {
		return fail("bind_invalid", "lark-cli did not generate the requested Bot configuration")
	}
	change.SetProjection(agentruntime.ExtensionProjection{
		Name: desired.Name, Kind: desired.Kind, Generation: desired.Generation, SourceRevision: desired.SourceRevision,
		Executable: executable, Environment: Environment(home, change.Directory(), agentID), Instructions: d.options.Instructions,
	})
	keep = true
	return change, result, nil
}

func (d *Driver) ObserveExtension(ctx context.Context, agentID string, desired agentruntime.ExtensionDesired) (agentruntime.ExtensionResult, error) {
	if payload, err := Decode(desired.Payload); err == nil && payload.Legacy != nil {
		return d.observeLegacy(ctx, agentID, desired, payload)
	}
	checked := time.Now().UTC()
	payload, err := decodePayload(agentID, desired.Payload)
	if err != nil {
		return errorResult("invalid_source", errors.New("The lark-cli source configuration is invalid"), checked), nil
	}
	executable, err := d.ensureExecutable(ctx)
	if err != nil {
		return agentruntime.ExtensionResult{State: agentruntime.ExtensionStateUnavailable, Reason: "executable_unavailable", Message: "lark-cli is unavailable; install or repair it and retry.", CheckedAt: checked}, nil
	}
	_, store, err := d.store(agentID)
	if err != nil {
		return agentruntime.ExtensionResult{}, err
	}
	projection, found, err := store.Load(desired.Name)
	if err != nil || !found {
		return errorResult("binding_missing", errors.New("The managed lark-cli projection is missing"), checked), nil
	}
	dir, err := store.Directory(projection)
	if err != nil || projection.SourceRevision != desired.SourceRevision || projection.Generation != desired.Generation || projection.Executable != executable || !validProjection(dir, payload) {
		return errorResult("binding_mismatch", errors.New("The lark-cli projection does not match its desired configuration"), checked), nil
	}
	loaded := d.options.RuntimeLoaded != nil && d.options.RuntimeLoaded(agentID, projection)
	return agentruntime.ExtensionResult{State: agentruntime.ExtensionStateConfigured, Reason: "configured", RuntimeLoaded: loaded, CheckedAt: checked}, nil
}

func (d *Driver) store(agentID string) (string, *extensionstate.Store, error) {
	if d == nil || d.options.ResolveHome == nil {
		return "", nil, errors.New("Runtime home resolver is unavailable")
	}
	home, err := d.options.ResolveHome(agentID)
	if err != nil {
		return "", nil, err
	}
	store, err := extensionstate.New(filepath.Join(home, "runtime-extensions"))
	return home, store, err
}

// Environment returns the worker-scoped lark-cli process environment.
func Environment(home, dir, agentID string) map[string]string {
	return map[string]string{
		"LARKSUITE_CLI_CONFIG_DIR": filepath.Join(dir, "config"),
		"LARK_CHANNEL":             "1",
		"LARK_CHANNEL_HOME":        home,
		"LARK_CHANNEL_PROFILE":     identity.CanonicalAgentID(agentID),
		"LARK_CHANNEL_CONFIG":      filepath.Join(dir, "source", ConfigFileName),
	}
}

func decodePayload(agentID string, raw json.RawMessage) (Payload, error) {
	payload, err := Decode(raw)
	if err != nil {
		return Payload{}, fmt.Errorf("decode lark-cli extension source: %w", err)
	}
	payload.AgentID = identity.CanonicalAgentID(payload.AgentID)
	if payload.AgentID == "" || payload.AgentID != identity.CanonicalAgentID(agentID) || strings.TrimSpace(payload.ParticipantID) == "" || strings.TrimSpace(payload.AppID) == "" || strings.TrimSpace(payload.BaseURL) == "" || strings.TrimSpace(payload.AccessToken) == "" || strings.TrimSpace(payload.HelperPath) == "" {
		return Payload{}, errors.New("lark-cli extension source is incomplete or belongs to a different agent")
	}
	payload.ParticipantID = strings.TrimSpace(payload.ParticipantID)
	payload.AppID = strings.TrimSpace(payload.AppID)
	payload.BaseURL = strings.TrimRight(strings.TrimSpace(payload.BaseURL), "/")
	payload.AccessToken = strings.TrimSpace(payload.AccessToken)
	payload.HelperPath = strings.TrimSpace(payload.HelperPath)
	return payload, nil
}

func (d *Driver) ensureExecutable(ctx context.Context) (string, error) {
	path, err := d.options.LookPath("lark-cli")
	path = strings.TrimSpace(path)
	if err != nil || path == "" {
		return "", errors.New("lark-cli is not installed or not on PATH; install @larksuite/cli or an official native binary, restart CSGClaw if PATH changed, then retry")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve lark-cli executable path: %w", err)
	}
	path = filepath.Clean(path)
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	cmd := d.options.CommandContext(probeCtx, path, "-v")
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return path, fmt.Errorf("lark-cli cannot start; check executable permissions, platform architecture, and runtime dependencies: %w", err)
	}
	return path, nil
}

func (d *Driver) runConfigBind(ctx context.Context, path, configDir, sourcePath, channelHome, profile string) error {
	bindCtx, cancel := context.WithTimeout(ctx, bindTimeout)
	defer cancel()
	cmd := d.options.CommandContext(bindCtx, path, "config", "bind", "--source", WorkspaceName, "--identity", identityPreset, "--force", "--lang", "zh")
	cmd.Dir = configDir
	cmd.Env = mergeCommandEnv(os.Environ(), map[string]string{
		"LARKSUITE_CLI_CONFIG_DIR": configDir,
		"LARK_CHANNEL":             "1",
		"LARK_CHANNEL_HOME":        channelHome,
		"LARK_CHANNEL_PROFILE":     identity.CanonicalAgentID(profile),
		"LARK_CHANNEL_CONFIG":      sourcePath,
	})
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("lark-cli config bind failed: %w", err)
	}
	return nil
}

func validProjection(dir string, payload Payload) bool {
	appID, ok := readConfigAppID(filepath.Join(dir, "config", WorkspaceName, ConfigFileName))
	if !ok || appID != payload.AppID {
		return false
	}
	actual, err := os.ReadFile(filepath.Join(dir, "source", ConfigFileName))
	if err != nil {
		return false
	}
	expected, err := sourceConfig(payload)
	return err == nil && bytes.Equal(actual, expected)
}

func sourceConfig(payload Payload) ([]byte, error) {
	value := map[string]any{
		"accounts": map[string]any{"app": map[string]any{
			"id":     payload.AppID,
			"secret": map[string]any{"source": "exec", "provider": sourceProviderName, "id": appSecretExecID},
			"tenant": "feishu",
		}},
		"secrets": map[string]any{"providers": map[string]any{sourceProviderName: map[string]any{
			"source": "exec", "command": payload.HelperPath,
			"args":        []string{"pt", "app-info", "--channel", "feishu", "--agent-id", payload.AgentID, "--exec-provider"},
			"env":         map[string]string{"CSGCLAW_BASE_URL": payload.BaseURL, "CSGCLAW_ACCESS_TOKEN": payload.AccessToken},
			"trustedDirs": trustedPaths(payload.HelperPath), "allowInsecurePath": true, "allowSymlinkCommand": true,
			"noOutputTimeoutMs": execProviderTimeoutMS, "maxOutputBytes": execProviderMaxBytes,
		}}},
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func writeSourceConfig(path string, payload Payload) error {
	data, err := sourceConfig(payload)
	if err != nil {
		return err
	}
	return writeFile(path, data)
}

func writeFile(path string, data []byte) error {
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

func readConfigAppID(path string) (string, bool) {
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

func trustedPaths(helperPath string) []string {
	if goruntime.GOOS == "windows" {
		return []string{helperPath}
	}
	return []string{filepath.Dir(helperPath)}
}

func mergeCommandEnv(base []string, overrides map[string]string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	effectiveKeys := make(map[string]string, len(base)+len(overrides))
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			canonical := agentruntime.CanonicalEnvironmentKey(key)
			if previous := effectiveKeys[canonical]; previous != "" && previous != key {
				delete(values, previous)
			}
			values[key] = value
			effectiveKeys[canonical] = key
		}
	}
	for key, value := range overrides {
		canonical := agentruntime.CanonicalEnvironmentKey(key)
		if previous := effectiveKeys[canonical]; previous != "" && previous != key {
			delete(values, previous)
		}
		values[key] = value
		effectiveKeys[canonical] = key
	}
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	return out
}

func errorResult(reason string, err error, checkedAt time.Time) agentruntime.ExtensionResult {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return agentruntime.ExtensionResult{State: agentruntime.ExtensionStateError, Reason: reason, Message: message, CheckedAt: checkedAt}
}
