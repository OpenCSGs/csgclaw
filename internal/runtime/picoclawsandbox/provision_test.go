package picoclawsandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox/hostuser"
	"csgclaw/internal/templates"
)

func TestProvisionPreparesGatewayAssets(t *testing.T) {
	agentHome := t.TempDir()
	projectsRoot := t.TempDir()
	overlayRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(overlayRoot, "USER.md"), []byte("overlay user\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(USER.md) error = %v", err)
	}

	rt := New(Dependencies{})

	if err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
		RuntimeID:        "rt-1",
		AgentID:          "u-alice",
		AgentName:        "alice",
		Profile:          agentruntime.Profile{},
		WorkspaceOverlay: overlayRoot,
		Gateway: &agentruntime.GatewayProvision{
			ModelFallback:     "fallback-model",
			Server:            config.ServerConfig{AdvertiseBaseURL: "http://127.0.0.1:18080", AccessToken: "shared-token"},
			ManagerBaseURL:    "http://127.0.0.1:18080",
			AgentHome:         agentHome,
			ProjectsRoot:      projectsRoot,
			WorkspaceTemplate: templates.PicoClawWorkerRoot,
		},
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(Root(agentHome), HostConfig)); err != nil {
		t.Fatalf("stat picoclaw config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(Root(agentHome), HostSecurity)); err != nil {
		t.Fatalf("stat picoclaw security config: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(workspaceRoot(agentHome), "USER.md"))
	if err != nil {
		t.Fatalf("ReadFile(USER.md) error = %v", err)
	}
	if got, want := string(data), "overlay user\n"; got != want {
		t.Fatalf("USER.md = %q, want %q", got, want)
	}
	if info, err := os.Stat(filepath.Join(workspaceRoot(agentHome), "projects")); err != nil {
		t.Fatalf("stat workspace projects mountpoint: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("workspace projects mountpoint is not a directory")
	}
}

func TestGatewayCreateSpecMountsPicoClawRuntimeRoot(t *testing.T) {
	agentHome := t.TempDir()
	projectsRoot := t.TempDir()
	rt := New(Dependencies{
		BuildRuntimeEnv: func(_, _, _, _, _ string, _ feishu.BotCredentialProvider) map[string]string {
			return map[string]string{}
		},
		AddProfileEnv: func(map[string]string, map[string]string) {},
	})

	if err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
		RuntimeID: "rt-1",
		AgentID:   "u-alice",
		AgentName: "alice",
		Gateway: &agentruntime.GatewayProvision{
			ModelFallback:     "fallback-model",
			Server:            config.ServerConfig{AdvertiseBaseURL: "http://127.0.0.1:18080", AccessToken: "shared-token"},
			ManagerBaseURL:    "http://127.0.0.1:18080",
			AgentHome:         agentHome,
			ProjectsRoot:      projectsRoot,
			WorkspaceTemplate: templates.PicoClawWorkerRoot,
		},
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	spec, err := rt.GatewayCreateSpec("picoclaw:test", "alice", "u-alice", agentruntime.Profile{})
	if err != nil {
		t.Fatalf("GatewayCreateSpec() error = %v", err)
	}
	if len(spec.Mounts) != 2 {
		t.Fatalf("GatewayCreateSpec() mounts = %+v, want 2 mounts", spec.Mounts)
	}
	if got, want := spec.Mounts[0].HostPath, Root(agentHome); got != want {
		t.Fatalf("runtime root mount host = %q, want %q", got, want)
	}
	if got, want := spec.Mounts[0].GuestPath, BoxDir; got != want {
		t.Fatalf("runtime root mount guest = %q, want %q", got, want)
	}
	if got, want := spec.Mounts[1].HostPath, projectsRoot; got != want {
		t.Fatalf("projects mount host = %q, want %q", got, want)
	}
	if got, want := spec.Mounts[1].GuestPath, BoxProjectsDir; got != want {
		t.Fatalf("projects mount guest = %q, want %q", got, want)
	}
	cmd := strings.Join(spec.Cmd, " ")
	if strings.Contains(cmd, "/csgclaw-projects") || strings.Contains(cmd, "ln -sfn") {
		t.Fatalf("GatewayCreateSpec() cmd = %q, want direct projects mount without symlink setup", spec.Cmd)
	}
	runUser, err := hostuser.RunUser()
	if err != nil {
		t.Skip("host uid/gid unavailable")
	}
	if spec.RunUser != runUser {
		t.Fatalf("GatewayCreateSpec() RunUser = %q, want %q", spec.RunUser, runUser)
	}
}
