package agents

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
)

func TestStopInterruptsExecutionBeforeDraining(t *testing.T) {
	for _, operation := range []string{"direct", "desired-state", "stale-version", "invalid-spec"} {
		t.Run(operation, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", filepath.Join(t.TempDir(), "agents.json"), WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}))
			if err != nil {
				t.Fatal(err)
			}
			svc.agents["agent-worker"] = Agent{ID: "agent-worker", Name: "worker", Role: RoleWorker, RuntimeKind: RuntimeKindCodex, RuntimeID: "rt-worker", Status: string(agentruntime.StateRunning), DesiredState: DesiredStateRunning, ProfileComplete: true, AgentProfile: AgentProfile{Name: "worker", Provider: ProviderAPI, BaseURL: "https://example.com/v1", APIKey: "fixture", ModelID: "model", ProfileComplete: true}}
			release, err := svc.LifecycleCoordinator().Execution(context.Background(), "agent-worker")
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			interrupted := false
			svc.interruptAgent = func(id string) {
				interrupted = true
				if admission, err := svc.LifecycleCoordinator().Execution(context.Background(), id); err == nil {
					admission()
					t.Error("new work admitted during stop")
				}
				release()
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if operation == "direct" {
				_, err = svc.Stop(ctx, "agent-worker")
			} else {
				request := contract.AgentUpdateRequest{Spec: contract.AgentSpec{DesiredState: contract.AgentDesiredStateStopped}, FieldMask: []string{"desired_state"}}
				if operation == "stale-version" {
					request.ResourceVersion = "stale"
				}
				if operation == "invalid-spec" {
					request.Spec.Name = ""
					request.FieldMask = append(request.FieldMask, "name")
				}
				_, err = svc.Update(ctx, "agent-worker", request)
			}
			if operation == "stale-version" || operation == "invalid-spec" {
				if err == nil || errors.Is(err, context.DeadlineExceeded) || interrupted {
					t.Fatalf("invalid stop interrupted execution or waited for it: interrupted=%v err=%v", interrupted, err)
				}
				return
			}
			if err != nil || !interrupted {
				t.Fatalf("stop failed to interrupt before drain: interrupted=%v err=%v", interrupted, err)
			}
			item, _ := svc.Agent("agent-worker")
			if item.Status != string(agentruntime.StateStopped) {
				t.Fatalf("stopped state not persisted: %+v", item)
			}
		})
	}
}
