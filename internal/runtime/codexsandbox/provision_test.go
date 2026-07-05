package codexsandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	templateembed "csgclaw/internal/template/embed"
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
		RuntimeID:        "rt-u-dev",
		AgentID:          "u-dev",
		AgentName:        "dev",
		ParticipantID:    "dev",
		WorkspaceOverlay: overlayRoot,
		Gateway: &agentruntime.GatewayProvision{
			ModelFallback:     "fallback-model",
			Server:            config.ServerConfig{AdvertiseBaseURL: "http://127.0.0.1:18080", AccessToken: "shared-token"},
			ManagerBaseURL:    "http://127.0.0.1:18080",
			AgentHome:         agentHome,
			ProjectsRoot:      projectsRoot,
			WorkspaceTemplate: templateembed.CodexSandboxWorkerRoot,
		},
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(Root(agentHome), HostConfig)); err != nil {
		t.Fatalf("stat codex sandbox config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(Root(agentHome), HostGatewayLog)); err != nil {
		t.Fatalf("stat codex sandbox gateway log: %v", err)
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
