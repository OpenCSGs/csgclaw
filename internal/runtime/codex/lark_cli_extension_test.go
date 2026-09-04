package codex

import (
	"context"
	agent "csgclaw/internal/agentengine/agents"
	agentruntime "csgclaw/internal/runtime"
	larkextension "csgclaw/internal/runtimeextension/larkcli"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLarkCLIExtensionStagesAndObservesProjection(t *testing.T) {
	rt, home := newLarkCLIExtensionTestRuntime(t)
	restore := installFakeLarkCLIExtensionCommand(t)
	defer restore()
	driver, _ := rt.RuntimeExtensionDriver(larkextension.Kind)
	ctx := context.Background()
	desired := agentruntime.ExtensionDesired{Name: larkextension.Name, Kind: larkextension.Kind, Generation: 1, SourceRevision: "rev-1", Payload: larkCLIExtensionPayload(t, "agent-dev", "pt-dev", "cli_dev")}
	change, result, err := driver.PrepareExtension(ctx, "agent-dev", desired)
	if err != nil || result.State != agentruntime.ExtensionStateConfigured {
		t.Fatalf("Prepare=%+v %v", result, err)
	}
	if items, _ := rt.ExtensionProjections("agent-dev"); len(items) != 0 {
		t.Fatal("staging altered active state")
	}
	if err := change.Activate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := rt.RenderExtensions(ctx, "agent-dev", []agentruntime.ExtensionProjection{change.Projection()}); err != nil {
		t.Fatal(err)
	}
	if err := change.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	projection := change.Projection()
	env, digests, err := buildSessionEnvironment(SessionSpec{AgentID: "agent-dev", CodexHomeDir: filepath.Dir(rt.Layout(home).SkillsRoot)})
	if err != nil || !envContains(env, "LARK_CHANNEL_PROFILE=agent-dev") || digests[larkextension.Name] != projection.Digest {
		t.Fatalf("launch environment: %v", err)
	}
	observed, err := driver.ObserveExtension(ctx, "agent-dev", desired)
	if err != nil || observed.State != agentruntime.ExtensionStateConfigured || observed.RuntimeLoaded {
		t.Fatalf("disk state mistaken for live process: %+v %v", observed, err)
	}
	desired.Generation++
	retry, _, err := driver.PrepareExtension(ctx, "agent-dev", desired)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Projection().Root != projection.Root || retry.Projection().Digest != projection.Digest {
		t.Fatal("retry replaced identical CLI configuration")
	}
	if err := retry.Activate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := retry.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	deleted, err := rt.PrepareExtensionDelete(ctx, "agent-dev", desired.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := deleted.Activate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := rt.RenderExtensions(ctx, "agent-dev", nil); err != nil {
		t.Fatal(err)
	}
	if err := deleted.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if items, err := rt.ExtensionProjections("agent-dev"); err != nil || len(items) != 0 {
		t.Fatalf("delete=%+v %v", items, err)
	}
}

func TestLarkCLIUnavailablePreparationDoesNotChangeActiveProjection(t *testing.T) {
	rt, _ := newLarkCLIExtensionTestRuntime(t)
	restore := installFakeLarkCLIExtensionCommand(t)
	driver, _ := rt.RuntimeExtensionDriver(larkextension.Kind)
	ctx := context.Background()
	desired := agentruntime.ExtensionDesired{Name: larkextension.Name, Kind: larkextension.Kind, Generation: 1, SourceRevision: "rev-1", Payload: larkCLIExtensionPayload(t, "agent-dev", "pt-dev", "cli_old")}
	change, _, err := driver.PrepareExtension(ctx, "agent-dev", desired)
	if err != nil {
		t.Fatal(err)
	}
	if err := change.Activate(ctx); err != nil {
		t.Fatal(err)
	}
	restore()
	previous := larkCLILookPath
	larkCLILookPath = func(string) (string, error) { return "", os.ErrNotExist }
	defer func() { larkCLILookPath = previous }()
	desired.Generation++
	desired.SourceRevision = "rev-2"
	desired.Payload = larkCLIExtensionPayload(t, "agent-dev", "pt-dev", "cli_new")
	prepared, result, err := driver.PrepareExtension(ctx, "agent-dev", desired)
	if err != nil || prepared != nil || result.State != agentruntime.ExtensionStateUnavailable {
		t.Fatalf("Prepare=%+v %v", result, err)
	}
	items, _ := rt.ExtensionProjections("agent-dev")
	if len(items) != 1 || items[0].SourceRevision != "rev-1" {
		t.Fatal("prepare altered active projection; Engine owns failure policy")
	}
}

func newLarkCLIExtensionTestRuntime(t *testing.T) (*Runtime, string) {
	t.Helper()
	root := t.TempDir()
	agentHome := filepath.Join(root, "agent-dev")
	runtimeImpl := New(Dependencies{
		AgentHome: func(string) (string, error) { return agentHome, nil },
		ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
			return AgentRef{ID: "agent-dev", Name: "dev", RuntimeID: "rt-agent-dev", Instructions: "Stay concise."}, nil
		},
	})
	return runtimeImpl, agentHome
}

func larkCLIExtensionPayload(t *testing.T, agentID, participantID, appID string) json.RawMessage {
	t.Helper()
	payload, err := larkextension.Encode(larkextension.Payload{
		AgentID: agent.CanonicalID(agentID), ParticipantID: participantID, AppID: appID,
		BaseURL: "http://csgclaw.test", AccessToken: "source-token", HelperPath: "/opt/csgclaw/bin/csgclaw",
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func installFakeLarkCLIExtensionCommand(t *testing.T) func() {
	t.Helper()
	originalLookPath := larkCLILookPath
	originalCommand := larkCLICommandContext
	t.Setenv("CSGCLAW_LARK_EXTENSION_HELPER", "1")
	larkCLILookPath = func(name string) (string, error) {
		if name != "lark-cli" {
			return "", os.ErrNotExist
		}
		return "/opt/lark-cli", nil
	}
	larkCLICommandContext = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		helperArgs := append([]string{"-test.run=^TestLarkCLIExtensionHelper$", "--"}, args...)
		return exec.CommandContext(ctx, os.Args[0], helperArgs...)
	}
	return func() {
		larkCLILookPath = originalLookPath
		larkCLICommandContext = originalCommand
	}
}

func TestLarkCLIExtensionHelper(t *testing.T) {
	if os.Getenv("CSGCLAW_LARK_EXTENSION_HELPER") != "1" {
		return
	}
	separator := -1
	for index, value := range os.Args {
		if value == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		os.Exit(2)
	}
	args := os.Args[separator+1:]
	if len(args) == 1 && args[0] == "-v" {
		os.Exit(0)
	}
	if strings.Join(args, " ") != "config bind --source lark-channel --identity bot-only --force --lang zh" {
		os.Exit(3)
	}
	sourceRaw, err := os.ReadFile(os.Getenv("LARK_CHANNEL_CONFIG"))
	if err != nil {
		os.Exit(4)
	}
	var source struct {
		Accounts struct {
			App struct {
				ID string `json:"id"`
			} `json:"app"`
		} `json:"accounts"`
	}
	if json.Unmarshal(sourceRaw, &source) != nil || source.Accounts.App.ID == "" {
		os.Exit(5)
	}
	path := filepath.Join(os.Getenv("LARKSUITE_CLI_CONFIG_DIR"), larkCLIWorkspaceName, larkCLIConfigFileName)
	if os.MkdirAll(filepath.Dir(path), 0o700) != nil {
		os.Exit(6)
	}
	data, _ := json.Marshal(map[string]any{"apps": []map[string]string{{"appId": source.Accounts.App.ID}}})
	if os.WriteFile(path, data, 0o600) != nil {
		os.Exit(7)
	}
	os.Exit(0)
}

func envContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
