package agentengine

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/activity"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/runtime"
)

type pendingRecoveryRuntime struct {
	*fakeConversationRuntime
	starts atomic.Int32
}

func (r *pendingRecoveryRuntime) Start(context.Context, runtime.Handle) (runtime.State, error) {
	r.starts.Add(1)
	r.mu.Lock()
	r.state = runtime.StateRunning
	r.mu.Unlock()
	return runtime.StateRunning, nil
}

func (r *pendingRecoveryRuntime) State(context.Context, runtime.Handle) (runtime.State, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state, nil
}

func (r *pendingRecoveryRuntime) Info(ctx context.Context, h runtime.Handle) (runtime.Info, error) {
	s, err := r.State(ctx, h)
	return runtime.Info{State: s}, err
}

func TestConversationRecoversPendingOfflineRuntime(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		state                               runtime.State
		desired                             string
		pending, complete, blocked, recover bool
		manager                             bool
	}{
		{"failed", runtime.StateFailed, agent.DesiredStateRunning, true, true, false, true, false},
		{"exited", runtime.StateExited, agent.DesiredStateRunning, true, true, false, true, false},
		{"unknown", runtime.StateUnknown, agent.DesiredStateRunning, true, true, false, true, false},
		{"model-now-configured", runtime.State("profile_incomplete"), agent.DesiredStateRunning, true, true, false, true, false},
		{"model-still-missing", runtime.State("profile_incomplete"), agent.DesiredStateRunning, true, false, false, false, false},
		{"user-stopped", runtime.StateStopped, agent.DesiredStateStopped, true, true, false, false, false},
		{"no-pending-config", runtime.StateFailed, agent.DesiredStateRunning, false, true, false, false, false},
		{"no-model", runtime.StateFailed, agent.DesiredStateRunning, true, false, false, false, false},
		{"connector-pending", runtime.StateFailed, agent.DesiredStateRunning, true, true, true, false, false},
		{"manager-initial-failure", runtime.StateFailed, "", true, true, false, true, true},
		{"manager-user-stopped", runtime.StateStopped, agent.DesiredStateStopped, true, true, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := &pendingRecoveryRuntime{fakeConversationRuntime: &fakeConversationRuntime{state: tc.state, workspace: t.TempDir()}}
			rt.prompt = func(_ context.Context, runtimeID, sessionID, _ string) error {
				rt.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptCompleted})
				return nil
			}
			profile := agent.AgentProfile{EnvRestartRequired: tc.pending}
			if tc.complete {
				profile = agent.AgentProfile{Provider: agent.ProviderAPI, BaseURL: "https://model.example/v1", APIKey: "test-only", ModelID: "test-model", ProfileComplete: true, EnvRestartRequired: tc.pending}
			}
			id, role := "agent-recovery", agent.RoleWorker
			if tc.manager {
				id, role = agent.ManagerUserID, agent.RoleManager
			}
			controller := newTestAgentService(t, []agent.Agent{{ID: id, Name: "Recovery", Role: role, RuntimeKind: runtime.KindCodex, RuntimeID: "rt-agent-recovery", Status: string(tc.state), DesiredState: tc.desired, AgentProfile: profile, ProfileComplete: tc.complete}}, rt)
			if tc.desired != "" {
				if _, err := controller.SetDesiredState(id, tc.desired, false); err != nil {
					t.Fatal(err)
				}
			}
			if tc.blocked {
				controller.SetRuntimeConfigurationGuard(func(context.Context, string) error { return errors.New("Connector configuration is pending") })
			}
			engine := New(controller)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			result := engine.Conversations(id).Run(ctx, TurnRequest{ID: "recovery-turn", ConversationKey: "recovery-chat", Input: textInput("hello")}, nil)
			if tc.recover {
				if result.Status != TurnSucceeded || rt.starts.Load() != 1 {
					t.Fatalf("conversation did not recover: result=%+v starts=%d", result, rt.starts.Load())
				}
				selected, _ := controller.Agent(id)
				if selected.AgentProfile.EnvRestartRequired {
					t.Fatal("pending marker survived successful recovery")
				}
				result = engine.Conversations(id).Run(ctx, TurnRequest{ID: "follow-up", ConversationKey: "recovery-chat", Input: textInput("again")}, nil)
				if result.Status != TurnSucceeded || rt.starts.Load() != 1 {
					t.Fatalf("follow-up did not reuse recovered runtime: %+v starts=%d", result, rt.starts.Load())
				}
			} else if result.Status != TurnFailed || rt.starts.Load() != 0 {
				t.Fatalf("unexpected recovery: result=%+v starts=%d", result, rt.starts.Load())
			}
			if tc.blocked && (result.Error == nil || result.Error.Message != "Connector configuration is pending") {
				t.Fatalf("configuration guard lost: %+v", result)
			}
		})
	}
}
