package dsh

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentruntime "csgclaw/internal/runtime"
)

func TestProvisionStoresDSHInstructionsInHome(t *testing.T) {
	for _, test := range []struct {
		name     string
		template string
		wantBase string
	}{
		{name: "embedded template", wantBase: "CSGClaw DSH Worker"},
		{name: "custom template", template: "Custom template instructions", wantBase: "Custom template instructions"},
	} {
		t.Run(test.name, func(t *testing.T) {
			agentHome := t.TempDir()
			rt := New(Dependencies{AgentHome: func(string) (string, error) { return agentHome, nil }})
			err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
				AgentID: "agent-test", Instructions: "Agent-specific instructions", TemplateInstructions: test.template,
				Profile: agentruntime.Profile{BaseURL: "https://gateway.example/v1", APIKey: "token", ModelID: "test-model"},
			})
			if err != nil {
				t.Fatal(err)
			}
			layout := rt.Layout(agentHome)
			if want := filepath.Join(agentHome, hostStateDirName, homeDirName, "AGENTS.md"); layout.InstructionsPath != want {
				t.Fatalf("InstructionsPath = %q, want %q", layout.InstructionsPath, want)
			}
			contents, err := os.ReadFile(layout.InstructionsPath)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{test.wantBase, "Agent-specific instructions"} {
				if !strings.Contains(string(contents), want) {
					t.Fatalf("DSH home instructions are missing %q", want)
				}
			}
			if _, err := os.Stat(filepath.Join(layout.WorkspaceRoot, "AGENTS.md")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("managed workspace contains a duplicate AGENTS.md: %v", err)
			}
		})
	}
}

func TestProvisionPreservesWorkspaceInstructions(t *testing.T) {
	agentHome := t.TempDir()
	overlay := t.TempDir()
	projectInstructions := "Project-specific instructions\n"
	if err := os.WriteFile(filepath.Join(overlay, "AGENTS.md"), []byte(projectInstructions), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := New(Dependencies{AgentHome: func(string) (string, error) { return agentHome, nil }})
	if err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
		AgentID: "agent-test", Instructions: "Agent-specific instructions", WorkspaceOverlay: overlay,
		Profile: agentruntime.Profile{BaseURL: "https://gateway.example/v1", APIKey: "token", ModelID: "test-model"},
	}); err != nil {
		t.Fatal(err)
	}
	layout := rt.Layout(agentHome)
	project, err := os.ReadFile(filepath.Join(layout.WorkspaceRoot, "AGENTS.md"))
	if err != nil || string(project) != projectInstructions {
		t.Fatalf("workspace instructions = %q, %v", project, err)
	}
	home, err := os.ReadFile(layout.InstructionsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CSGClaw DSH Worker", "Agent-specific instructions"} {
		if !strings.Contains(string(home), want) {
			t.Fatalf("DSH home instructions are missing %q", want)
		}
	}
}
