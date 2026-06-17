package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/picoclawsandbox"
	"csgclaw/internal/sandbox"
)

func TestEnsureChannelGatewayCreatesFeishuSidecarForCodexAgent(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	rt := &fakeRuntime{}
	SetTestHooks(
		func(_ *Service, agentName string) (sandbox.Runtime, error) {
			if agentName != "dev" {
				t.Fatalf("ensureRuntime() agentName = %q, want dev", agentName)
			}
			return rt, nil
		},
		func(_ *Service, _ context.Context, gotRT sandbox.Runtime, image, name, botID string, profile AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			if gotRT != rt {
				t.Fatalf("createGatewayBox() runtime = %p, want %p", gotRT, rt)
			}
			if image != "manager-image:1" {
				t.Fatalf("createGatewayBox() image = %q, want manager-image:1", image)
			}
			if name != "dev-feishu-gateway" {
				t.Fatalf("createGatewayBox() name = %q, want dev-feishu-gateway", name)
			}
			if botID != "u-dev" {
				t.Fatalf("createGatewayBox() botID = %q, want u-dev", botID)
			}
			if profile.BaseURL != "http://127.0.0.1:18080/api/v1/agents/u-dev/llm" {
				t.Fatalf("createGatewayBox() profile.BaseURL = %q, want codex LLM bridge", profile.BaseURL)
			}
			info := sandbox.Info{
				ID:        "box-dev-feishu",
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC),
			}
			return &fakeInfoInstance{info: info}, info, nil
		},
	)
	defer ResetTestHooks()

	var lookedUp []string
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		lookedUp = append(lookedUp, idOrName)
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		},
		"manager-image:1",
		filepath.Join(t.TempDir(), "agents.json"),
		withTestPicoClawSandboxRuntime(map[string]feishu.AppConfig{
			"u-dev": {AppID: "cli_dev", AppSecret: "dev-secret"},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	err = svc.EnsureChannelGateway(context.Background(), ChannelGatewayRequest{
		Agent:         codexGatewayTestAgent(),
		Channel:       "feishu",
		ParticipantID: "agent-dev",
		Fingerprint:   "fp-v1",
	})
	if err != nil {
		t.Fatalf("EnsureChannelGateway() error = %v", err)
	}

	if got, want := strings.Join(lookedUp, ","), "dev-feishu-gateway"; got != want {
		t.Fatalf("lookup keys = %q, want %q", got, want)
	}
	state := readChannelGatewayStateForTest(t, homeDir, "dev", "feishu")
	if state.Fingerprint != "fp-v1" || state.BoxID != "box-dev-feishu" {
		t.Fatalf("gateway state = %+v, want fingerprint fp-v1 and box-dev-feishu", state)
	}
	configText := readPicoClawConfig(t, homeDir, "dev")
	if !strings.Contains(configText, `"feishu"`) || !strings.Contains(configText, `"enabled": true`) {
		t.Fatalf("sidecar config missing enabled feishu channel:\n%s", configText)
	}
	if !strings.Contains(configText, `"csgclaw": {`) || !strings.Contains(configText, `"enabled": false`) {
		t.Fatalf("sidecar config should disable internal csgclaw channel:\n%s", configText)
	}
}

func TestEnsureChannelGatewayReusesRunningSidecarWhenFingerprintMatches(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	rt := &fakeRuntime{}
	existing := sandbox.Info{ID: "box-dev-feishu", Name: "dev-feishu-gateway", State: sandbox.StateRunning}
	writeChannelGatewayStateForTest(t, homeDir, "dev", "feishu", channelGatewayState{
		BoxID:       existing.ID,
		Name:        existing.Name,
		Channel:     "feishu",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Fingerprint: "fp-v1",
	})

	SetTestHooks(func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil },
		func(*Service, context.Context, sandbox.Runtime, string, string, string, AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			t.Fatal("createGatewayBox() called, want running sidecar reuse")
			return nil, sandbox.Info{}, nil
		},
	)
	defer ResetTestHooks()

	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		if idOrName != existing.ID {
			t.Fatalf("getBox() idOrName = %q, want existing box id", idOrName)
		}
		return &fakeInfoInstance{info: existing}, nil
	}
	testBoxInfoHook = func(_ *Service, _ context.Context, box sandbox.Instance) (sandbox.Info, error) {
		return box.Info(context.Background())
	}
	testForceRemoveBoxHook = func(*Service, context.Context, sandbox.Runtime, string) error {
		t.Fatal("forceRemoveBox() called, want reuse")
		return nil
	}

	svc, err := NewService(testModelConfig(), config.ServerConfig{ListenAddr: ":18080"}, "manager-image:1", filepath.Join(t.TempDir(), "agents.json"), withTestPicoClawSandboxRuntime())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := svc.EnsureChannelGateway(context.Background(), ChannelGatewayRequest{
		Agent:         codexGatewayTestAgent(),
		Channel:       "feishu",
		ParticipantID: "agent-dev",
		Fingerprint:   "fp-v1",
	}); err != nil {
		t.Fatalf("EnsureChannelGateway() error = %v", err)
	}
}

func TestEnsureChannelGatewayRecreatesSidecarWhenFingerprintChanges(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	rt := &fakeRuntime{}
	existing := sandbox.Info{ID: "box-dev-old", Name: "dev-feishu-gateway", State: sandbox.StateRunning}
	writeChannelGatewayStateForTest(t, homeDir, "dev", "feishu", channelGatewayState{
		BoxID:       existing.ID,
		Name:        existing.Name,
		Channel:     "feishu",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Fingerprint: "fp-old",
	})

	var removed []string
	var created int
	SetTestHooks(func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil },
		func(*Service, context.Context, sandbox.Runtime, string, string, string, AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			created++
			info := sandbox.Info{ID: "box-dev-new", Name: "dev-feishu-gateway", State: sandbox.StateRunning}
			return &fakeInfoInstance{info: info}, info, nil
		},
	)
	defer ResetTestHooks()

	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		if idOrName == existing.ID || idOrName == existing.Name {
			return &fakeInfoInstance{info: existing}, nil
		}
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) error {
		removed = append(removed, idOrName)
		return nil
	}

	svc, err := NewService(testModelConfig(), config.ServerConfig{ListenAddr: ":18080"}, "manager-image:1", filepath.Join(t.TempDir(), "agents.json"), withTestPicoClawSandboxRuntime())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := svc.EnsureChannelGateway(context.Background(), ChannelGatewayRequest{
		Agent:         codexGatewayTestAgent(),
		Channel:       "feishu",
		ParticipantID: "agent-dev",
		Fingerprint:   "fp-new",
	}); err != nil {
		t.Fatalf("EnsureChannelGateway() error = %v", err)
	}

	if got := strings.Join(removed, ","); got != existing.ID {
		t.Fatalf("removed = %q, want old box id", got)
	}
	if created != 1 {
		t.Fatalf("createGatewayBox() calls = %d, want 1", created)
	}
	state := readChannelGatewayStateForTest(t, homeDir, "dev", "feishu")
	if state.Fingerprint != "fp-new" || state.BoxID != "box-dev-new" {
		t.Fatalf("gateway state = %+v, want updated state", state)
	}
}

func TestStopChannelGatewayRemovesSidecarByStateThenName(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	rt := &fakeRuntime{}
	writeChannelGatewayStateForTest(t, homeDir, "dev", "feishu", channelGatewayState{
		BoxID:       "box-dev-feishu",
		Name:        "dev-feishu-gateway",
		Channel:     "feishu",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Fingerprint: "fp-v1",
	})

	SetTestHooks(func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil }, nil)
	defer ResetTestHooks()

	var removed []string
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) error {
		removed = append(removed, idOrName)
		if idOrName == "box-dev-feishu" {
			return fmt.Errorf("%w: stale", sandbox.ErrNotFound)
		}
		return nil
	}

	svc, err := NewService(testModelConfig(), config.ServerConfig{ListenAddr: ":18080"}, "manager-image:1", filepath.Join(t.TempDir(), "agents.json"), withTestPicoClawSandboxRuntime())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := svc.StopChannelGateway(context.Background(), "dev", "feishu"); err != nil {
		t.Fatalf("StopChannelGateway() error = %v", err)
	}

	if got, want := strings.Join(removed, ","), "box-dev-feishu,dev-feishu-gateway"; got != want {
		t.Fatalf("removed = %q, want %q", got, want)
	}
	if _, err := os.Stat(channelGatewayStatePathForTest(homeDir, "dev", "feishu")); !os.IsNotExist(err) {
		t.Fatalf("state file exists after stop: %v", err)
	}
}

func codexGatewayTestAgent() Agent {
	return Agent{
		ID:          "u-dev",
		Name:        "dev",
		RuntimeID:   "rt-u-dev",
		RuntimeKind: RuntimeKindCodex,
		Role:        RoleWorker,
		Status:      string(agentruntime.StateRunning),
		AgentProfile: AgentProfile{
			Name:            "dev",
			Provider:        ProviderCodex,
			ModelID:         "gpt-5.5",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}
}

func readPicoClawConfig(t *testing.T, homeDir, agentName string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, agentName, picoclawsandbox.HostDir, picoclawsandbox.HostConfig))
	if err != nil {
		t.Fatalf("ReadFile(picoclaw config) error = %v", err)
	}
	return string(data)
}

func readChannelGatewayStateForTest(t *testing.T, homeDir, agentName, channel string) channelGatewayState {
	t.Helper()
	path := channelGatewayStatePathForTest(homeDir, agentName, channel)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var state channelGatewayState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("json.Unmarshal(state) error = %v", err)
	}
	return state
}

func writeChannelGatewayStateForTest(t *testing.T, homeDir, agentName, channel string, state channelGatewayState) {
	t.Helper()
	path := channelGatewayStatePathForTest(homeDir, agentName, channel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(state dir) error = %v", err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("json.Marshal(state) error = %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile(state) error = %v", err)
	}
}

func channelGatewayStatePathForTest(homeDir, agentName, channel string) string {
	return filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, agentName, "channel-gateways", channel+".json")
}
