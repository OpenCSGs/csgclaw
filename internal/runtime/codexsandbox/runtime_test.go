package codexsandbox

import (
	"path/filepath"
	"strings"
	"testing"

	agentruntime "csgclaw/internal/runtime"
)

func TestGatewayRunCommandUsesCodexGatewayBinaryAndLog(t *testing.T) {
	cmd := GatewayRunCommand()
	if !strings.Contains(cmd, "/usr/local/bin/csgclaw-codex-gateway") {
		t.Fatalf("GatewayRunCommand() = %q, want csgclaw-codex-gateway", cmd)
	}
	if !strings.Contains(cmd, BoxGatewayLogPath) {
		t.Fatalf("GatewayRunCommand() = %q, want log path %q", cmd, BoxGatewayLogPath)
	}
}

func TestLayoutUsesCodexSandboxWorkspace(t *testing.T) {
	agentHome := t.TempDir()
	rt := New(Dependencies{})
	layout := rt.Layout(agentHome)
	if got, want := layout.WorkspaceRoot, workspaceRoot(agentHome); got != want {
		t.Fatalf("WorkspaceRoot = %q, want %q", got, want)
	}
	if got, want := layout.SkillsRoot, filepath.Join(workspaceRoot(agentHome), "skills"); got != want {
		t.Fatalf("SkillsRoot = %q, want %q", got, want)
	}
	if got, want := rt.GatewayLogPath(), BoxGatewayLogPath; got != want {
		t.Fatalf("GatewayLogPath() = %q, want %q", got, want)
	}
}

func TestRestartRequiredWhenRuntimeProfileChanges(t *testing.T) {
	rt := New(Dependencies{})
	got, err := rt.RestartRequired(agentruntime.RuntimeConfigChange{
		Previous: agentruntime.RuntimeConfigSnapshot{Profile: agentruntime.RuntimeProfileConfig{
			BaseURL: "http://127.0.0.1:18080/api/v1/agents/u-dev/llm",
			APIKey:  "token",
			ModelID: "gpt-5.4",
		}},
		Current: agentruntime.RuntimeConfigSnapshot{Profile: agentruntime.RuntimeProfileConfig{
			BaseURL: "http://127.0.0.1:18080/api/v1/agents/u-dev/llm",
			APIKey:  "token",
			ModelID: "gpt-5.5",
		}},
	})
	if err != nil {
		t.Fatalf("RestartRequired() error = %v", err)
	}
	if !got {
		t.Fatal("RestartRequired() = false, want true when model changes")
	}

	got, err = rt.RestartRequired(agentruntime.RuntimeConfigChange{
		Previous: agentruntime.RuntimeConfigSnapshot{Profile: agentruntime.RuntimeProfileConfig{
			BaseURL: "http://127.0.0.1:18080/api/v1/agents/u-dev/llm",
			APIKey:  "token",
			ModelID: "gpt-5.5",
		}},
		Current: agentruntime.RuntimeConfigSnapshot{Profile: agentruntime.RuntimeProfileConfig{
			BaseURL: "http://127.0.0.1:18080/api/v1/agents/u-dev/llm/",
			APIKey:  "token",
			ModelID: "gpt-5.5",
		}},
	})
	if err != nil {
		t.Fatalf("RestartRequired() unchanged error = %v", err)
	}
	if got {
		t.Fatal("RestartRequired() = true, want false for equivalent profile")
	}
}
