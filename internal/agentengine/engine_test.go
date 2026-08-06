package agentengine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/runtime"
)

type fakeConversationRuntime struct {
	mu              sync.Mutex
	subscribers     []chan activity.RuntimeEvent
	conversationKey string
	prompt          func(context.Context, string, string, string) error
}

func (*fakeConversationRuntime) Kind() string                 { return runtime.KindCodex }
func (*fakeConversationRuntime) Layout(string) runtime.Layout { return runtime.Layout{} }
func (*fakeConversationRuntime) New(_ context.Context, spec runtime.Spec) (runtime.Handle, error) {
	return runtime.Handle{RuntimeID: spec.RuntimeID}, nil
}
func (*fakeConversationRuntime) Start(context.Context, runtime.Handle) (runtime.State, error) {
	return runtime.StateRunning, nil
}
func (*fakeConversationRuntime) Stop(context.Context, runtime.Handle) (runtime.State, error) {
	return runtime.StateStopped, nil
}
func (*fakeConversationRuntime) Delete(context.Context, runtime.Handle) error { return nil }
func (*fakeConversationRuntime) State(context.Context, runtime.Handle) (runtime.State, error) {
	return runtime.StateRunning, nil
}
func (*fakeConversationRuntime) Info(context.Context, runtime.Handle) (runtime.Info, error) {
	return runtime.Info{State: runtime.StateRunning}, nil
}
func (f *fakeConversationRuntime) EnsureSession(_ context.Context, _, conversationKey string) (string, error) {
	f.conversationKey = conversationKey
	return "codex-thread", nil
}
func (f *fakeConversationRuntime) Prompt(ctx context.Context, runtimeID, sessionID, prompt string) error {
	if f.prompt != nil {
		return f.prompt(ctx, runtimeID, sessionID, prompt)
	}
	return nil
}
func (f *fakeConversationRuntime) SubscribeSession(_, _ string) (<-chan activity.RuntimeEvent, func()) {
	ch := make(chan activity.RuntimeEvent, 8)
	f.mu.Lock()
	f.subscribers = append(f.subscribers, ch)
	f.mu.Unlock()
	return ch, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		for i, candidate := range f.subscribers {
			if candidate == ch {
				f.subscribers = append(f.subscribers[:i], f.subscribers[i+1:]...)
				break
			}
		}
	}
}
func (f *fakeConversationRuntime) publish(event activity.RuntimeEvent) {
	f.mu.Lock()
	subscribers := append([]chan activity.RuntimeEvent(nil), f.subscribers...)
	f.mu.Unlock()
	for _, subscriber := range subscribers {
		subscriber <- event
	}
}

func TestConversationRunStreamsCodexTextAndTools(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	runtimeImpl.prompt = func(_ context.Context, runtimeID, sessionID, prompt string) error {
		if runtimeID != "runtime-a" || sessionID != "codex-thread" || prompt != "hello\n\nworld" {
			t.Fatalf("unexpected prompt: %q %q %q", runtimeID, sessionID, prompt)
		}
		runtimeImpl.publish(activity.RuntimeEvent{
			RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventToolCallStart,
			ToolCallID: "call-1", ToolKind: "exec_command", Payload: map[string]any{"command": "pwd"},
		})
		runtimeImpl.publish(activity.RuntimeEvent{
			RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventTextDelta,
			Text: "answer", Payload: map[string]any{"phase": "final_answer"},
		})
		runtimeImpl.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptCompleted})
		return nil
	}

	var events []TurnEvent
	result := engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
		ID: "turn-1", ConversationKey: "conversation-1", Input: textInput("hello", "world"),
	}, EventSinkFunc(func(_ context.Context, event TurnEvent) error {
		events = append(events, event)
		return nil
	}))
	if result.Status != TurnSucceeded || result.Output != "answer" || result.Error != nil {
		t.Fatalf("result = %+v", result)
	}
	if runtimeImpl.conversationKey != "conversation-1" {
		t.Fatalf("conversation key = %q", runtimeImpl.conversationKey)
	}
	if len(events) != 2 || events[0].Kind != TurnEventToolCallStart || events[1].Kind != TurnEventTextDelta {
		t.Fatalf("events = %#v", events)
	}
}

func TestConversationRunStreamsCodexTextWithNullPhase(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	runtimeImpl.prompt = func(_ context.Context, runtimeID, sessionID, _ string) error {
		runtimeImpl.publish(activity.RuntimeEvent{
			RuntimeID: runtimeID,
			SessionID: sessionID,
			Kind:      activity.RuntimeEventTextDelta,
			Text:      "answer",
			Payload:   map[string]any{"phase": nil},
		})
		runtimeImpl.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptCompleted})
		return nil
	}

	result := engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
		ID: "turn-1", ConversationKey: "conversation-1", Input: textInput("hello"),
	}, nil)
	if result.Status != TurnSucceeded || result.Output != "answer" || result.Error != nil {
		t.Fatalf("result = %+v", result)
	}
}

func TestConversationRunRejectsUnsupportedInteraction(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	runtimeImpl.prompt = func(ctx context.Context, runtimeID, sessionID, _ string) error {
		runtimeImpl.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventUserInputRequest})
		<-ctx.Done()
		return ctx.Err()
	}
	result := engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
		ID: "turn-1", ConversationKey: "conversation-1", Input: textInput("hello"),
	}, nil)
	if result.Error == nil || result.Error.Code != ErrorInteractionUnsupported {
		t.Fatalf("result = %+v", result)
	}
}

func TestConversationRunPropagatesContextCancellation(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	runtimeImpl.prompt = func(ctx context.Context, _, _, _ string) error {
		<-ctx.Done()
		close(cleanupStarted)
		<-releaseCleanup
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan TurnResult, 1)
	go func() {
		done <- engine.Conversations("agent-a").Run(ctx, TurnRequest{
			ID: "turn-1", ConversationKey: "conversation-1", Input: textInput("hello"),
		}, nil)
	}()
	cancel()
	<-cleanupStarted
	select {
	case result := <-done:
		t.Fatalf("Run returned before runtime cancellation completed: %+v", result)
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseCleanup)
	result := <-done
	if result.Status != TurnCanceled {
		t.Fatalf("result = %+v", result)
	}
}

func TestConversationRunWaitsForRuntimeCleanupAfterSinkFailure(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	runtimeImpl.prompt = func(ctx context.Context, runtimeID, sessionID, _ string) error {
		runtimeImpl.publish(activity.RuntimeEvent{
			RuntimeID: runtimeID,
			SessionID: sessionID,
			Kind:      activity.RuntimeEventTextDelta,
			Text:      "partial",
			Payload:   map[string]any{"phase": "final_answer"},
		})
		<-ctx.Done()
		close(cleanupStarted)
		<-releaseCleanup
		return ctx.Err()
	}
	done := make(chan TurnResult, 1)
	go func() {
		done <- engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
			ID: "turn-1", ConversationKey: "conversation-1", Input: textInput("hello"),
		}, EventSinkFunc(func(context.Context, TurnEvent) error {
			return errors.New("client disconnected")
		}))
	}()
	<-cleanupStarted
	select {
	case result := <-done:
		t.Fatalf("Run returned before runtime cancellation completed: %+v", result)
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseCleanup)
	result := <-done
	if result.Status != TurnFailed || result.Error == nil || !strings.Contains(result.Error.Message, "client disconnected") {
		t.Fatalf("result = %+v", result)
	}
}

func TestConversationRunRejectsFileInputUntilCodexSupportsIt(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))

	result := engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
		ID:              "turn-1",
		ConversationKey: "conversation-1",
		Input: []InputPart{{
			Kind: InputPartFile,
			File: &InputFile{ID: "file-1", SourcePath: "/authorized/report.pdf", Name: "report.pdf"},
		}},
	}, nil)
	if result.Status != TurnFailed || result.Error == nil || result.Error.Code != ErrorFileUnavailable {
		t.Fatalf("result = %+v", result)
	}
	if runtimeImpl.conversationKey != "" {
		t.Fatalf("runtime was called for unsupported file input: %q", runtimeImpl.conversationKey)
	}
}

func textInput(values ...string) []InputPart {
	parts := make([]InputPart, 0, len(values))
	for _, value := range values {
		parts = append(parts, InputPart{Kind: InputPartText, Text: value})
	}
	return parts
}

func newTestAgentService(t *testing.T, agents []agent.Agent, runtimeImpl runtime.Runtime) *agent.Service {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "agents.json")
	data, err := json.Marshal(map[string]any{"agents": agents})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := agent.NewService(config.ModelConfig{}, config.ServerConfig{}, "manager:test", statePath, agent.WithRuntime(runtimeImpl))
	if err != nil {
		t.Fatal(err)
	}
	return service
}
