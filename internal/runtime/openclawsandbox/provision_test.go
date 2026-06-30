package openclawsandbox

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
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
			WorkspaceTemplate: templateembed.OpenClawWorkerRoot,
		},
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(agentHome, HostDir, HostConfig)); err != nil {
		t.Fatalf("stat openclaw config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agentHome, HostDir, HostExecApproval)); err != nil {
		t.Fatalf("stat openclaw approvals: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agentHome, HostDir, HostGatewayLog)); err != nil {
		t.Fatalf("stat openclaw gateway log: %v", err)
	}
	approvalsRaw, err := os.ReadFile(filepath.Join(agentHome, HostDir, HostExecApproval))
	if err != nil {
		t.Fatalf("ReadFile(openclaw approvals) error = %v", err)
	}
	var approvals struct {
		Agents map[string]struct {
			Security  string `json:"security"`
			Ask       string `json:"ask"`
			Allowlist []struct {
				Pattern string `json:"pattern"`
			} `json:"allowlist"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(approvalsRaw, &approvals); err != nil {
		t.Fatalf("json.Unmarshal(openclaw approvals) error = %v", err)
	}
	wildcard := approvals.Agents["*"]
	if got, want := wildcard.Security, "full"; got != want {
		t.Fatalf("openclaw approvals agents.*.security = %q, want %q", got, want)
	}
	if got, want := wildcard.Ask, "off"; got != want {
		t.Fatalf("openclaw approvals agents.*.ask = %q, want %q", got, want)
	}
	if len(wildcard.Allowlist) != 1 || wildcard.Allowlist[0].Pattern != "*" {
		t.Fatalf("openclaw approvals agents.*.allowlist = %#v, want wildcard pattern", wildcard.Allowlist)
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

func TestWorkspaceLayoutForWindowsAvoidsMountingOpenClawHome(t *testing.T) {
	agentHome := filepath.Join("tmp", "agent-home")

	layout := workspaceLayoutForGOOS(agentHome, "windows")

	if got, want := layout.MountHostPath, workspaceRoot(agentHome); got != want {
		t.Fatalf("windows MountHostPath = %q, want workspace root %q", got, want)
	}
	if got, want := layout.MountGuestPath, BoxWindowsWorkspaceDir; got != want {
		t.Fatalf("windows MountGuestPath = %q, want isolated workspace guest path %q", got, want)
	}
	if got, want := layout.WorkspaceHostPath, workspaceRoot(agentHome); got != want {
		t.Fatalf("windows WorkspaceHostPath = %q, want %q", got, want)
	}
	wantMounts := []sandbox.Mount{{
		HostPath:  filepath.Join(Root(agentHome), HostConfig),
		GuestPath: BoxConfigPath,
		ReadOnly:  true,
	}, {
		HostPath:  filepath.Join(Root(agentHome), HostExecApproval),
		GuestPath: BoxExecApprovalPath,
		ReadOnly:  true,
	}, {
		HostPath:  filepath.Join(Root(agentHome), HostGatewayLog),
		GuestPath: BoxGatewayLogPath,
	}}
	if !reflect.DeepEqual(layout.ExtraMounts, wantMounts) {
		t.Fatalf("windows ExtraMounts = %+v, want readonly config and approvals plus writable log mount %+v", layout.ExtraMounts, wantMounts)
	}
}

func TestWorkspaceLayoutForNonWindowsMountsOpenClawHome(t *testing.T) {
	agentHome := filepath.Join("tmp", "agent-home")

	layout := workspaceLayoutForGOOS(agentHome, "linux")

	if got, want := layout.MountHostPath, Root(agentHome); got != want {
		t.Fatalf("linux MountHostPath = %q, want openclaw root %q", got, want)
	}
	if got, want := layout.MountGuestPath, BoxDir; got != want {
		t.Fatalf("linux MountGuestPath = %q, want openclaw home %q", got, want)
	}
	if len(layout.ExtraMounts) != 0 {
		t.Fatalf("linux ExtraMounts = %+v, want none", layout.ExtraMounts)
	}
}
