package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	agentruntime "csgclaw/internal/runtime"
)

func TestRuntimeRestartPreservesHostSkillSnapshot(t *testing.T) {
	for _, agentID := range []string{"agent-alice", agent.ManagerUserID} {
		for _, operation := range []string{"start", "restore", "bootstrap"} {
			name := agentID + "/" + operation
			t.Run(name, func(t *testing.T) {
				withAppServerHelperCommand(t, "resume-success")
				hostHome := t.TempDir()
				t.Setenv("CODEX_HOME", hostHome)
				hostSkills := filepath.Join(hostHome, "skills")
				writeSkill := func(root, name, content string) {
					t.Helper()
					file := filepath.Join(root, name, "SKILL.md")
					if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				writeSkill(hostSkills, "removed", "host original\n")
				writeSkill(hostSkills, "edited", "host original\n")
				root := t.TempDir()
				deps := Dependencies{
					BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
					AgentHome:      func(id string) (string, error) { return filepath.Join(root, id), nil },
					ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
						return AgentRef{ID: agentID, Name: "alice", RuntimeID: h.RuntimeID}, nil
					},
				}
				rt := New(deps)
				t.Cleanup(func() { _ = rt.Close() })
				req := agentruntime.ProvisionRequest{AgentID: agentID, AgentName: "alice"}
				if err := rt.Provision(context.Background(), req); err != nil {
					t.Fatal(err)
				}
				spec := agentruntime.Spec{AgentID: agentID, AgentName: "alice", RuntimeID: "rt-" + agentID}
				handle, err := rt.New(context.Background(), spec)
				if err != nil {
					t.Fatal(err)
				}
				skills := rt.Layout(filepath.Join(root, agentID)).SkillsRoot
				assertRuntimeSkillFile(t, filepath.Join(skills, "removed", "SKILL.md"), "host original\n", 0o644)
				if _, err := rt.Stop(context.Background(), handle); err != nil {
					t.Fatal(err)
				}
				if err := os.RemoveAll(filepath.Join(skills, "removed")); err != nil {
					t.Fatal(err)
				}
				writeSkill(skills, "edited", "agent edit\n")
				writeSkill(skills, "custom", "agent custom\n")
				writeSkill(hostSkills, "edited", "host update\n")
				writeSkill(hostSkills, "new-host", "host added\n")
				if agentID == agent.ManagerUserID {
					if err := os.RemoveAll(filepath.Join(skills, "agent-creator")); err != nil {
						t.Fatal(err)
					}
				}

				if operation == "restore" {
					rt = New(deps)
					t.Cleanup(func() { _ = rt.Close() })
					if _, err := rt.SessionManager().Session(SessionHandle{RuntimeID: spec.RuntimeID}); err != nil {
						t.Fatal(err)
					}
				} else if operation == "bootstrap" {
					if _, err := rt.New(context.Background(), spec); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := rt.Provision(context.Background(), req); err != nil {
						t.Fatal(err)
					}
					if _, err := rt.Start(context.Background(), handle); err != nil {
						t.Fatal(err)
					}
				}
				for _, name := range []string{"removed", "new-host"} {
					if _, err := os.Stat(filepath.Join(skills, name)); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("重启后出现 host 技能 %q：%v", name, err)
					}
				}
				assertRuntimeSkillFile(t, filepath.Join(skills, "edited", "SKILL.md"), "agent edit\n", 0o644)
				assertRuntimeSkillFile(t, filepath.Join(skills, "custom", "SKILL.md"), "agent custom\n", 0o644)
				if agentID == agent.ManagerUserID {
					if _, err := os.Stat(filepath.Join(skills, "agent-creator", "SKILL.md")); err != nil {
						t.Fatalf("模板技能未同步：%v", err)
					}
				}

				if _, err := rt.Stop(context.Background(), handle); err != nil {
					t.Fatal(err)
				}
				if err := rt.Delete(context.Background(), handle); err != nil {
					t.Fatal(err)
				}
				if _, err := rt.New(context.Background(), spec); err != nil {
					t.Fatal(err)
				}
				assertRuntimeSkillFile(t, filepath.Join(skills, "new-host", "SKILL.md"), "host added\n", 0o644)
				assertRuntimeSkillFile(t, filepath.Join(skills, "edited", "SKILL.md"), "host update\n", 0o644)
			})
		}
	}
}
