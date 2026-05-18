package picoclawsandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
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

	if _, err := os.Stat(filepath.Join(agentHome, "workspace", HostPicoClawStateDir, HostPicoClawConfig)); err != nil {
		t.Fatalf("stat picoclaw config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agentHome, "workspace", HostPicoClawStateDir, HostPicoClawSecurity)); err != nil {
		t.Fatalf("stat picoclaw security config: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(agentHome, "workspace", "USER.md"))
	if err != nil {
		t.Fatalf("ReadFile(USER.md) error = %v", err)
	}
	if got, want := string(data), "overlay user\n"; got != want {
		t.Fatalf("USER.md = %q, want %q", got, want)
	}
}
