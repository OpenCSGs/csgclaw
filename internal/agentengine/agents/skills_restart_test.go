package agents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
)

func TestSkillListUpdateRestartsRunningCodex(t *testing.T) {
	for _, tc := range []struct {
		name    string
		kind    string
		state   agentruntime.State
		desired string
		skills  []string
		fail    bool
		restart bool
	}{
		{name: "add", kind: RuntimeKindCodex, state: agentruntime.StateRunning, desired: DesiredStateRunning, skills: []string{"existing", "skill-creator"}, restart: true},
		{name: "delete", kind: RuntimeKindCodex, state: agentruntime.StateRunning, desired: DesiredStateRunning, skills: []string{}, restart: true},
		{name: "unchanged", kind: RuntimeKindCodex, state: agentruntime.StateRunning, desired: DesiredStateRunning, skills: []string{"existing"}},
		{name: "stopped", kind: RuntimeKindCodex, state: agentruntime.StateStopped, desired: DesiredStateStopped, skills: []string{}},
		{name: "restart_failure", kind: RuntimeKindCodex, state: agentruntime.StateRunning, desired: DesiredStateRunning, skills: []string{}, restart: true, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			var skillsRoot string
			startErr := errors.New("启动失败")
			state := tc.state
			svc, err := NewController(testModelConfig(), config.ServerConfig{}, "", filepath.Join(t.TempDir(), "agents.json"), WithRuntime(fakeAgentRuntime{
				kind: tc.kind,
				stop: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
					calls = append(calls, "stop")
					state = agentruntime.StateStopped
					return state, nil
				},
				provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
					calls = append(calls, "provision")
					if req.InitShell != "echo initialized" {
						t.Fatalf("初始化脚本未传入：%q", req.InitShell)
					}
					return nil
				},
				start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
					calls = append(calls, "start")
					entries, err := os.ReadDir(skillsRoot)
					if err != nil {
						t.Fatal(err)
					}
					names := make([]string, 0, len(entries))
					for _, entry := range entries {
						if entry.IsDir() {
							names = append(names, entry.Name())
						}
					}
					if !reflect.DeepEqual(names, tc.skills) {
						t.Fatalf("启动前的技能列表：%v，预期 %v", names, tc.skills)
					}
					if tc.fail {
						return state, startErr
					}
					state = agentruntime.StateRunning
					return state, nil
				},
				info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
					return agentruntime.Info{HandleID: h.HandleID, State: state}, nil
				},
				del: func(context.Context, agentruntime.Handle) error { t.Fatal("技能修改触发重建"); return nil },
				new: func(context.Context, agentruntime.Spec) (agentruntime.Handle, error) {
					t.Fatal("技能修改创建新实例")
					return agentruntime.Handle{}, nil
				},
			}))
			if err != nil {
				t.Fatal(err)
			}
			profile := AgentProfile{Name: "alice", Provider: ProviderAPI, BaseURL: "https://api.example/v1", ModelID: "test", APIKey: "test", ProfileComplete: true}
			item := Agent{ID: "agent-alice", Name: "alice", Role: RoleWorker, RuntimeID: "rt-agent-alice", RuntimeKind: tc.kind, RuntimeName: RuntimeNameCodex, Status: string(tc.state), DesiredState: tc.desired, ProfileComplete: true, AgentProfile: profile}
			if tc.kind == RuntimeKindDSH {
				item.RuntimeName = RuntimeNameDSH
				item.SandboxEnabled = true
			}
			if tc.kind == RuntimeKindCodex {
				item.SetRuntimeProvision(nil, "echo initialized")
			}
			svc.agents[item.ID] = item
			root, err := svc.agentSkillsRoot(item.ID, item.RuntimeKind)
			skillsRoot = root
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "existing"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "existing", "SKILL.md"), []byte("---\nname: existing\ndescription: test\n---\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			result, err := svc.Update(context.Background(), item.ID, contract.AgentUpdateRequest{Spec: contract.AgentSpec{Skills: tc.skills}, FieldMask: []string{"skills"}})
			if tc.fail {
				if !errors.Is(err, startErr) {
					t.Fatalf("启动错误未返回：%v", err)
				}
				current, _ := svc.Agent(item.ID)
				if current.Status != string(agentruntime.StateStopped) {
					t.Fatalf("失败后的状态：%s", current.Status)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if result.Status.State != contract.AgentState(tc.state) {
					t.Fatalf("更新后的状态：%s", result.Status.State)
				}
			}
			var want []string
			if tc.restart {
				want = []string{"stop", "provision", "start"}
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("生命周期调用：%v，预期 %v", calls, want)
			}
		})
	}
}
