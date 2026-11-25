package agents

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"testing"

	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/skill/installstate"
	skill "csgclaw/internal/skill/state"
	"csgclaw/internal/skill/system"
)

// testdata/pre-catalog/skill-installer is the actual tree from 7227930b,
// not a marker or an installer seeded by the binary under test.
func TestNormalStartupUpgradesOnlyStockSystemCopies(t *testing.T) {
	for _, kind := range []string{RuntimeKindCodex, RuntimeKindDSH} {
		t.Run(kind, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			stateFile := filepath.Join(t.TempDir(), "agents.json")
			rt := fakeAgentRuntime{kind: kind, info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateStopped}, nil
			}}
			var rows []Agent
			dirs := map[string]string{}
			for _, variant := range []string{"stock", "edited", "removed", "different-origin"} {
				id := "agent-" + variant
				dir := rt.Layout(filepath.Join(serviceAgentsRoot(stateFile), id)).SkillsRoot
				dirs[variant] = filepath.Join(dir, "skill-installer")
				rows = append(rows, Agent{ID: id, Name: variant, Role: RoleWorker, RuntimeKind: kind, RuntimeName: kind, RuntimeID: "rt-" + id, BoxID: "rt-" + id, Status: "stopped", DesiredState: DesiredStateRunning, ProfileComplete: true,
					AgentProfile: AgentProfile{Provider: ProviderAPI, BaseURL: "https://example.test", APIKey: "fixture", ModelID: "test", ProfileComplete: true}, SkillStates: map[string]skill.State{"skill-installer": {Enabled: false}}})
				if variant == "removed" {
					continue
				}
				if err := os.CopyFS(dirs[variant], os.DirFS("testdata/pre-catalog/skill-installer")); err != nil {
					t.Fatal(err)
				}
				if variant == "edited" {
					if err := os.WriteFile(filepath.Join(dirs[variant], "notes.txt"), []byte("my changes"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if variant == "different-origin" {
					if err := installstate.Update(dir, "skill-installer", &installstate.Record{Source: installstate.SourceLocal}, nil); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := writeSeededAgents(stateFile, rows); err != nil {
				t.Fatal(err)
			}
			svc, err := NewController(testModelConfig(), config.ServerConfig{}, "", stateFile, WithRuntime(rt))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = svc.Close() })
			if err := svc.StartConfiguredAgents(t.Context()); err != nil {
				t.Fatal(err)
			}
			source, err := system.ResolveSource("skill-installer")
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range []string{"SKILL.md", "scripts/install-skills-sh.mjs"} {
				want, err := fs.ReadFile(source.FS, path.Join(source.RootPath, file))
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(filepath.Join(dirs["stock"], file))
				if err != nil || !bytes.Equal(want, got) {
					t.Fatalf("ordinary upgrade did not install %s: %v", file, err)
				}
			}
			old, err := os.ReadFile("testdata/pre-catalog/skill-installer/SKILL.md")
			if err != nil {
				t.Fatal(err)
			}
			for _, variant := range []string{"edited", "different-origin"} {
				got, err := os.ReadFile(filepath.Join(dirs[variant], "SKILL.md"))
				if err != nil || !bytes.Equal(old, got) {
					t.Fatalf("overwrote %s", variant)
				}
			}
			if _, err := os.Stat(dirs["removed"]); !os.IsNotExist(err) {
				t.Fatal("restored intentionally missing skill")
			}
			for _, a := range rows {
				got, _ := svc.agentSnapshot(a.ID)
				if skill.Enabled(got.SkillStates, "skill-installer") {
					t.Fatal("migration enabled a disabled copy")
				}
			}
		})
	}
}
