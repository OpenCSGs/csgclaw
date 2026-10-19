package agents

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"testing"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	skillsystem "csgclaw/internal/skill/system"
)

func assertDefaultSystemSkills(t *testing.T, root string) {
	t.Helper()
	for _, name := range []string{"skill-creator", "skill-installer", "csgclaw-self-diagnosis"} {
		source, err := skillsystem.ResolveSource(name)
		if err != nil {
			t.Fatal(err)
		}
		want, err := fs.ReadFile(source.FS, path.Join(source.RootPath, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(root, name, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("内建技能 %s 内容与当前版本不同", name)
		}
	}
}

func TestHostWorkerInstallsSystemSkillsBeforeCreateAndRecreate(t *testing.T) {
	for _, kind := range []string{RuntimeKindCodex, RuntimeKindDSH} {
		t.Run(kind, func(t *testing.T) {
			var svc *Controller
			var root string
			provisions, starts := 0, 0
			rt := fakeAgentRuntime{
				kind:      kind,
				provision: func(context.Context, agentruntime.ProvisionRequest) error { provisions++; return nil },
				del: func(context.Context, agentruntime.Handle) error {
					// 重建删除旧目录前，Provision 应当保留技能修改。
					data, err := os.ReadFile(filepath.Join(root, "skill-creator", "SKILL.md"))
					if err != nil {
						t.Fatal(err)
					}
					if string(data) != "old version" {
						t.Fatal("重建在删除旧目录前重复安装了内建技能")
					}
					if _, err := os.Stat(filepath.Join(root, "skill-installer")); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("重建提前恢复了已删除的内建技能：%v", err)
					}
					return os.RemoveAll(root)
				},
				new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
					var err error
					root, err = svc.agentSkillsRoot(spec.AgentID, kind)
					if err != nil {
						t.Fatal(err)
					}
					assertDefaultSystemSkills(t, root)
					starts++
					return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "test-session"}, nil
				},
				info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
					return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
				},
			}
			var err error
			svc, err = NewController(testModelConfig(), config.ServerConfig{}, "", filepath.Join(t.TempDir(), "agents.json"), WithRuntime(rt))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = svc.Close() })
			item, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
				ID: "agent-alice", Name: "alice", RuntimeKind: kind,
				AgentProfile: AgentProfile{Name: "alice", Provider: ProviderAPI, BaseURL: "https://api.example/v1", APIKey: "test", ModelID: "test", ProfileComplete: true},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "custom"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "custom", "SKILL.md"), []byte("custom skill"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "skill-creator", "SKILL.md"), []byte("old version"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(filepath.Join(root, "skill-installer")); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.RecreateRecord(context.Background(), item.ID); err != nil {
				t.Fatal(err)
			}
			assertDefaultSystemSkills(t, root)
			data, err := os.ReadFile(filepath.Join(root, "custom", "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != "custom skill" {
				t.Fatal("自定义技能内容发生变化")
			}
			if provisions != 2 || starts != 2 {
				t.Fatalf("创建和重建调用次数：Provision=%d，New=%d", provisions, starts)
			}
		})
	}
}

func TestHostRuntimeUpdatesPreserveSkillSelection(t *testing.T) {
	for _, kind := range []string{RuntimeKindCodex, RuntimeKindDSH} {
		for _, state := range []agentruntime.State{agentruntime.StateRunning, agentruntime.StateStopped} {
			for _, combined := range []bool{false, true} {
				name := kind + "/" + string(state) + "/config"
				if combined {
					name += "-and-skills"
				}
				t.Run(name, func(t *testing.T) {
					runtimeState := state
					provisions := 0
					svc, err := NewController(testModelConfig(), config.ServerConfig{}, "", filepath.Join(t.TempDir(), "agents.json"), WithRuntime(fakeAgentRuntime{
						kind:      kind,
						provision: func(context.Context, agentruntime.ProvisionRequest) error { provisions++; return nil },
						stop: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
							runtimeState = agentruntime.StateStopped
							return runtimeState, nil
						},
						start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
							runtimeState = agentruntime.StateRunning
							return runtimeState, nil
						},
						info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
							return agentruntime.Info{HandleID: h.HandleID, State: runtimeState}, nil
						},
						new: func(context.Context, agentruntime.Spec) (agentruntime.Handle, error) {
							t.Fatal("配置更新创建了新实例")
							return agentruntime.Handle{}, nil
						},
						del: func(context.Context, agentruntime.Handle) error { t.Fatal("配置更新删除了实例"); return nil },
					}))
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = svc.Close() })
					runtimeName := RuntimeNameCodex
					if kind == RuntimeKindDSH {
						runtimeName = RuntimeNameDSH
					}
					item := Agent{ID: "agent-alice", Name: "alice", Role: RoleWorker, RuntimeKind: kind, RuntimeName: runtimeName, RuntimeID: "rt-agent-alice", BoxID: "test-session", Status: string(state), DesiredState: string(state), ProfileComplete: true,
						AgentProfile: AgentProfile{Name: "alice", Provider: ProviderAPI, BaseURL: "https://api.example/v1", APIKey: "test", ModelID: "test", ProfileComplete: true},
					}
					svc.agents[item.ID] = item
					if err := svc.installDefaultSystemSkills(item.ID, kind); err != nil {
						t.Fatal(err)
					}
					root, err := svc.agentSkillsRoot(item.ID, kind)
					if err != nil {
						t.Fatal(err)
					}
					if err := svc.DeleteSkill(item.ID, "skill-installer"); err != nil {
						t.Fatal(err)
					}
					creatorPath := filepath.Join(root, "skill-creator", "SKILL.md")
					edited := "---\nname: skill-creator\ndescription: edited\n---\nuser edits\n"
					if err := os.WriteFile(creatorPath, []byte(edited), 0o644); err != nil {
						t.Fatal(err)
					}
					spec := contract.AgentSpec{Model: modelFromService(item.AgentProfile)}
					spec.Model.Env = map[string]string{"SKILL_TEST_CONFIG": "updated"}
					mask := []string{"model"}
					want := []string{"csgclaw-self-diagnosis", "skill-creator"}
					if combined {
						spec.Skills = []string{"skill-installer"}
						mask = append(mask, "skills")
						want = spec.Skills
					}
					updated, err := svc.Update(context.Background(), item.ID, contract.AgentUpdateRequest{Spec: spec, FieldMask: mask})
					if err != nil {
						t.Fatal(err)
					}
					if !slices.Equal(updated.Spec.Skills, want) {
						t.Fatalf("更新后的技能列表：%v，预期 %v", updated.Spec.Skills, want)
					}
					if state == agentruntime.StateRunning && provisions == 0 {
						t.Fatal("配置更新未执行 Provision")
					}
					beforeRestart := provisions
					_, restarted, err := svc.RestartRuntimeIfRunning(context.Background(), item.ID)
					if err != nil {
						t.Fatal(err)
					}
					if restarted != (state == agentruntime.StateRunning) {
						t.Fatalf("重启状态：%v", restarted)
					}
					expectedProvisions := beforeRestart
					if restarted {
						expectedProvisions++
					}
					if provisions != expectedProvisions {
						t.Fatalf("Provision 调用次数：%d，预期 %d", provisions, expectedProvisions)
					}
					names, err := svc.Skills(item.ID)
					if err != nil {
						t.Fatal(err)
					}
					if !slices.Equal(names, want) {
						t.Fatalf("重启后的技能列表：%v，预期 %v", names, want)
					}
					if !combined {
						data, err := os.ReadFile(creatorPath)
						if err != nil {
							t.Fatal(err)
						}
						if string(data) != edited {
							t.Fatal("配置更新或重启覆盖了用户编辑的技能")
						}
					}
				})
			}
		}
	}
}
