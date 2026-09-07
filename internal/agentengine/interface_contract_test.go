package agentengine_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentengine/enginetest"
	"csgclaw/internal/config"
	"csgclaw/internal/runtime"
	"csgclaw/internal/runtime/codex"
)

func TestRealEngineInterfaceContract(t *testing.T) {
	enginetest.RunInterfaceContract(t, newRealContractEngine)
}

type contractRuntime struct {
	inputStarted chan struct{}
	kind         string

	mu            sync.Mutex
	states        map[string]runtime.State
	conversations map[string]string
	sessionKeys   map[string]agentengine.ConversationKey
	subscribers   []chan activity.RuntimeEvent
	behavior      enginetest.TurnBehavior
	workspace     string
	permission    *codex.MemoryPermissionBroker
	userInput     *codex.MemoryUserInputBroker
	openFile      func(context.Context, string) (io.ReadCloser, error)
}

func (r *contractRuntime) Conversation(runtimeID string) agentengine.RuntimeConversation {
	if r.kind != "codex" {
		return nil
	}
	return codex.NewConversationAdapter(runtimeID, r)
}

func newContractRuntime(kind, workspace string, behavior enginetest.TurnBehavior) *contractRuntime {
	r := &contractRuntime{
		kind: kind, states: make(map[string]runtime.State), conversations: make(map[string]string),
		sessionKeys: make(map[string]agentengine.ConversationKey), behavior: behavior, workspace: workspace,
		permission: codex.NewPermissionBroker(nil), inputStarted: make(chan struct{}, 16),
	}
	r.userInput = codex.NewUserInputBroker(contractInputSink{runtime: r})
	return r
}

func (r *contractRuntime) Kind() string { return r.kind }

func (r *contractRuntime) RuntimeExtensionDriver(kind string) (runtime.ExtensionDriver, bool) {
	if kind != "contract" || r.kind != runtime.KindCodex {
		return nil, false
	}
	return r, true
}

func (r *contractRuntime) ObserveExtension(_ context.Context, _ string, _ runtime.ExtensionDesired) (runtime.ExtensionResult, error) {
	return runtime.ExtensionResult{State: runtime.ExtensionStateConfigured, CheckedAt: time.Now().UTC(), RuntimeLoaded: true}, nil
}

type contractExtensionSource struct{}

func (contractExtensionSource) Resolve(context.Context, string, string) (agentengine.ResolvedRuntimeExtension, error) {
	return agentengine.ResolvedRuntimeExtension{SourceRevision: "contract-revision", Payload: json.RawMessage(`{"value":"resolved-only"}`)}, nil
}

func (r *contractRuntime) Layout(root string) runtime.Layout {
	return runtime.Layout{
		WorkspaceRoot: filepath.Join(root, "workspace"),
		SkillsRoot:    filepath.Join(root, "workspace", ".agents", "skills"),
	}
}

func (r *contractRuntime) New(_ context.Context, spec runtime.Spec) (runtime.Handle, error) {
	runtimeID := strings.TrimSpace(spec.RuntimeID)
	if runtimeID == "" {
		runtimeID = "runtime-" + strings.TrimSpace(spec.AgentID)
	}
	r.mu.Lock()
	r.states[runtimeID] = runtime.StateCreated
	r.mu.Unlock()
	return runtime.Handle{RuntimeID: runtimeID}, nil
}

func (r *contractRuntime) Start(_ context.Context, handle runtime.Handle) (runtime.State, error) {
	r.setState(handle.RuntimeID, runtime.StateRunning)
	return runtime.StateRunning, nil
}

func (r *contractRuntime) Stop(_ context.Context, handle runtime.Handle) (runtime.State, error) {
	r.setState(handle.RuntimeID, runtime.StateStopped)
	return runtime.StateStopped, nil
}

func (r *contractRuntime) Delete(_ context.Context, handle runtime.Handle) error {
	r.mu.Lock()
	delete(r.states, handle.RuntimeID)
	r.mu.Unlock()
	return nil
}

func (r *contractRuntime) State(_ context.Context, handle runtime.Handle) (runtime.State, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state := r.states[handle.RuntimeID]
	if state == "" {
		state = runtime.StateStopped
	}
	return state, nil
}

func (r *contractRuntime) Info(ctx context.Context, handle runtime.Handle) (runtime.Info, error) {
	state, err := r.State(ctx, handle)
	return runtime.Info{HandleID: handle.RuntimeID, State: state}, err
}

func (*contractRuntime) ValidateMCPServers(context.Context, runtime.MCPServersSnapshot) error {
	return nil
}

func (*contractRuntime) MCPServersRestartRequired(runtime.MCPServersChange) (bool, error) {
	return false, nil
}

func (*contractRuntime) ReconcileMCPServers(context.Context, runtime.Handle, runtime.MCPServersChange) error {
	return nil
}

func (*contractRuntime) ReadMemoryDocument(_ context.Context, _ string, options map[string]any) (runtime.MemoryDocument, error) {
	return runtime.MemoryDocument{Enabled: options["memory_mode"] != "disabled", Ready: true, Name: "MEMORY.md"}, nil
}

func (*contractRuntime) ConfigureMemory(options map[string]any, enabled bool) (map[string]any, error) {
	next := make(map[string]any, len(options)+1)
	for key, value := range options {
		next[key] = value
	}
	if enabled {
		next["memory_mode"] = "enabled"
	} else {
		next["memory_mode"] = "disabled"
	}
	return next, nil
}

func (r *contractRuntime) EnsureEngineSession(_ context.Context, _ string, conversationKey string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if sessionID := r.conversations[conversationKey]; sessionID != "" {
		return sessionID, nil
	}
	sessionID := "contract-session-" + conversationKey
	r.conversations[conversationKey] = sessionID
	r.sessionKeys[sessionID] = agentengine.ConversationKey(conversationKey)
	return sessionID, nil
}

func (r *contractRuntime) ExistingEngineSession(_ context.Context, _ string, conversationKey string) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sessionID := r.conversations[conversationKey]
	return sessionID, sessionID != "", nil
}

func (r *contractRuntime) PromptTurn(ctx context.Context, runtimeID, sessionID, turnID string, blocks []codex.PromptContentBlock, accepted func()) error {
	if accepted != nil {
		accepted()
	}
	r.mu.Lock()
	key := r.sessionKeys[sessionID]
	behavior := r.behavior
	r.mu.Unlock()
	request := agentengine.TurnRequest{ID: agentengine.TurnID(turnID), ConversationKey: key}
	for _, block := range blocks {
		if block.Text != nil {
			request.Input = append(request.Input, agentengine.InputPart{Kind: agentengine.InputPartText, Text: block.Text.Text})
		}
	}
	result := agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
	if behavior != nil {
		result = behavior(ctx, "agent-a", request, agentengine.EventSinkFunc(func(_ context.Context, event agentengine.TurnEvent) error {
			return r.publishTurnEvent(ctx, runtimeID, sessionID, event)
		}))
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if result.Status == agentengine.TurnFailed {
		message := "contract Runtime failed"
		if result.Error != nil && strings.TrimSpace(result.Error.Message) != "" {
			message = result.Error.Message
		}
		r.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptFailed, Error: message})
		return nil
	}
	for _, file := range result.Files {
		if r.openFile == nil {
			return fmt.Errorf("contract file opener is unavailable")
		}
		reader, err := r.openFile(context.Background(), file.ID)
		if err != nil {
			return err
		}
		content, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			return errors.Join(readErr, closeErr)
		}
		relativePath := filepath.Join("contract-outputs", file.Name)
		if err := os.MkdirAll(filepath.Join(r.workspace, filepath.Dir(relativePath)), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(r.workspace, relativePath), content, 0o600); err != nil {
			return err
		}
		r.publish(activity.RuntimeEvent{
			RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventFileOutput,
			Payload: activity.RuntimeFile{Path: relativePath, Name: file.Name, MIMEType: file.MediaType},
		})
	}
	r.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptCompleted})
	return nil
}

func (r *contractRuntime) ResetConversation(_ context.Context, _ string, conversationKey string) error {
	r.mu.Lock()
	sessionID := r.conversations[conversationKey]
	delete(r.conversations, conversationKey)
	delete(r.sessionKeys, sessionID)
	r.mu.Unlock()
	return nil
}

func (r *contractRuntime) WorkspaceDir(string) (string, error) { return r.workspace, nil }

func (r *contractRuntime) PermissionBroker() codex.PermissionBroker { return r.permission }

func (r *contractRuntime) UserInputBroker() codex.UserInputBroker { return r.userInput }

func (r *contractRuntime) SubscribeSession(_, _ string) (<-chan activity.RuntimeEvent, func()) {
	stream := make(chan activity.RuntimeEvent, 16)
	r.mu.Lock()
	r.subscribers = append(r.subscribers, stream)
	r.mu.Unlock()
	return stream, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		for index, candidate := range r.subscribers {
			if candidate == stream {
				r.subscribers = append(r.subscribers[:index], r.subscribers[index+1:]...)
				return
			}
		}
	}
}

func (r *contractRuntime) publishTurnEvent(ctx context.Context, runtimeID, sessionID string, event agentengine.TurnEvent) error {
	runtimeEvent := activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID}
	switch event.Kind {
	case agentengine.TurnEventTextDelta:
		runtimeEvent.Kind = activity.RuntimeEventTextDelta
		runtimeEvent.Text = event.Text
		runtimeEvent.Payload = map[string]any{"phase": "final_answer"}
	case agentengine.TurnEventThoughtDelta:
		runtimeEvent.Kind = activity.RuntimeEventThoughtDelta
		runtimeEvent.Text = event.Thought
	case agentengine.TurnEventToolCallStart, agentengine.TurnEventToolCallUpdate:
		if event.Tool == nil {
			return fmt.Errorf("tool event is missing tool data")
		}
		if event.Kind == agentengine.TurnEventToolCallStart {
			runtimeEvent.Kind = activity.RuntimeEventToolCallStart
		} else {
			runtimeEvent.Kind = activity.RuntimeEventToolCallUpdate
		}
		runtimeEvent.ToolCallID = event.Tool.ID
		runtimeEvent.ToolKind = event.Tool.Kind
		runtimeEvent.ToolTitle = event.Tool.Title
		runtimeEvent.ToolStatus = event.Tool.Status
		runtimeEvent.ToolInputSummary = event.Tool.InputSummary
		runtimeEvent.ToolOutputSummary = event.Tool.OutputSummary
		runtimeEvent.Payload = event.Tool.Payload
	case agentengine.TurnEventActivityUpdate:
		if event.Activity == nil {
			return fmt.Errorf("activity event is missing activity data")
		}
		runtimeEvent.Kind = activity.RuntimeEventPlanUpdate
		runtimeEvent.ActionID = event.Activity.ID
		runtimeEvent.ActionStatus = event.Activity.Status
		runtimeEvent.Payload = event.Activity.Payload
	case agentengine.TurnEventOutputItem:
		if event.Output == nil {
			return fmt.Errorf("output event is missing output data")
		}
		artifact := activity.StructuredOutputArtifact{}
		switch event.Output.Kind {
		case agentengine.OutputItemResourceLink:
			link, ok := event.Output.Payload.(activity.ResourceLink)
			if !ok {
				return fmt.Errorf("resource link output has invalid payload")
			}
			artifact.ResourceLinks = []activity.ResourceLink{link}
		case agentengine.OutputItemRequestUserInput:
			request, ok := event.Output.Payload.(activity.RequestUserInputArgs)
			if !ok {
				return fmt.Errorf("request user input output has invalid payload")
			}
			artifact.RequestUserInput = &request
		default:
			return fmt.Errorf("unsupported output kind %q", event.Output.Kind)
		}
		runtimeEvent.Kind = activity.RuntimeEventStructuredOutput
		runtimeEvent.Payload = artifact
	case agentengine.TurnEventInteractionRequest:
		if event.Interaction == nil || event.Interaction.Kind != agentengine.InteractionUserInput {
			return fmt.Errorf("unsupported contract interaction")
		}
		go func() {
			_, _ = r.userInput.Request(ctx, codex.PendingUserInputRequest{
				Execution: activity.ExecutionRef{RuntimeID: runtimeID, SessionID: sessionID},
				Questions: []activity.UserInputQuestionSnapshot{{ID: "choice", Header: "Choice", Question: "Continue?", Options: []activity.UserInputOptionSnapshot{{Label: "Yes"}, {Label: "No"}}}},
			})
		}()
		select {
		case <-r.inputStarted:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	default:
		return fmt.Errorf("unsupported contract event %q", event.Kind)
	}
	r.publish(runtimeEvent)
	return nil
}

func (r *contractRuntime) publish(event activity.RuntimeEvent) {
	r.mu.Lock()
	streams := append([]chan activity.RuntimeEvent(nil), r.subscribers...)
	r.mu.Unlock()
	for _, stream := range streams {
		stream <- event
	}
}

func (r *contractRuntime) setState(runtimeID string, state runtime.State) {
	r.mu.Lock()
	r.states[runtimeID] = state
	r.mu.Unlock()
}

func newRealContractEngine(t testing.TB, seeded []agentengine.Agent, behavior enginetest.TurnBehavior) agentengine.Interface {
	t.Helper()
	root := t.TempDir()
	codexWorkspace := filepath.Join(root, "workspace")
	openClawWorkspace := filepath.Join(root, "openclaw-workspace")
	for _, workspace := range []string{codexWorkspace, openClawWorkspace} {
		if err := os.MkdirAll(workspace, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	codexRuntime := newContractRuntime(runtime.KindCodex, codexWorkspace, behavior)
	openClawRuntime := newContractRuntime(runtime.KindOpenClawSandbox, openClawWorkspace, behavior)
	stored := make([]agent.Agent, 0, len(seeded))
	for _, item := range seeded {
		runtimeKind := strings.TrimSpace(item.Spec.Runtime.Adapter)
		runtimeName := runtimeKind
		if runtimeKind == "" || runtimeKind == "codex" {
			runtimeKind = agent.RuntimeKindCodex
			runtimeName = agent.RuntimeNameCodex
		}
		stored = append(stored, agent.Agent{
			ID: item.ID, Name: item.Spec.Name, Description: item.Spec.Description, Instructions: item.Spec.Instructions,
			Role: string(item.Spec.Role), RuntimeKind: runtimeKind, RuntimeName: runtimeName,
			RuntimeID: item.Status.RuntimeID, BoxID: item.Status.RuntimeID, Status: string(item.Status.State), RuntimeOptions: item.Spec.Runtime.Options,
			AgentProfile: agent.AgentProfile{
				ModelProviderID: item.Spec.Model.ProviderID, ModelID: item.Spec.Model.ModelID,
				ReasoningEffort: item.Spec.Model.ReasoningEffort, EnableFastMode: item.Spec.Model.FastMode,
				RequestOptions: item.Spec.Model.Options,
			},
		})
		if runtimeKind == agent.RuntimeKindCodex {
			codexRuntime.setState(item.Status.RuntimeID, runtime.State(item.Status.State))
		} else {
			openClawRuntime.setState(item.Status.RuntimeID, runtime.State(item.Status.State))
		}
	}
	statePath := filepath.Join(root, "agents.json")
	data, err := json.Marshal(map[string]any{"agents": stored})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := agent.NewController(
		config.ModelConfig{}, config.ServerConfig{}, "manager:test", statePath,
		agent.WithRuntime(codexRuntime), agent.WithRuntime(openClawRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	engine := agentengine.New(service)
	if err := engine.RegisterRuntimeExtensionSource("contract", contractExtensionSource{}); err != nil {
		t.Fatal(err)
	}
	codexRuntime.openFile = func(ctx context.Context, fileID string) (io.ReadCloser, error) {
		download, err := engine.Conversations("agent-a").Files().Get(ctx, fileID)
		if err != nil {
			return nil, err
		}
		return download.Content, nil
	}
	return engine
}

type contractInputSink struct{ runtime *contractRuntime }

func (s contractInputSink) Publish(event codex.SessionEvent) {
	s.runtime.publish(event)
	if event.Kind == activity.RuntimeEventUserInputRequest {
		s.runtime.inputStarted <- struct{}{}
	}
}
