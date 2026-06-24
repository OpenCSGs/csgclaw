package sandboxgateway

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
)

type testSandboxRuntime struct{}

func (testSandboxRuntime) Create(context.Context, sandbox.CreateSpec) (sandbox.Instance, error) {
	return nil, nil
}

func (testSandboxRuntime) Get(context.Context, string) (sandbox.Instance, error) {
	return nil, nil
}

func (testSandboxRuntime) Remove(context.Context, string, sandbox.RemoveOptions) error {
	return nil
}

func (testSandboxRuntime) Close() error {
	return nil
}

type testSandboxBox struct{}

func (testSandboxBox) Start(context.Context) error {
	return nil
}

func (testSandboxBox) Stop(context.Context, sandbox.StopOptions) error {
	return nil
}

func (testSandboxBox) Info(context.Context) (sandbox.Info, error) {
	return sandbox.Info{}, nil
}

func (testSandboxBox) Run(context.Context, sandbox.CommandSpec) (sandbox.CommandResult, error) {
	return sandbox.CommandResult{}, nil
}

func (testSandboxBox) Close() error {
	return nil
}

func TestCreateGatewayBoxWaitsForDockerReadiness(t *testing.T) {
	var attempts int
	rt := New(testGatewayDeps(func() string { return "docker" }, func(_ context.Context, _ sandbox.Instance, name string, args []string, _ io.Writer) (int, error) {
		attempts++
		if name != "probe" || len(args) != 1 || args[0] != "ready" {
			t.Fatalf("RunBoxCommand() got name=%q args=%v", name, args)
		}
		if attempts == 1 {
			return 1, nil
		}
		return 0, nil
	}))
	rt.RememberPreparedGatewayProvision("u-manager", testPreparedGatewayProvision())

	box, info, err := rt.CreateGatewayBox(context.Background(), testSandboxRuntime{}, "image:1", "manager", "u-manager", agentruntime.Profile{})
	if err != nil {
		t.Fatalf("CreateGatewayBox() error = %v", err)
	}
	if box == nil || info.ID != "box-1" {
		t.Fatalf("CreateGatewayBox() got box=%v info=%+v", box, info)
	}
	if attempts != 2 {
		t.Fatalf("readiness attempts = %d, want 2", attempts)
	}
}

func TestCreateGatewayBoxSkipsReadinessForBoxlite(t *testing.T) {
	var attempts int
	rt := New(testGatewayDeps(func() string { return "boxlite" }, func(context.Context, sandbox.Instance, string, []string, io.Writer) (int, error) {
		attempts++
		return 0, nil
	}))
	rt.RememberPreparedGatewayProvision("u-manager", testPreparedGatewayProvision())

	if _, _, err := rt.CreateGatewayBox(context.Background(), testSandboxRuntime{}, "image:1", "manager", "u-manager", agentruntime.Profile{}); err != nil {
		t.Fatalf("CreateGatewayBox() error = %v", err)
	}
	if attempts != 0 {
		t.Fatalf("readiness attempts = %d, want 0 for boxlite", attempts)
	}
}

func TestStartWaitsForDockerReadiness(t *testing.T) {
	var attempts int
	rt := New(testGatewayDeps(func() string { return "docker" }, func(context.Context, sandbox.Instance, string, []string, io.Writer) (int, error) {
		attempts++
		return 0, nil
	}))

	state, err := rt.Start(context.Background(), agentruntime.Handle{RuntimeID: "rt-u-manager", HandleID: "box-1"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if state != agentruntime.StateRunning {
		t.Fatalf("Start() state = %q, want running", state)
	}
	if attempts != 1 {
		t.Fatalf("readiness attempts = %d, want 1", attempts)
	}
}

func TestDeleteStopsBoxBeforeForceRemove(t *testing.T) {
	var calls []string
	deps := testGatewayDeps(func() string { return "docker" }, func(context.Context, sandbox.Instance, string, []string, io.Writer) (int, error) {
		return 0, nil
	})
	deps.StopBox = func(context.Context, sandbox.Instance, sandbox.StopOptions) error {
		calls = append(calls, "stop")
		return nil
	}
	deps.ForceRemoveBox = func(context.Context, sandbox.Runtime, string) error {
		calls = append(calls, "remove")
		return nil
	}

	rt := New(deps)
	if err := rt.Delete(context.Background(), agentruntime.Handle{RuntimeID: "rt-u-manager", HandleID: "box-1"}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if strings.Join(calls, ",") != "stop,remove" {
		t.Fatalf("Delete() sandbox calls = %q, want stop then remove", strings.Join(calls, ","))
	}
}

func TestResolveSandboxToolsDirSupportsCSGHubProvider(t *testing.T) {
	toolsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(toolsDir, "csgclaw-cli"), []byte("cli"), 0o755); err != nil {
		t.Fatalf("WriteFile(csgclaw-cli) error = %v", err)
	}
	rt := New(Dependencies{
		SandboxProviderName: func() string { return config.CSGHubProvider },
		SandboxToolsDir:     func() (string, error) { return toolsDir, nil },
	})
	got, mount, err := rt.resolveSandboxToolsDir()
	if err != nil {
		t.Fatalf("resolveSandboxToolsDir() error = %v", err)
	}
	if !mount {
		t.Fatal("resolveSandboxToolsDir() mount = false, want true")
	}
	if got != toolsDir {
		t.Fatalf("resolveSandboxToolsDir() dir = %q, want %q", got, toolsDir)
	}
}

func testGatewayDeps(providerName func() string, run func(context.Context, sandbox.Instance, string, []string, io.Writer) (int, error)) Dependencies {
	return Dependencies{
		RuntimeKind:         agentruntime.KindOpenClawSandbox,
		SandboxProviderName: providerName,
		EnsureRuntime: func(string) (sandbox.Runtime, error) {
			return testSandboxRuntime{}, nil
		},
		RuntimeHome: func(string) (string, error) {
			return "/tmp/runtime", nil
		},
		CloseRuntime: func(string, sandbox.Runtime) error {
			return nil
		},
		ResolveBox: func(context.Context, sandbox.Runtime, AgentRef) (sandbox.Instance, string, error) {
			return testSandboxBox{}, "box-1", nil
		},
		CreateBox: func(context.Context, sandbox.Runtime, sandbox.CreateSpec) (sandbox.Instance, error) {
			return testSandboxBox{}, nil
		},
		StartBox: func(context.Context, sandbox.Instance) error {
			return nil
		},
		BoxInfo: func(context.Context, sandbox.Instance) (sandbox.Info, error) {
			return sandbox.Info{
				ID:    "box-1",
				Name:  "manager",
				State: sandbox.StateRunning,
			}, nil
		},
		CloseBox: func(sandbox.Instance) error {
			return nil
		},
		RunBoxCommand: run,
		ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
			return AgentRef{ID: "u-manager", Name: "manager", RuntimeID: "rt-u-manager", BoxID: "box-1"}, nil
		},
		SyncHandle: func(agentruntime.Handle) error {
			return nil
		},
		BuildRuntimeEnv: func(string, string, string, string, string, string, feishu.AgentCredentialProvider) map[string]string {
			return map[string]string{}
		},
		AddProfileEnv:      func(map[string]string, map[string]string) {},
		HomeEnv:            "/home/node",
		MountGuestPath:     "/home/node/.openclaw",
		WorkspaceGuestPath: "/home/node/.openclaw/workspace",
		ProjectsGuestPath:  "/home/node/.openclaw/workspace/projects",
		GatewayCommand: func() string {
			return "gateway"
		},
		ReadinessProbe: GatewayReadinessProbe{
			Name:     "probe",
			Args:     []string{"ready"},
			Timeout:  250 * time.Millisecond,
			Interval: time.Millisecond,
		},
	}
}

func testPreparedGatewayProvision() PreparedGatewayProvision {
	return PreparedGatewayProvision{
		ModelID: "model",
		Profile: agentruntime.Profile{
			ModelID: "model",
		},
		WorkspaceLayout: WorkspaceLayout{
			MountHostPath:      "/tmp/agent/.openclaw",
			MountGuestPath:     "/home/node/.openclaw",
			WorkspaceHostPath:  "/tmp/agent/.openclaw/workspace",
			WorkspaceGuestPath: "/home/node/.openclaw/workspace",
		},
		ProjectsRoot:   "/tmp/projects",
		ManagerBaseURL: "http://127.0.0.1:18080",
		Server: config.ServerConfig{
			AccessToken: "token",
		},
	}
}

func TestWaitForGatewayReadyReturnsLastError(t *testing.T) {
	rt := New(testGatewayDeps(func() string { return "docker" }, func(_ context.Context, _ sandbox.Instance, _ string, _ []string, _ io.Writer) (int, error) {
		return 1, fmt.Errorf("probe failed")
	}))

	err := rt.waitForGatewayReady(context.Background(), testSandboxBox{})
	if err == nil {
		t.Fatal("waitForGatewayReady() error = nil, want error")
	}
}

func TestWaitForGatewayReadyFailsWhenDockerBoxExits(t *testing.T) {
	deps := testGatewayDeps(func() string { return "docker" }, func(_ context.Context, _ sandbox.Instance, _ string, _ []string, _ io.Writer) (int, error) {
		return 126, fmt.Errorf("exec failed")
	})
	deps.BoxInfo = func(context.Context, sandbox.Instance) (sandbox.Info, error) {
		return sandbox.Info{
			ID:    "box-1",
			Name:  "manager",
			State: sandbox.StateExited,
		}, nil
	}
	rt := New(deps)

	err := rt.waitForGatewayReady(context.Background(), testSandboxBox{})
	if err == nil {
		t.Fatal("waitForGatewayReady() error = nil, want sandbox exited error")
	}
	if !strings.Contains(err.Error(), "sandbox exited before ready") {
		t.Fatalf("waitForGatewayReady() error = %v, want sandbox exited context", err)
	}
}
