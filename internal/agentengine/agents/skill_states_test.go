package agents

import (
	"context"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	skill "csgclaw/internal/skill/state"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type skillStateRuntime struct {
	fakeAgentRuntime
	reconciled map[string]skill.State
}

func (r *skillStateRuntime) ReconcileSkills(_ context.Context, _ agentruntime.Handle, states map[string]skill.State) error {
	r.reconciled = states
	return nil
}

func TestResourceEnablementLifecycle(t *testing.T) {
	for _, kind := range []string{RuntimeKindCodex, RuntimeKindDSH} {
		for _, running := range []bool{false, true} {
			t.Run(kind+map[bool]string{true: "/running", false: "/stopped"}[running], func(t *testing.T) {
				state := agentruntime.StateStopped
				desired := DesiredStateStopped
				if running {
					state = agentruntime.StateRunning
					desired = DesiredStateRunning
				}
				stops, starts := 0, 0
				fail := false
				rt := &skillStateRuntime{fakeAgentRuntime: fakeAgentRuntime{kind: kind,
					mcpRestart: func(agentruntime.MCPServersChange) (bool, error) { return true, nil },
					stop: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
						stops++
						state = agentruntime.StateStopped
						return state, nil
					},
					start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
						starts++
						if fail {
							return state, errors.New("启动失败")
						}
						state = agentruntime.StateRunning
						return state, nil
					},
					info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
						return agentruntime.Info{HandleID: h.HandleID, State: state}, nil
					},
				}}
				svc, err := NewController(testModelConfig(), config.ServerConfig{}, "", filepath.Join(t.TempDir(), "agents.json"), WithRuntime(rt))
				if err != nil {
					t.Fatal(err)
				}
				name := RuntimeNameCodex
				if kind == RuntimeKindDSH {
					name = RuntimeNameDSH
				}
				item := Agent{ID: "agent-skill", Name: "skill", Role: RoleWorker, RuntimeID: "rt-skill", RuntimeKind: kind, RuntimeName: name, Status: string(state), DesiredState: desired, ProfileComplete: true, AgentProfile: AgentProfile{Provider: ProviderAPI, BaseURL: "https://example.com", ModelID: "test", APIKey: "test", ProfileComplete: true}}
				svc.agents[item.ID] = item
				root, err := svc.agentSkillsRoot(item.ID, kind)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(root, "reviewer"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "reviewer", "SKILL.md"), []byte("---\nname: reviewer\ndescription: review\n---\n"), 0600); err != nil {
					t.Fatal(err)
				}
				req := contract.AgentUpdateRequest{Spec: contract.AgentSpec{SkillStates: map[string]skill.State{"reviewer": {Enabled: false}}, MCPServers: map[string]contract.MCPServerConfig{"search": {"url": "https://example.com/mcp", "enabled": false}}}, FieldMask: []string{"skill_states", "mcp_servers"}}
				before, err := svc.Get(context.Background(), item.ID, contract.AgentGetOptions{})
				if err != nil {
					t.Fatal(err)
				}
				req.ResourceVersion = before.ResourceVersion
				result, err := svc.Update(context.Background(), item.ID, req)
				if err != nil {
					t.Fatal(err)
				}
				if skill.Enabled(result.Spec.SkillStates, "reviewer") {
					t.Fatal("禁用状态未保存")
				}
				if running && (stops != 1 || starts != 1) || !running && (stops != 0 || starts != 0) {
					t.Fatalf("生命周期调用次数：%d/%d", stops, starts)
				}
				if _, err := svc.Update(context.Background(), item.ID, req); !errors.Is(err, ErrAgentResourceVersionConflict) {
					t.Fatalf("过期版本未拒绝：%v", err)
				}
				if err := svc.Reload(); err != nil {
					t.Fatal(err)
				}
				stored, _ := svc.Agent(item.ID)
				if skill.Enabled(stored.SkillStates, "reviewer") {
					t.Fatal("读取持久状态后禁用状态丢失")
				}
				req.ResourceVersion = ""
				if running {
					fail = true
					req.Spec.SkillStates["reviewer"] = skill.State{Enabled: true}
					if _, err := svc.Update(context.Background(), item.ID, req); err == nil {
						t.Fatal("启动失败未返回错误")
					}
					stored, _ = svc.Agent(item.ID)
					if !stored.AgentProfile.EnvRestartRequired {
						t.Fatal("缺少重试标记")
					}
					fail = false
					if _, err := svc.Update(context.Background(), item.ID, req); err != nil {
						t.Fatal(err)
					}
					stored, _ = svc.Agent(item.ID)
					if stored.AgentProfile.EnvRestartRequired || starts != 3 {
						t.Fatalf("重试状态错误：%v/%d", stored.AgentProfile.EnvRestartRequired, starts)
					}
				} else if skill.Enabled(rt.reconciled, "reviewer") {
					t.Fatal("停止状态未写入配置")
				}
				if _, err := os.Stat(filepath.Join(root, "reviewer", "SKILL.md")); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestMetadataUpdateDoesNotApplyPendingResources(t *testing.T) {
	for _, kind := range []string{RuntimeKindCodex, RuntimeKindDSH} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			rt := &skillStateRuntime{fakeAgentRuntime: fakeAgentRuntime{kind: kind,
				start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
					calls++
					return agentruntime.StateRunning, nil
				},
				mcpReconcile: func(context.Context, agentruntime.Handle, agentruntime.MCPServersChange) error { calls++; return nil },
			}}
			svc, err := NewController(testModelConfig(), config.ServerConfig{}, "", filepath.Join(t.TempDir(), "agents.json"), WithRuntime(rt))
			if err != nil {
				t.Fatal(err)
			}
			item := Agent{ID: "agent-metadata", Name: "metadata", Role: RoleWorker, RuntimeKind: kind, RuntimeName: kind, RuntimeID: "rt-metadata", Status: string(agentruntime.StateRunning), DesiredState: DesiredStateRunning,
				ProfileComplete: true, AgentProfile: AgentProfile{Provider: ProviderAPI, BaseURL: "https://example.com", APIKey: "test", ModelID: "test", ProfileComplete: true, EnvRestartRequired: true},
				MCPServers: map[string]any{"search": map[string]any{"url": "https://example.com/mcp"}},
			}
			svc.agents[item.ID] = item
			updated, err := svc.Update(context.Background(), item.ID, contract.AgentUpdateRequest{Spec: contract.AgentSpec{Description: "修改说明"}, FieldMask: []string{"description"}})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 0 {
				t.Fatalf("修改说明触发了资源应用：%d", calls)
			}
			if !updated.Status.Model.EnvRestartRequired {
				t.Fatal("待重启标记被清除")
			}
		})
	}
}

func TestResourceUpdateAlsoReconcilesRuntimeConfig(t *testing.T) {
	for _, kind := range []string{RuntimeKindCodex, RuntimeKindDSH} {
		t.Run(kind, func(t *testing.T) {
			configCalls, starts := 0, 0
			rt := &skillStateRuntime{fakeAgentRuntime: fakeAgentRuntime{kind: kind,
				reconcile: func(context.Context, agentruntime.Handle, agentruntime.RuntimeConfigChange) error {
					configCalls++
					return nil
				},
				mcpRestart: func(agentruntime.MCPServersChange) (bool, error) { return true, nil },
				start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
					starts++
					return agentruntime.StateRunning, nil
				},
			}}
			svc, err := NewController(testModelConfig(), config.ServerConfig{}, "", filepath.Join(t.TempDir(), "agents.json"), WithRuntime(rt))
			if err != nil {
				t.Fatal(err)
			}
			item := Agent{ID: "agent-combined", Name: "combined", Role: RoleWorker, RuntimeKind: kind, RuntimeName: kind, RuntimeID: "rt-combined", Status: string(agentruntime.StateRunning), DesiredState: DesiredStateRunning,
				ProfileComplete: true, AgentProfile: AgentProfile{Provider: ProviderAPI, BaseURL: "https://example.com", APIKey: "test", ModelID: "test", ProfileComplete: true},
			}
			svc.agents[item.ID] = item
			_, err = svc.Update(context.Background(), item.ID, contract.AgentUpdateRequest{Spec: contract.AgentSpec{Instructions: "新的指令", MCPServers: map[string]contract.MCPServerConfig{"search": {"url": "https://example.com/mcp"}}}, FieldMask: []string{"instructions", "mcp_servers"}})
			if err != nil {
				t.Fatal(err)
			}
			if configCalls != 1 || starts != 1 {
				t.Fatalf("配置应用及启动次数错误：%d/%d", configCalls, starts)
			}
		})
	}
}
