package agents

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
)

func TestInstructionsUpdateReloadsRunningRuntime(t *testing.T) {
	for _, kind := range []string{RuntimeKindCodex, RuntimeKindDSH} {
		for _, role := range []string{RoleWorker, RoleManager} {
			if kind == RuntimeKindDSH && role == RoleManager {
				continue
			}
			for _, scenario := range []string{"changed", "combined_mcp", "unchanged", "replace", "stopped", "metadata", "reconcile_failure", "stop_failure", "start_failure"} {
				t.Run(kind+"/"+role+"/"+scenario, func(t *testing.T) {
					state := agentruntime.StateRunning
					if scenario == "stopped" {
						state = agentruntime.StateStopped
					}
					initialState := state
					var calls []string
					failure := errors.New("指令更新失败")
					failureEnabled := true
					instructions := "旧指令"
					next := "最新指令"
					if scenario == "unchanged" || scenario == "replace" {
						next = instructions
					}
					var svc *Controller
					id := "agent-instructions"
					name := "instructions"
					if role == RoleManager {
						id, name = ManagerUserID, ManagerName
					}
					rt := fakeAgentRuntime{
						kind:       kind,
						mcpRestart: func(agentruntime.MCPServersChange) (bool, error) { return false, nil },
						reconcile: func(context.Context, agentruntime.Handle, agentruntime.RuntimeConfigChange) error {
							calls = append(calls, "reconcile")
							if scenario == "reconcile_failure" && failureEnabled {
								return failure
							}
							current, _ := svc.Agent(id)
							instructions = current.Instructions
							return nil
						},
						stop: func(_ context.Context, h agentruntime.Handle) (agentruntime.State, error) {
							calls = append(calls, "stop")
							if instructions != next {
								t.Fatalf("重启前的指令为 %q，预期 %q", instructions, next)
							}
							if h.RuntimeID != normalizeRuntimeID("rt-instructions", id) {
								t.Fatalf("运行环境发生变化：%s", h.RuntimeID)
							}
							if scenario == "stop_failure" && failureEnabled {
								return state, failure
							}
							state = agentruntime.StateStopped
							return state, nil
						},
						start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
							calls = append(calls, "start")
							if scenario == "start_failure" && failureEnabled {
								return state, failure
							}
							state = agentruntime.StateRunning
							return state, nil
						},
						info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
							return agentruntime.Info{HandleID: h.HandleID, State: state}, nil
						},
						provision: func(context.Context, agentruntime.ProvisionRequest) error {
							if scenario != "combined_mcp" {
								t.Fatal("指令保存重新执行了初始化操作")
							}
							calls = append(calls, "provision")
							return nil
						},
						del: func(context.Context, agentruntime.Handle) error {
							t.Fatal("指令保存删除了运行环境")
							return nil
						},
						new: func(context.Context, agentruntime.Spec) (agentruntime.Handle, error) {
							t.Fatal("指令保存创建了运行环境")
							return agentruntime.Handle{}, nil
						},
					}
					var err error
					svc, err = NewController(testModelConfig(), config.ServerConfig{}, "", filepath.Join(t.TempDir(), "agents.json"), WithRuntime(rt))
					if err != nil {
						t.Fatal(err)
					}
					item := Agent{ID: id, Name: name, Role: role, RuntimeID: "rt-instructions", BoxID: "existing-runtime", RuntimeKind: kind, RuntimeName: kind, Status: string(state), DesiredState: string(state), Instructions: instructions,
						ProfileComplete: true, AgentProfile: AgentProfile{Provider: ProviderAPI, BaseURL: "https://example.com/v1", APIKey: "test", ModelID: "test", ProfileComplete: true},
					}
					svc.agents[id] = item
					req := contract.AgentUpdateRequest{Spec: contract.AgentSpec{Instructions: next}, FieldMask: []string{"instructions"}}
					if scenario == "replace" {
						spec, err := svc.specFromService(item, nil, true)
						if err != nil {
							t.Fatal(err)
						}
						req = contract.AgentUpdateRequest{Spec: spec}
					}
					if scenario == "combined_mcp" {
						req.Spec.MCPServers = map[string]contract.MCPServerConfig{"search": {"url": "https://example.com/mcp"}}
						req.FieldMask = append(req.FieldMask, "mcp_servers")
					}
					if scenario == "metadata" {
						req = contract.AgentUpdateRequest{Spec: contract.AgentSpec{Description: "最新描述"}, FieldMask: []string{"description"}}
					}
					result, err := svc.Update(context.Background(), id, req)
					wantFailure := scenario == "reconcile_failure" || scenario == "stop_failure" || scenario == "start_failure"
					if wantFailure {
						if !errors.Is(err, failure) {
							t.Fatalf("更新错误为 %v，预期 %v", err, failure)
						}
					} else if err != nil {
						t.Fatal(err)
					} else if result.Status.State != contract.AgentState(initialState) {
						t.Fatalf("运行状态为 %s，预期 %s", result.Status.State, initialState)
					}
					var want []string
					switch scenario {
					case "changed", "unchanged", "replace", "start_failure":
						want = []string{"reconcile", "stop", "start"}
					case "combined_mcp":
						want = []string{"reconcile", "stop", "start"}
						if isHostRuntimeKind(kind) {
							want = []string{"reconcile", "stop", "provision", "start"}
						}
					case "stopped", "reconcile_failure":
						want = []string{"reconcile"}
					case "stop_failure":
						want = []string{"reconcile", "stop"}
					}
					if !reflect.DeepEqual(calls, want) {
						t.Fatalf("调用顺序为 %v，预期 %v", calls, want)
					}
					current, _ := svc.Agent(id)
					if scenario != "metadata" && current.Instructions != next {
						t.Fatalf("保存的指令为 %q，预期 %q", current.Instructions, next)
					}
					if scenario == "start_failure" && current.Status != string(agentruntime.StateStopped) {
						t.Fatalf("启动失败后的状态为 %s", current.Status)
					}
					failureEnabled = false
					calls = nil
					result, err = svc.Update(context.Background(), id, req)
					if err != nil {
						t.Fatalf("再次保存失败：%v", err)
					}
					if result.Status.State != contract.AgentState(initialState) {
						t.Fatalf("再次保存后的状态为 %s，预期 %s", result.Status.State, initialState)
					}
					want = []string{"reconcile", "stop", "start"}
					switch scenario {
					case "metadata":
						want = nil
					case "stopped":
						want = []string{"reconcile"}
					case "start_failure":
						want = []string{"reconcile", "start"}
					case "combined_mcp":
						if isHostRuntimeKind(kind) {
							want = []string{"reconcile", "stop", "provision", "start"}
						}
					}
					if !reflect.DeepEqual(calls, want) {
						t.Fatalf("再次保存的调用顺序为 %v，预期 %v", calls, want)
					}
				})
			}
		}
	}
}
