package agents

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/config"
	"csgclaw/internal/dshcli"
	agentruntime "csgclaw/internal/runtime"
	runtimecodex "csgclaw/internal/runtime/codex"
	runtimedsh "csgclaw/internal/runtime/dsh"
	skilllocal "csgclaw/internal/skill/local"
)

type creationCodexSessionManager struct {
	recreateCodexSessionManager
	check func(runtimecodex.SessionSpec)
}

func (m *creationCodexSessionManager) Start(ctx context.Context, spec runtimecodex.SessionSpec) (*runtimecodex.Session, error) {
	m.check(spec)
	session, err := m.recreateCodexSessionManager.Start(ctx, spec)
	if err == nil {
		session.PreserveSkills = spec.PreserveSkills
		session.ProcessID = 0
	}
	return session, err
}

func TestCreateCodexPreservesSkillsThroughStartup(t *testing.T) {
	t.Cleanup(runtimecodex.TestOnlySetResponsesAPIProbe(func(context.Context, string, string, string, map[string]string) error {
		return nil
	}))
	for _, tc := range []struct {
		name   string
		skills []string
	}{
		{name: "builtin", skills: []string{"skill-creator"}},
		{name: "custom", skills: []string{"reviewer"}},
		{name: "empty", skills: []string{}},
		{name: "default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			globalRoot, err := skilllocal.SkillsRoot()
			if err != nil {
				t.Fatal(err)
			}
			customSkill := filepath.Join(globalRoot, "reviewer", "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(customSkill), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(customSkill, []byte("---\nname: reviewer\ndescription: Review changes\n---\nReview carefully.\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			hostHome := t.TempDir()
			t.Setenv("CODEX_HOME", hostHome)
			file := filepath.Join(hostHome, "skills", "host-extra", "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, []byte("# Host extra\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			manager := &creationCodexSessionManager{check: func(spec runtimecodex.SessionSpec) {
				entries, err := os.ReadDir(filepath.Join(spec.CodexHomeDir, "skills"))
				if err != nil {
					t.Fatal(err)
				}
				var names []string
				for _, entry := range entries {
					names = append(names, entry.Name())
				}
				if tc.skills != nil && !slices.Equal(names, tc.skills) {
					t.Fatalf("skills before Codex process startup = %v, want %v", names, tc.skills)
				}
				if tc.skills == nil && (!slices.Contains(names, "host-extra") || !slices.Contains(names, "agent-teams")) {
					t.Fatalf("omitted selection lost inherited skills: %v", names)
				}
			}}
			var svc *Controller
			rt := runtimecodex.New(runtimecodex.Dependencies{
				BinaryProvider: recreateCodexBinaryProvider{},
				Manager:        manager,
				AgentHome:      func(id string) (string, error) { return svc.agentHomeDir(id) },
				ResolveAgent: func(h agentruntime.Handle) (runtimecodex.AgentRef, error) {
					item, ok := svc.agentSnapshot("agent-reviewer")
					if !ok {
						return runtimecodex.AgentRef{}, fmt.Errorf("agent not persisted before startup")
					}
					return runtimecodex.AgentRef{ID: item.ID, Name: item.Name, RuntimeID: h.RuntimeID, HandleID: item.BoxID, Profile: svc.runtimeProfileForAgent(item), RuntimeOptions: item.RuntimeOptions}, nil
				},
			})
			svc, err = NewController(testModelConfig(), config.ServerConfig{ListenAddr: "127.0.0.1:18080"}, "", filepath.Join(t.TempDir(), "agents.json"), WithRuntime(rt))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = svc.Close() })
			ctx := context.Background()
			created, err := svc.Create(ctx, contract.AgentCreateRequest{
				ID: "agent-reviewer",
				Spec: contract.AgentSpec{
					Name: "reviewer", Skills: tc.skills, Runtime: contract.RuntimeSpec{Adapter: RuntimeNameCodex},
					Model: contract.ModelSpec{Provider: ProviderAPI, BaseURL: "https://api.example/v1", APIKey: "test", ModelID: "test"},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if tc.skills != nil && !slices.Equal(created.Spec.Skills, tc.skills) {
				t.Fatalf("saved skills = %v, want %v", created.Spec.Skills, tc.skills)
			}
			if _, err := svc.Stop(ctx, created.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.Start(ctx, created.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCreateDSHAppliesSkillsBeforeStartup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ACP test helper uses a POSIX launcher")
	}
	for _, tc := range []struct {
		name   string
		skills []string
	}{
		{name: "custom", skills: []string{"reviewer"}},
		{name: "empty", skills: []string{}},
		{name: "default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("GO_WANT_CREATION_DSH_HELPER", "1")
			t.Setenv("DSH_TEST_BINARY", os.Args[0])
			launcher := filepath.Join(t.TempDir(), "dsh-test")
			if err := os.WriteFile(launcher, []byte("#!/bin/sh\nexec \"$DSH_TEST_BINARY\" -test.run=^TestCreationDSHHelperProcess$\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			globalRoot, err := skilllocal.SkillsRoot()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(globalRoot, "reviewer"), 0o755); err != nil {
				t.Fatal(err)
			}
			const content = "---\nname: reviewer\ndescription: Review changes\n---\nReview carefully.\n"
			if err := os.WriteFile(filepath.Join(globalRoot, "reviewer", "SKILL.md"), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			var svc *Controller
			rt := runtimedsh.New(runtimedsh.Dependencies{
				ResolveBinary: func(context.Context) (dshcli.Info, error) {
					return dshcli.Info{Path: launcher, Version: "0.1.5-rc.2"}, nil
				},
				AgentHome: func(id string) (string, error) { return svc.agentHomeDir(id) },
				ResolveAgent: func(agentruntime.Handle) (runtimedsh.AgentRef, error) {
					item, ok := svc.agentSnapshot("agent-reviewer")
					if !ok {
						return runtimedsh.AgentRef{}, fmt.Errorf("agent not persisted before startup")
					}
					return runtimedsh.AgentRef{
						ID: item.ID, Name: item.Name, RuntimeID: item.RuntimeID, Instructions: item.Instructions,
						RuntimeOptions: item.RuntimeOptions, MCPServers: item.MCPServers, SkillStates: item.SkillStates,
						Profile: svc.runtimeProfileForKind(item.RuntimeKind, item.ID, item.Name, item.Description, item.AgentProfile),
					}, nil
				},
			})
			svc, err = NewController(testModelConfig(), config.ServerConfig{ListenAddr: "127.0.0.1:18080"}, "", filepath.Join(t.TempDir(), "agents.json"), WithRuntime(rt))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = svc.Close() })
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			created, err := svc.Create(ctx, contract.AgentCreateRequest{
				ID: "agent-reviewer",
				Spec: contract.AgentSpec{
					Name: "reviewer", Skills: tc.skills, Runtime: contract.RuntimeSpec{Adapter: RuntimeNameDSH},
					Model: contract.ModelSpec{Provider: ProviderAPI, BaseURL: "https://api.example/v1", APIKey: "test", ModelID: "test"},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if tc.skills != nil && !slices.Equal(created.Spec.Skills, tc.skills) {
				t.Fatalf("saved skills = %v, want %v", created.Spec.Skills, tc.skills)
			}
			if tc.skills == nil && !slices.Contains(created.Spec.Skills, "skill-installer") {
				t.Fatalf("omitted selection lost default skills: %v", created.Spec.Skills)
			}
			home, err := svc.agentHomeDir(created.ID)
			if err != nil {
				t.Fatal(err)
			}
			checkView := func() {
				t.Helper()
				entries, err := os.ReadDir(filepath.Join(home, ".dsh", "skill-view", "skills"))
				if err != nil {
					t.Fatal(err)
				}
				var names []string
				for _, entry := range entries {
					names = append(names, entry.Name())
				}
				if !slices.Equal(names, created.Spec.Skills) {
					t.Fatalf("active DSH skills = %v, saved selection = %v", names, created.Spec.Skills)
				}
			}
			checkView()
			if _, err := svc.Start(ctx, created.ID); err != nil {
				t.Fatal(err)
			}
			checkView()
		})
	}
}

func TestCreationDSHHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CREATION_DSH_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var frame struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal(scanner.Bytes(), &frame) != nil {
			os.Exit(2)
		}
		result := map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{"mcpCapabilities": map[string]any{"http": true}}}
		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": frame.ID, "result": result}); err != nil {
			os.Exit(3)
		}
	}
	os.Exit(0)
}
