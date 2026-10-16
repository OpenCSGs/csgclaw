package agents

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/config"
	skill "csgclaw/internal/skill/state"
)

func TestConnectorSkillsOwnershipAndCompleteReferences(t *testing.T) {
	for _, kind := range []string{RuntimeKindCodex, RuntimeKindDSH} {
		t.Run(kind, func(t *testing.T) {
			svc, err := NewController(testModelConfig(), config.ServerConfig{}, "", filepath.Join(t.TempDir(), "agents.json"), WithRuntime(fakeAgentRuntime{kind: kind}))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = svc.Close() })
			item, err := svc.CreateWorker(context.Background(), CreateAgentSpec{ID: "agent-cli", Name: "cli", RuntimeKind: kind, AgentProfile: AgentProfile{Name: "cli", Provider: ProviderAPI, BaseURL: "https://api.example/v1", APIKey: "test", ModelID: "test", ProfileComplete: true}})
			if err != nil {
				t.Fatal(err)
			}
			root, err := svc.ConnectorSkillsRoot(item.ID)
			if err != nil {
				t.Fatal(err)
			}
			bundle := t.TempDir()
			write := func(root, name, content string) {
				t.Helper()
				name = filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(name, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			write(bundle, "lark-test/SKILL.md", "official instructions")
			write(bundle, "lark-test/references/details.md", "complete references")
			write(bundle, "lark-manual/SKILL.md", "manual identical")
			write(bundle, "lark-remove/SKILL.md", "owned unchanged")
			write(root, "lark-manual/SKILL.md", "manual identical")
			write(root, "custom/SKILL.md", "user skill")
			plan, err := svc.PlanConnectorSkills(item.ID, bundle, "commit", nil, true)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range plan.After {
				if entry.Name == "lark-manual" && entry.Managed {
					t.Fatal("claimed a manual directory")
				}
			}
			if err := svc.ApplyConnectorSkills(t.Context(), item.ID, plan); err != nil {
				t.Fatal(err)
			}
			if err := svc.ApplyConnectorSkills(t.Context(), item.ID, plan); err != nil {
				t.Fatalf("repeat: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(root, "lark-test/references/details.md"))
			if err != nil || string(data) != "complete references" {
				t.Fatalf("reference=%q error=%v", data, err)
			}
			reverse := skill.SkillPlan{Before: plan.After, After: plan.Before}
			if err := svc.ApplyConnectorSkills(t.Context(), item.ID, reverse); err != nil {
				t.Fatalf("rollback: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "lark-manual/SKILL.md")); err != nil {
				t.Fatalf("manual skill after rollback: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "lark-test")); !os.IsNotExist(err) {
				t.Fatalf("connector skill after rollback: %v", err)
			}
			if err := svc.ApplyConnectorSkills(t.Context(), item.ID, plan); err != nil {
				t.Fatalf("retry after rollback: %v", err)
			}
			write(root, "lark-test/references/details.md", "user modified")
			if _, err := svc.PlanConnectorSkills(item.ID, bundle, "next", plan.After, true); err == nil || !strings.Contains(err.Error(), "skill_conflict") {
				t.Fatalf("conflict=%v", err)
			}
			disabled, err := svc.PlanConnectorSkills(item.ID, bundle, "commit", plan.After, false)
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.ApplyConnectorSkills(t.Context(), item.ID, disabled); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"custom/SKILL.md", "lark-manual/SKILL.md", "lark-test/references/details.md"} {
				if _, err := os.Stat(filepath.Join(root, name)); err != nil {
					t.Fatalf("removed user file %s: %v", name, err)
				}
			}
			if _, err := os.Stat(filepath.Join(root, "lark-remove")); !os.IsNotExist(err) {
				t.Fatalf("managed directory remains: %v", err)
			}
		})
	}
}
