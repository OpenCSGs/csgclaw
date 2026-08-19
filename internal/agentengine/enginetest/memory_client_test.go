package enginetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
)

func runningAgent(id string) agentengine.Agent {
	return agentengine.Agent{
		ID: id,
		Spec: agentengine.AgentSpec{
			Name:    id,
			Role:    agentengine.AgentRoleWorker,
			Runtime: agentengine.RuntimeSpec{Adapter: "codex"},
		},
		Status: agentengine.AgentStatus{State: agentengine.AgentStateRunning, RuntimeID: "runtime-" + id, Ready: true},
	}
}

func TestMemoryClientAgentLifecycleAndSecretRedaction(t *testing.T) {
	client := NewMemoryClient()
	created, err := client.Agents().Create(context.Background(), agentengine.AgentSpec{
		Name:       "worker",
		Role:       agentengine.AgentRoleWorker,
		Runtime:    agentengine.RuntimeSpec{Adapter: "codex", Options: map[string]any{"mode": "fast"}},
		Skills:     []string{"review"},
		MCPServers: map[string]agentengine.MCPServerConfig{"tools": {"command": "tool"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	started, err := client.Agents().Start(context.Background(), created.ID)
	if err != nil || started.Status.State != agentengine.AgentStateRunning {
		t.Fatalf("Start() = %+v, %v", started, err)
	}
	started.Spec.Skills[0] = "mutated"
	got, err := client.Agents().Get(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.Skills[0] != "review" || got.Spec.Runtime.Credentials != nil {
		t.Fatalf("Get() leaked mutation or credentials: %+v", got)
	}
	provisioned, err := client.Agents().Update(context.Background(), created.ID, agentengine.AgentSpec{
		Name:    "worker",
		Runtime: agentengine.RuntimeSpec{Adapter: "codex", Credentials: map[string]string{"secrets/token": "secret"}, InitShell: "test -f secrets/token"},
	})
	if err != nil || provisioned.Spec.Runtime.Credentials != nil || provisioned.Spec.Runtime.InitShell != "test -f secrets/token" {
		t.Fatalf("provisioned Update = %+v, %v", provisioned, err)
	}
	if _, err := client.Agents().Update(context.Background(), created.ID, agentengine.AgentSpec{
		Name:    "worker",
		Runtime: agentengine.RuntimeSpec{Adapter: "openclaw", Credentials: map[string]string{"token": "secret"}},
	}); !hasCode(err, agentengine.ErrorUnsupportedRuntimeProvision) {
		t.Fatalf("unsupported credential Update error = %v", err)
	}
}

func TestMemoryClientConversationContract(t *testing.T) {
	client := NewMemoryClient(runningAgent("agent-a"))
	started := make(chan agentengine.ConversationKey, 2)
	release := make(chan struct{})
	client.SetTurnBehavior(func(ctx context.Context, _ string, request agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
		started <- request.ConversationKey
		if err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "one"}); err != nil {
			return agentengine.TurnResult{Status: agentengine.TurnFailed, Dispatched: true, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: err.Error()}}
		}
		<-release
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "one", Dispatched: true}
	})
	firstDone := make(chan agentengine.TurnResult, 1)
	go func() {
		firstDone <- client.Conversations("agent-a").Run(context.Background(), turnRequest("turn-1", "conversation-1"), nil)
	}()
	<-started
	overlap := client.Conversations("agent-a").Run(context.Background(), turnRequest("turn-2", "conversation-1"), nil)
	if overlap.Error == nil || overlap.Error.Code != agentengine.ErrorConversationBusy || overlap.Dispatched {
		t.Fatalf("overlap = %+v", overlap)
	}
	secondDone := make(chan agentengine.TurnResult, 1)
	go func() {
		secondDone <- client.Conversations("agent-a").Run(context.Background(), turnRequest("turn-3", "conversation-2"), nil)
	}()
	<-started
	close(release)
	if (<-firstDone).Status != agentengine.TurnSucceeded || (<-secondDone).Status != agentengine.TurnSucceeded {
		t.Fatal("different conversations did not complete")
	}
	calls := client.Calls()
	if len(calls) != 2 || len(calls[0].Events) != 1 || calls[0].Events[0].Sequence != 1 {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestMemoryClientCancelResetAndResolve(t *testing.T) {
	client := NewMemoryClient(runningAgent("agent-a"))
	started := make(chan struct{})
	cleanup := make(chan struct{})
	releaseCleanup := make(chan struct{})
	client.SetTurnBehavior(func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
		if err := sink.Emit(ctx, agentengine.TurnEvent{
			Kind:        agentengine.TurnEventInteractionRequest,
			Interaction: &agentengine.InteractionRequest{ID: "question-1", Kind: agentengine.InteractionUserInput},
		}); err != nil {
			t.Fatal(err)
		}
		close(started)
		<-ctx.Done()
		close(cleanup)
		<-releaseCleanup
		return agentengine.TurnResult{Status: agentengine.TurnCanceled, Dispatched: true, Error: &agentengine.TurnError{Code: agentengine.ErrorCanceled, Message: ctx.Err().Error()}}
	})
	done := make(chan agentengine.TurnResult, 1)
	go func() {
		done <- client.Conversations("agent-a").Run(context.Background(), turnRequest("turn-1", "conversation-1"), nil)
	}()
	<-started
	if err := client.Conversations("agent-a").Resolve(context.Background(), agentengine.InteractionResolution{
		ConversationKey: "conversation-1", InteractionID: "question-1", ResponderID: "tester",
	}); err != nil {
		t.Fatal(err)
	}
	resolutions := client.Resolutions()
	if len(resolutions) != 1 || resolutions[0].Value.ResponderID != "tester" {
		t.Fatalf("resolutions = %+v", resolutions)
	}
	resetDone := make(chan error, 1)
	go func() {
		resetDone <- client.Conversations("agent-a").Reset(context.Background(), "conversation-1")
	}()
	<-cleanup
	select {
	case err := <-resetDone:
		t.Fatalf("Reset returned before cleanup: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseCleanup)
	if (<-done).Status != agentengine.TurnCanceled {
		t.Fatal("turn was not canceled")
	}
	if err := <-resetDone; err != nil {
		t.Fatal(err)
	}
	strict := turnRequest("turn-2", "conversation-1")
	strict.Continuation = agentengine.ContinuationRequireExisting
	if result := client.Conversations("agent-a").Run(context.Background(), strict, nil); result.Error == nil || result.Error.Code != agentengine.ErrorConversationNotResumable {
		t.Fatalf("strict result = %+v", result)
	}
}

func TestMemoryClientSinkFailureRecordsFailure(t *testing.T) {
	client := NewMemoryClient(runningAgent("agent-a"))
	client.SetTurnBehavior(func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
		err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "partial"})
		return agentengine.TurnResult{Status: agentengine.TurnFailed, Dispatched: true, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: err.Error()}}
	})
	result := client.Conversations("agent-a").Run(context.Background(), turnRequest("turn-1", "conversation-1"), agentengine.EventSinkFunc(func(context.Context, agentengine.TurnEvent) error {
		return errors.New("sink closed")
	}))
	if result.Error == nil || result.Error.Code != agentengine.ErrorRuntimeFailed {
		t.Fatalf("result = %+v", result)
	}
}

func turnRequest(turnID agentengine.TurnID, key agentengine.ConversationKey) agentengine.TurnRequest {
	return agentengine.TurnRequest{
		ID: turnID, ConversationKey: key,
		Input: []agentengine.InputPart{{Kind: agentengine.InputPartText, Text: "hello"}},
	}
}

func hasCode(err error, code agentengine.ErrorCode) bool {
	var turnErr *agentengine.TurnError
	return errors.As(err, &turnErr) && turnErr.Code == code
}
