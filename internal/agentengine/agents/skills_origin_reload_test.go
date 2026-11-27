package agents

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/config"
	"csgclaw/internal/resourcequery"
	skillinstall "csgclaw/internal/skill/installstate"
	skilllocal "csgclaw/internal/skill/local"
)

func TestSkillOriginLifecycleSurvivesControllerReload(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	statePath := filepath.Join(t.TempDir(), "agents.json")
	open := func() *Controller {
		t.Helper()
		controller, err := NewController(testModelConfig(), config.ServerConfig{}, "", statePath)
		if err != nil {
			t.Fatal(err)
		}
		return controller
	}
	svc := open()
	t.Cleanup(func() { _ = svc.Close() })
	item := Agent{ID: "agent-origin", Name: "origin", Role: RoleWorker, RuntimeKind: RuntimeKindCodex}
	svc.agents[item.ID] = item
	if err := svc.saveLocked(); err != nil {
		t.Fatal(err)
	}
	reload := func() {
		t.Helper()
		if err := svc.Close(); err != nil {
			t.Fatal(err)
		}
		svc = open()
	}
	globalRoot, err := skilllocal.SkillsRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"market", "legacy"} {
		writeWorkspaceFileAt(t, globalRoot, name+"/SKILL.md", "---\ndescription: example\n---\n", 0600)
	}
	if err := skillinstall.Update(globalRoot, "market", &skillinstall.Record{Source: skillinstall.SourceMarket, SourceURL: "https://example.test/skills/market"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.BatchAddSkills(item.ID, []string{"market", "legacy", "skill-creator"}); err != nil {
		t.Fatal(err)
	}
	assertSources := func(want map[string]string) {
		t.Helper()
		for _, paged := range []bool{false, true} {
			items, err := svc.Workspace().SkillSummaries(context.Background(), item.ID)
			if paged {
				items, _, err = svc.Workspace().SkillSummariesPage(context.Background(), item.ID, resourcequery.Query{Mode: "page", Page: 1, Size: 20})
			}
			if err != nil || len(items) != len(want) {
				t.Fatalf("summaries = %+v, error = %v", items, err)
			}
			for _, summary := range items {
				if summary.Source != want[summary.Name] {
					t.Fatalf("source of %s = %q, want %q", summary.Name, summary.Source, want[summary.Name])
				}
			}
		}
	}
	reload()
	assertSources(map[string]string{"market": skillinstall.SourceMarket, "legacy": skillinstall.SourceUnknown, "skill-creator": skillinstall.SourceBuiltin})
	if err := skillinstall.Update(globalRoot, "market", &skillinstall.Record{Source: skillinstall.SourceLocal}, nil); err != nil {
		t.Fatal(err)
	}
	reload()
	assertSources(map[string]string{"market": skillinstall.SourceMarket, "legacy": skillinstall.SourceUnknown, "skill-creator": skillinstall.SourceBuiltin})
	if err := svc.ReplaceSkills(context.Background(), item.ID, []string{"market", "legacy"}); err != nil {
		t.Fatal(err)
	}
	reload()
	assertSources(map[string]string{"market": skillinstall.SourceLocal, "legacy": skillinstall.SourceUnknown})
	if err := svc.DeleteSkill(item.ID, "market"); err != nil {
		t.Fatal(err)
	}
	reload()
	assertSources(map[string]string{"legacy": skillinstall.SourceUnknown})
	layout, err := svc.AgentLayout(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	records, err := skillinstall.Read(layout.SkillsRoot)
	if _, exists := records["market"]; err != nil || exists {
		t.Fatalf("removed origin remains after reload: error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(layout.SkillsRoot, ".skillhub-lock.json"), []byte("invalid JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	reload()
	assertSources(map[string]string{"legacy": skillinstall.SourceUnknown})
}

func TestControllerReloadRecoversSkillOriginsAtInterruptionPoints(t *testing.T) {
	for _, interruption := range []string{"prepared", "copy-journal", "restored", "copy-removed"} {
		t.Run(interruption, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			statePath := filepath.Join(t.TempDir(), "agents.json")
			svc, err := NewController(testModelConfig(), config.ServerConfig{}, "", statePath)
			if err != nil {
				t.Fatal(err)
			}
			root, err := svc.agentSkillsRoot("agent-origin", RuntimeKindCodex)
			if err != nil {
				t.Fatal(err)
			}
			writeWorkspaceFileAt(t, root, "custom/SKILL.md", "original custom skill", 0600)
			if err := skillinstall.Update(root, "custom", &skillinstall.Record{Source: skillinstall.SourceMarket}, nil); err != nil {
				t.Fatal(err)
			}
			recordsPath := filepath.Join(root, ".skillhub-lock.json")
			original, err := os.ReadFile(recordsPath)
			if err != nil {
				t.Fatal(err)
			}
			preserved, err := svc.prepareWorkspaceSkillsPreservation("agent-origin", RuntimeKindCodex, RuntimeKindCodex, RoleWorker)
			if err != nil || preserved == nil {
				t.Fatalf("prepare skills: %v", err)
			}
			if err := svc.installDefaultSystemSkills("agent-origin", RuntimeKindCodex); err != nil {
				t.Fatal(err)
			}
			switch interruption {
			case "copy-journal":
				preserved.phase = workspaceSkillsStateApplying
				preserved.appliedRoot = root
				if err := os.MkdirAll(preserved.displacedRoot, 0700); err != nil {
					t.Fatal(err)
				}
				for index := range preserved.entries {
					entry := &preserved.entries[index]
					if !skillinstall.IsRecordsFile(entry.Name) {
						continue
					}
					entry.Displaced, entry.Restored, entry.Copied = true, true, true
					if err := os.Rename(recordsPath, filepath.Join(preserved.displacedRoot, entry.Name)); err != nil {
						t.Fatal(err)
					}
				}
				if err := preserved.persistState(); err != nil {
					t.Fatal(err)
				}
			case "restored", "copy-removed":
				if err := preserved.Restore(); err != nil {
					t.Fatal(err)
				}
				if interruption == "copy-removed" {
					if err := os.Remove(recordsPath); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := svc.Close(); err != nil {
				t.Fatal(err)
			}
			svc, err = NewController(testModelConfig(), config.ServerConfig{}, "", statePath)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = svc.Close() })
			after, err := os.ReadFile(recordsPath)
			if err != nil || !bytes.Equal(after, original) {
				t.Fatalf("origins changed after recovery: error = %v", err)
			}
			contents, err := os.ReadFile(filepath.Join(root, "custom", "SKILL.md"))
			if err != nil || string(contents) != "original custom skill" {
				t.Fatalf("custom skill changed after recovery: error = %v", err)
			}
			if _, err := os.Stat(preserved.tempRoot); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("recovered transaction remains: %v", err)
			}
		})
	}
}

func TestWorkerOnlySkillReplacementDoesNotReadGlobalOrigins(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "", filepath.Join(t.TempDir(), "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	item := Agent{ID: "agent-origin", Name: "origin", Role: RoleWorker, RuntimeKind: RuntimeKindCodex}
	svc.agents[item.ID] = item
	layout, err := svc.AgentLayout(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFileAt(t, layout.SkillsRoot, "custom/SKILL.md", "custom skill", 0600)
	globalRoot, err := skilllocal.SkillsRoot()
	if err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFileAt(t, globalRoot, ".skillhub-lock.json", "invalid JSON", 0600)
	const id = "team/tools/review"
	writeWorkspaceFileAt(t, layout.SkillsRoot, id+"/SKILL.md", "# Review", 0600)
	if err := svc.ReplaceSkills(t.Context(), item.ID, []string{"team", id}); !errors.Is(err, ErrAgentSkillInvalid) {
		t.Fatalf("overlapping selection: %v", err)
	}
	if err := svc.ReplaceSkills(t.Context(), item.ID, []string{id}); err != nil {
		t.Fatalf("grouped skill needs unrelated global origins: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(layout.SkillsRoot, id, "SKILL.md")); err != nil || string(data) != "# Review" {
		t.Fatalf("grouped skill = %q, %v", data, err)
	}
	if err := svc.ReplaceSkills(context.Background(), item.ID, nil); err != nil {
		t.Fatalf("empty skill replacement depends on unrelated global origins: %v", err)
	}
	names, err := svc.Skills(item.ID)
	if err != nil || len(names) != 0 {
		t.Fatalf("skills after empty replacement = %v, error = %v", names, err)
	}
}

func TestPreparedReplacementPreservesSkillGroup(t *testing.T) {
	root := t.TempDir()
	stage := t.TempDir()
	const manifest = "team/tools/review/SKILL.md"
	writeWorkspaceFileAt(t, root, manifest, "# Review", 0600)
	writeWorkspaceFileAt(t, stage, "next/team/SKILL.md", "# Team", 0600)
	p := preparedSkillReplacement{targetRoot: root, stageRoot: stage, names: []string{"team"}, origins: map[string]*skillinstall.Record{"team": {Source: skillinstall.SourceLocal}}}
	if err := p.apply(); !errors.Is(err, ErrAgentSkillAlreadyExists) {
		t.Fatalf("replacement error = %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(root, manifest)); err != nil || string(data) != "# Review" {
		t.Fatalf("grouped skill = %q, %v", data, err)
	}
	records, err := skillinstall.Read(root)
	if _, exists := records["team"]; err != nil || exists {
		t.Fatalf("rejected origin persisted: %+v, %v", records, err)
	}
}

func TestDefaultSystemSkillsPreserveGroupWithSameName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "", filepath.Join(t.TempDir(), "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	root, err := svc.agentSkillsRoot("group-worker", RuntimeKindCodex)
	if err != nil {
		t.Fatal(err)
	}
	const manifest = "skill-creator/review/SKILL.md"
	writeWorkspaceFileAt(t, root, manifest, "# Review", 0600)
	if err := svc.installDefaultSystemSkills("group-worker", RuntimeKindCodex); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(root, manifest)); err != nil || string(data) != "# Review" {
		t.Fatalf("grouped skill = %q, %v", data, err)
	}
	records, err := skillinstall.Read(root)
	if _, exists := records["skill-creator"]; err != nil || exists || records["skill-installer"].Source != skillinstall.SourceBuiltin {
		t.Fatalf("default skill records = %+v, %v", records, err)
	}
}
