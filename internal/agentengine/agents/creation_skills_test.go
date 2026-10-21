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
	runtimedsh "csgclaw/internal/runtime/dsh"
	skilllocal "csgclaw/internal/skill/local"
)

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
