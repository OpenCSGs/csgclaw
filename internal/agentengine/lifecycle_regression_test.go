package agentengine

import (
	"context"
	"errors"
	goruntime "runtime"
	"strings"
	"testing"
	"time"
	"weak"

	"csgclaw/internal/activity"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/runtime"
	"csgclaw/internal/runtime/codex"
)

func TestConversationResolvePreservesMixedAnswersAndSkips(t *testing.T) {
	rt := &fakeConversationRuntime{}
	broker := codex.NewUserInputBroker(nativeInteractionSink(rt.publish))
	rt.userInput = broker
	rt.prompt = func(ctx context.Context, runtimeID, sessionID, _ string) error {
		decision, err := broker.Request(ctx, codex.PendingUserInputRequest{
			Execution: activity.ExecutionRef{RuntimeID: runtimeID, SessionID: sessionID},
			Questions: []activity.UserInputQuestionSnapshot{
				{ID: "choice", Header: "Choice", Question: "Continue?", Options: []activity.UserInputOptionSnapshot{{Label: "Yes"}}},
				{ID: "note", Header: "Note", Question: "Optional note?", IsOther: true},
			},
		})
		if err == nil {
			choice, ok := decision.Response.Answers["choice"]
			note, hasNote := decision.Response.Answers["note"]
			if !ok || len(choice.Answers) != 1 || choice.Answers[0] != "Yes" || !hasNote || note.Answers == nil || len(note.Answers) != 0 {
				return errors.New("native response did not preserve both the selected answer and explicit skip")
			}
			rt.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptCompleted})
		}
		return err
	}
	engine := New(newTestAgentService(t, []agent.Agent{{ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex, RuntimeID: "runtime-a", Status: string(runtime.StateRunning)}}, rt))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	requests := make(chan string, 1)
	done := make(chan TurnResult, 1)
	go func() {
		done <- engine.Conversations("agent-a").Run(ctx, TurnRequest{ID: "mixed", ConversationKey: "conv", Input: textInput("Ask me"), Interaction: InteractionResolve}, EventSinkFunc(func(_ context.Context, event TurnEvent) error {
			if event.Interaction != nil {
				requests <- event.Interaction.ID
			}
			return nil
		}))
	}()
	var id string
	select {
	case id = <-requests:
	case <-ctx.Done():
		t.Fatal("missing question")
	}
	err := engine.Conversations("agent-a").Resolve(ctx, InteractionResolution{ConversationKey: "conv", InteractionID: id, ResponderID: "tester", Answers: map[string]InteractionAnswer{"choice": {Values: []string{"Yes"}}, "note": {Skipped: true}}})
	if err != nil {
		cancel()
		<-done
		t.Fatalf("valid mixed answer/skip rejected: %v", err)
	}
	if result := <-done; result.Status != TurnSucceeded {
		t.Fatalf("result=%+v", result)
	}
}

func TestRequiredExtensionDeletionDoesNotGateItsOwnReload(t *testing.T) {
	engine, _ := newProjectionEngine(t, true)
	ctx := context.Background()
	request := projectionRequest("required-tool", "one")
	request.Spec.FailurePolicy = RuntimeExtensionBlockRuntime
	extensions := engine.RuntimeExtensions("agent-a")
	if item, err := extensions.Apply(ctx, request); err != nil || !item.Status.RuntimeLoaded {
		t.Fatalf("Apply=%+v err=%v", item, err)
	}
	if err := extensions.Delete(ctx, "required-tool"); err != nil {
		t.Fatalf("healthy required extension cannot be deleted in one call: %v", err)
	}
	if _, err := extensions.Get(ctx, "required-tool"); ErrorCodeOf(err) != ErrorRuntimeExtensionNotFound {
		t.Fatalf("deleted resource remains: %v", err)
	}
}

func TestDetachedAnswerCanceledDuringTranscriptCannotComplete(t *testing.T) {
	engine := &Engine{}
	item, err := engine.interactions.CreateDetached("agent-a", "conv", "question-turn", activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{ID: "choice", Header: "Choice", Question: "Continue?", IsOther: true}}}, InteractionResolve, nil)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- engine.Conversations("agent-a").Resolve(context.Background(), InteractionResolution{ConversationKey: "conv", InteractionID: item.ID, ResponderID: "tester", Answers: map[string]InteractionAnswer{"choice": {Values: []string{"user_note: continue"}}}, BeforeResolve: func(context.Context, InteractionRequest) error { close(entered); <-release; return nil }})
	}()
	<-entered
	if err := engine.Conversations("agent-a").Cancel(context.Background(), "conv", "question-turn"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; ErrorCodeOf(err) != ErrorInteractionGone {
		t.Errorf("canceled answer completed: %v", err)
	}
	got, _ := engine.Conversations("agent-a").GetInteraction(context.Background(), "conv", item.ID)
	if got.Payload.(activity.UserInputSnapshot).Status != activity.UserInputStatusInterrupted {
		t.Errorf("status after Cancel = %s", got.Payload.(activity.UserInputSnapshot).Status)
	}
}

func TestRuntimeReplacementPreservesExtensionResource(t *testing.T) {
	rt := &projectionRuntime{fakeConversationRuntime: &fakeConversationRuntime{state: runtime.StateStopped}, root: t.TempDir()}
	controller := newTestAgentService(t, []agent.Agent{{ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: runtime.KindCodex, RuntimeID: "rt-agent-a", Status: "stopped", AgentProfile: agent.AgentProfile{Provider: agent.ProviderAPI, BaseURL: "https://example.com/v1", APIKey: "test-only", ModelID: "test-model", ProfileComplete: true}}}, rt)
	other := &fakeConversationRuntime{kind: runtime.KindDSH, state: runtime.StateRunning}
	if err := agent.WithRuntime(other)(controller); err != nil {
		t.Fatal(err)
	}
	engine := New(controller)
	if err := engine.RegisterRuntimeExtensionSource("fixture", projectionSource{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := engine.RuntimeExtensions("agent-a").Apply(ctx, projectionRequest("tool", "one")); err != nil {
		t.Fatal(err)
	}
	_, err := engine.Agents().Update(ctx, "agent-a", AgentUpdateRequest{FieldMask: []string{"runtime"}, Spec: AgentSpec{Runtime: RuntimeSpec{Adapter: "dsh"}}})
	if err != nil {
		t.Fatalf("runtime switch failed: %v", err)
	}
	item, err := engine.RuntimeExtensions("agent-a").Get(ctx, "tool")
	if err != nil {
		t.Fatalf("independent extension resource lost on Runtime switch: %v", err)
	}
	if item.Status.Reason != "extension_unsupported" || item.Status.RuntimeLoaded || item.Status.Generation != 1 {
		t.Fatalf("unsupported target lost desired generation or has stale status: %+v", item.Status)
	}
	if err := controller.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.RuntimeExtensions("agent-a").Get(ctx, "tool"); err != nil {
		t.Fatalf("extension was not persisted across replacement: %v", err)
	}
	if err := engine.RuntimeExtensions("agent-a").Delete(ctx, "tool"); err != nil {
		t.Fatalf("unsupported target prevented removing desired resource: %v", err)
	}
}

func TestFailedRuntimeReplacementRetainsRequiredExtension(t *testing.T) {
	rt := &projectionRuntime{fakeConversationRuntime: &fakeConversationRuntime{state: runtime.StateStopped}, root: t.TempDir()}
	controller := newTestAgentService(t, []agent.Agent{{ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: runtime.KindCodex, RuntimeID: "rt-agent-a", Status: "stopped", AgentProfile: agent.AgentProfile{Provider: agent.ProviderAPI, BaseURL: "https://example.com/v1", APIKey: "test-only", ModelID: "test-model", ProfileComplete: true}}}, rt)
	if err := agent.WithRuntime(&fakeConversationRuntime{kind: runtime.KindDSH})(controller); err != nil {
		t.Fatal(err)
	}
	engine := New(controller)
	if err := engine.RegisterRuntimeExtensionSource("fixture", projectionSource{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	request := projectionRequest("required", "one")
	request.Spec.FailurePolicy = RuntimeExtensionBlockRuntime
	if _, err := engine.RuntimeExtensions("agent-a").Apply(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Agents().Update(ctx, "agent-a", AgentUpdateRequest{FieldMask: []string{"runtime"}, Spec: AgentSpec{Runtime: RuntimeSpec{Adapter: "dsh"}}}); err == nil {
		t.Fatal("replacement started without required extension support")
	}
	if err := controller.Reload(); err != nil {
		t.Fatal(err)
	}
	item, err := engine.RuntimeExtensions("agent-a").Get(ctx, "required")
	if err != nil || item.Spec != request.Spec || item.Status.Reason != "extension_unsupported" {
		t.Fatalf("failed replacement lost required resource: %+v %v", item, err)
	}
	agent, err := engine.Agents().Get(ctx, "agent-a", AgentGetOptions{})
	if err != nil || agent.Status.Ready || agent.Status.State != AgentStateFailed {
		t.Fatalf("failed replacement should remain retryable and not ready: %+v %v", agent.Status, err)
	}
}

func TestExtensionDeletionDoesNotRequireUnrelatedExtensionReadiness(t *testing.T) {
	engine, rt := newProjectionEngine(t, true)
	ctx := context.Background()
	if _, err := engine.RuntimeExtensions("agent-a").Apply(ctx, projectionRequest("first", "one")); err != nil {
		t.Fatal(err)
	}
	// A second required extension cannot prepare. This should block admission,
	// not turn a successful removal of an optional extension into a failed delete.
	rt.unavailable = true
	req := projectionRequest("required", "one")
	req.Spec.FailurePolicy = RuntimeExtensionBlockRuntime
	if _, err := engine.RuntimeExtensions("agent-a").Apply(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err := engine.RuntimeExtensions("agent-a").Delete(ctx, "first"); err != nil {
		t.Fatalf("optional deletion fails due to unrelated unconfigured required resource: %v", err)
	}
}

func TestDetachedAnswerCanRetryUntilRuntimeAccepts(t *testing.T) {
	rt := &fakeConversationRuntime{state: runtime.StateStopped}
	calls := 0
	rt.prompt = func(context.Context, string, string, string) error {
		calls++
		return errors.New("runtime failed after accepting the prompt")
	}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, rt))
	item, err := engine.interactions.CreateDetached("agent-a", "conv", "question", activity.RequestUserInputArgs{
		Questions: []activity.RequestUserInputQuestion{{ID: "choice", Header: "Choice", Question: "Continue?", IsOther: true}},
	}, InteractionResolve, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conversation := engine.Conversations("agent-a")
	var turnID TurnID
	var result TurnResult
	answer := InteractionResolution{
		ConversationKey: "conv", InteractionID: item.ID,
		Answers: map[string]InteractionAnswer{"choice": {Values: []string{"user_note: continue"}}},
		ContinueDetached: func(ctx context.Context, question InteractionRequest) bool {
			result = conversation.Run(ctx, TurnRequest{
				ID: turnID, ConversationKey: "conv", RespondingTo: question.ID,
				Input: textInput("continue"), Admission: AdmissionWait,
			}, nil)
			return result.Dispatched
		},
	}

	turnID = "before-runtime"
	_ = conversation.Resolve(ctx, answer)
	if ErrorCodeOf(result.Error) != ErrorAgentUnavailable || result.Dispatched {
		t.Fatalf("runtime acquisition: %+v", result)
	}
	pending, err := conversation.PendingInteractions(ctx, "conv")
	if err != nil || len(pending) != 1 || pending[0].ID != item.ID {
		t.Fatalf("pending after acquisition failure: %+v, %v", pending, err)
	}

	rt.state = runtime.StateRunning
	turnID = "retry"
	if err := conversation.Resolve(ctx, answer); err != nil {
		t.Fatal(err)
	}
	if result.Status != TurnFailed || !result.Dispatched {
		t.Fatalf("failure after dispatch: %+v", result)
	}
	turnID = "duplicate"
	if err := conversation.Resolve(ctx, answer); ErrorCodeOf(err) != ErrorInteractionAlreadyResolved {
		t.Fatalf("duplicate answer: %v", err)
	}
	if calls != 1 {
		t.Fatalf("runtime received the answer %d times", calls)
	}
}

// A response sink can hold an HTTP writer, buffered events, and request context.
type detachedSessionSink struct{ strings.Builder }

func (s *detachedSessionSink) Emit(_ context.Context, event TurnEvent) error {
	_, err := s.WriteString(event.Text)
	return err
}

func TestDetachedSessionFormReleasesResponseSink(t *testing.T) {
	rt := &fakeConversationRuntime{}
	rt.prompt = func(_ context.Context, runtimeID, sessionID, _ string) error {
		rt.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventStructuredOutput, Payload: activity.StructuredOutputArtifact{
			RequestUserInput: &activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{ID: "q", Header: "Q", Question: "Continue?"}}},
		}})
		rt.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptCompleted})
		return nil
	}
	engine := New(newTestAgentService(t, []agent.Agent{{ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex, RuntimeID: "runtime-a", Status: string(runtime.StateRunning)}}, rt))
	conversation := engine.Conversations("agent-a")
	sink := new(detachedSessionSink)
	result := conversation.Run(t.Context(), TurnRequest{ID: "form", ConversationKey: "session", Input: textInput("Plan"), Interaction: InteractionUserInputOnly}, sink)
	if len(result.Interactions) != 1 || !result.Interactions[0].Detached {
		t.Fatalf("expected a detached form: %+v", result)
	}
	ref := weak.Make(sink)
	sink = nil
	goruntime.GC()
	if ref.Value() != nil {
		t.Fatal("completed HTTP turn is retaining its response sink")
	}
	forms, err := conversation.PendingInteractions(t.Context(), "session")
	if err != nil || len(forms) != 1 || forms[0].ID != result.Interactions[0].ID {
		t.Fatalf("abandoned form should remain recoverable until its deadline: %+v, %v", forms, err)
	}
}
