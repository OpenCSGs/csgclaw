package agentengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/runtime"
	"csgclaw/internal/runtime/codex"
)

func TestAgentInterfaceRemainsResourceShaped(t *testing.T) {
	typeOf := reflect.TypeOf((*AgentInterface)(nil)).Elem()
	want := []string{"Create", "Delete", "Get", "List", "Recreate", "Update"}
	if typeOf.NumMethod() != len(want) {
		t.Fatalf("AgentInterface methods = %d, want %d", typeOf.NumMethod(), len(want))
	}
	for index, name := range want {
		method := typeOf.Method(index)
		if method.Name != name {
			t.Fatalf("AgentInterface method %d = %s, want %s", index, method.Name, name)
		}
	}
}

type fakeConversationRuntime struct {
	mu              sync.Mutex
	subscribers     []chan activity.RuntimeEvent
	conversationKey string
	conversations   map[string]string
	workspace       string
	permission      *codex.MemoryPermissionBroker
	userInput       *codex.MemoryUserInputBroker
	turnID          string
	promptBlocks    []codex.PromptContentBlock
	prompt          func(context.Context, string, string, string) error
	provision       func(context.Context, runtime.ProvisionRequest) error
	state           runtime.State
	closeEvents     bool
	deleteCalls     int
}

func (*fakeConversationRuntime) Kind() string { return runtime.KindCodex }
func (*fakeConversationRuntime) Layout(root string) runtime.Layout {
	return runtime.Layout{
		WorkspaceRoot: filepath.Join(root, "workspace"),
		SkillsRoot:    filepath.Join(root, "workspace", ".agents", "skills"),
	}
}
func (*fakeConversationRuntime) New(_ context.Context, spec runtime.Spec) (runtime.Handle, error) {
	return runtime.Handle{RuntimeID: spec.RuntimeID}, nil
}
func (*fakeConversationRuntime) Start(context.Context, runtime.Handle) (runtime.State, error) {
	return runtime.StateRunning, nil
}
func (*fakeConversationRuntime) Stop(context.Context, runtime.Handle) (runtime.State, error) {
	return runtime.StateStopped, nil
}
func (f *fakeConversationRuntime) Delete(context.Context, runtime.Handle) error {
	f.deleteCalls++
	return nil
}
func (f *fakeConversationRuntime) State(context.Context, runtime.Handle) (runtime.State, error) {
	if f.state != "" {
		return f.state, nil
	}
	return runtime.StateRunning, nil
}
func (f *fakeConversationRuntime) Info(context.Context, runtime.Handle) (runtime.Info, error) {
	if f.state != "" {
		return runtime.Info{State: f.state}, nil
	}
	return runtime.Info{State: runtime.StateRunning}, nil
}
func (*fakeConversationRuntime) ValidateMCPServers(context.Context, runtime.MCPServersSnapshot) error {
	return nil
}
func (*fakeConversationRuntime) MCPServersRestartRequired(runtime.MCPServersChange) (bool, error) {
	return false, nil
}
func (*fakeConversationRuntime) ReconcileMCPServers(context.Context, runtime.Handle, runtime.MCPServersChange) error {
	return nil
}
func (f *fakeConversationRuntime) Provision(ctx context.Context, request runtime.ProvisionRequest) error {
	if f.provision != nil {
		return f.provision(ctx, request)
	}
	return nil
}
func (f *fakeConversationRuntime) EnsureEngineSession(_ context.Context, _, conversationKey string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.conversationKey = conversationKey
	if f.conversations == nil {
		f.conversations = make(map[string]string)
	}
	f.conversations[conversationKey] = "codex-thread"
	return "codex-thread", nil
}
func (f *fakeConversationRuntime) ExistingEngineSession(_ context.Context, _, conversationKey string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sessionID := f.conversations[conversationKey]
	return sessionID, sessionID != "", nil
}
func (f *fakeConversationRuntime) PromptTurn(ctx context.Context, runtimeID, sessionID, turnID string, blocks []codex.PromptContentBlock, accepted func()) error {
	f.mu.Lock()
	f.turnID = turnID
	f.promptBlocks = append([]codex.PromptContentBlock(nil), blocks...)
	f.mu.Unlock()
	if accepted != nil {
		accepted()
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Text != nil {
			parts = append(parts, block.Text.Text)
		} else if block.LocalImage != nil {
			parts = append(parts, block.LocalImage.Path)
		}
	}
	if f.prompt != nil {
		return f.prompt(ctx, runtimeID, sessionID, strings.Join(parts, "\n\n"))
	}
	return nil
}
func (f *fakeConversationRuntime) ResetConversation(_ context.Context, _, conversationKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.conversations, conversationKey)
	return nil
}
func (f *fakeConversationRuntime) WorkspaceDir(string) (string, error) {
	return f.workspace, nil
}
func (f *fakeConversationRuntime) PermissionBroker() codex.PermissionBroker {
	if f.permission == nil {
		f.permission = codex.NewPermissionBroker(nil)
	}
	return f.permission
}
func (f *fakeConversationRuntime) UserInputBroker() codex.UserInputBroker {
	if f.userInput == nil {
		f.userInput = codex.NewUserInputBroker(nil)
	}
	return f.userInput
}
func (f *fakeConversationRuntime) SubscribeSession(_, _ string) (<-chan activity.RuntimeEvent, func()) {
	ch := make(chan activity.RuntimeEvent, 8)
	if f.closeEvents {
		close(ch)
		return ch, func() {}
	}
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

func TestConversationRunWaitsForPromptAfterEventSubscriptionCloses(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{closeEvents: true}
	started := make(chan struct{})
	release := make(chan struct{})
	runtimeImpl.prompt = func(context.Context, string, string, string) error {
		close(started)
		<-release
		return nil
	}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	done := make(chan TurnResult, 1)
	go func() {
		done <- engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
			ID: "turn-1", ConversationKey: "conversation-1", Input: textInput("hello"),
		}, nil)
	}()
	<-started
	select {
	case result := <-done:
		t.Fatalf("Run returned before PromptTurn completed: %+v", result)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if result := <-done; result.Status != TurnFailed || result.Error == nil || result.Error.Code != ErrorRuntimeFailed || !result.Dispatched {
		t.Fatalf("Run() = %+v", result)
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

func TestConversationRunAuthorizesRuntimeOutputFile(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "reports"), 0o700); err != nil {
		t.Fatal(err)
	}
	content := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)
	outputPath := filepath.Join(workspace, "reports", "result.png")
	if err := os.WriteFile(outputPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeImpl := &fakeConversationRuntime{workspace: workspace}
	promptCalls := 0
	runtimeImpl.prompt = func(_ context.Context, runtimeID, sessionID, _ string) error {
		promptCalls++
		runtimeImpl.publish(activity.RuntimeEvent{
			RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventStructuredOutput,
			Payload: activity.StructuredOutputArtifact{ResourceLinks: []activity.ResourceLink{{Type: "resource_link", Name: "docs", URI: "https://example.com/docs"}}},
		})
		runtimeImpl.publish(activity.RuntimeEvent{
			RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventFileOutput,
			Payload: activity.RuntimeFile{Path: filepath.Join("reports", "result.png"), Name: "result.png", MIMEType: "image/png"},
		})
		runtimeImpl.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptCompleted})
		return nil
	}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	var outputs []*OutputItem
	request := TurnRequest{ID: "turn-output", ConversationKey: "conversation-output", Input: textInput("create an image")}
	result := engine.Conversations("agent-a").Run(context.Background(), request, EventSinkFunc(func(_ context.Context, event TurnEvent) error {
		if event.Output != nil {
			outputs = append(outputs, event.Output)
		}
		return nil
	}))
	if result.Status != TurnSucceeded || !result.Dispatched || len(outputs) != 1 || len(result.Files) != 1 {
		t.Fatalf("Run() = %+v, outputs = %+v", result, outputs)
	}
	if outputs[0].Kind != OutputItemResourceLink {
		t.Fatalf("first output = %+v, want ordinary resource link", outputs[0])
	}
	file := result.Files[0]
	if file.Name != "result.png" || file.MediaType != "image/png" || file.SizeBytes != int64(len(content)) || len(file.SHA256) != 64 || !strings.HasPrefix(file.ID, "file-") || file.ID == "sha256:"+file.SHA256 {
		t.Fatalf("OutputFile = %+v", file.OutputFileMetadata)
	}
	if err := os.WriteFile(outputPath, bytes.Repeat([]byte{1}, len(content)), 0o600); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		download, err := engine.Conversations("agent-a").Files().Get(context.Background(), file.ID)
		if err != nil {
			t.Fatal(err)
		}
		got, readErr := io.ReadAll(download.Content)
		closeErr := download.Content.Close()
		if download.Metadata != file.OutputFileMetadata || readErr != nil || closeErr != nil || !bytes.Equal(got, content) {
			t.Fatalf("attempt %d authorized file: metadata=%+v bytes=%d read=%v close=%v", attempt, download.Metadata, len(got), readErr, closeErr)
		}
	}
	replacedWorkspace := workspace + "-replaced"
	if err := os.Rename(workspace, replacedWorkspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(replacedWorkspace) })
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	download, err := engine.Conversations("agent-a").Files().Get(context.Background(), file.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, readErr := io.ReadAll(download.Content)
	closeErr := download.Content.Close()
	if download.Metadata != file.OutputFileMetadata || readErr != nil || closeErr != nil || !bytes.Equal(got, content) {
		t.Fatalf("replaced workspace metadata=%+v content=%q read=%v close=%v", download.Metadata, got, readErr, closeErr)
	}
	replayed := engine.Conversations("agent-a").Run(context.Background(), request, nil)
	if replayed.Status != TurnSucceeded || len(replayed.Files) != 1 || replayed.Files[0].ID != file.ID || promptCalls != 1 {
		t.Fatalf("replayed result=%+v promptCalls=%d", replayed, promptCalls)
	}
}

func TestConversationRunRejectsRuntimeOutputOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		t.Fatal(err)
	}
	runtimeImpl := &fakeConversationRuntime{workspace: workspace}
	runtimeImpl.prompt = func(_ context.Context, runtimeID, sessionID, _ string) error {
		runtimeImpl.publish(activity.RuntimeEvent{
			RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventFileOutput,
			Payload: activity.RuntimeFile{Path: filepath.Join("escape", "secret.txt")},
		})
		return nil
	}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	result := engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
		ID: "turn-output", ConversationKey: "conversation-output", Input: textInput("read outside")}, nil)
	if result.Status != TurnFailed || result.Error == nil || result.Error.Code != ErrorFileUnavailable || !result.Dispatched {
		t.Fatalf("Run() = %+v", result)
	}
	if strings.Contains(result.Error.Message, workspace) || strings.Contains(result.Error.Message, outside) {
		t.Fatalf("public error leaked workspace path: %q", result.Error.Message)
	}
}

func TestConversationRunCleansOutputSnapshotWhenLaterEventFails(t *testing.T) {
	snapshotDir := t.TempDir()
	t.Setenv("TMPDIR", snapshotDir)
	workspace := t.TempDir()
	content := []byte("temporary output")
	if err := os.WriteFile(filepath.Join(workspace, "result.txt"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeImpl := &fakeConversationRuntime{workspace: workspace}
	runtimeImpl.prompt = func(_ context.Context, runtimeID, sessionID, _ string) error {
		runtimeImpl.publish(activity.RuntimeEvent{
			RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventFileOutput,
			Payload: activity.RuntimeFile{Path: "result.txt", MIMEType: "text/plain"},
		})
		runtimeImpl.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptFailed, Error: "later failure"})
		return nil
	}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	result := engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
		ID: "turn-output", ConversationKey: "conversation-output", Input: textInput("create then fail"),
	}, nil)
	if result.Status != TurnFailed || result.Error == nil {
		t.Fatalf("Run() = %+v", result)
	}
	paths, err := filepath.Glob(filepath.Join(snapshotDir, "csgclaw-output-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("failed Turn retained snapshots: %v", paths)
	}
}

func TestConversationRunRejectsMismatchedRuntimeOutputMIMEAndCleansSnapshot(t *testing.T) {
	snapshotDir := t.TempDir()
	t.Setenv("TMPDIR", snapshotDir)
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "not-image.png"), []byte("plain text"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeImpl := &fakeConversationRuntime{workspace: workspace}
	runtimeImpl.prompt = func(_ context.Context, runtimeID, sessionID, _ string) error {
		runtimeImpl.publish(activity.RuntimeEvent{
			RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventFileOutput,
			Payload: activity.RuntimeFile{Path: "not-image.png", MIMEType: "image/png"},
		})
		return nil
	}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	result := engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
		ID: "turn-output", ConversationKey: "conversation-output", Input: textInput("publish image"),
	}, nil)
	if result.Status != TurnFailed || result.Error == nil || result.Error.Code != ErrorFileUnavailable {
		t.Fatalf("Run() = %+v", result)
	}
	paths, err := filepath.Glob(filepath.Join(snapshotDir, "csgclaw-output-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("MIME rejection retained snapshots: %v", paths)
	}
}

func TestEngineAgentsFacadeMapsAndRedactsCompleteState(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{state: runtime.StateStopped}
	service := newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Description: "desc", Instructions: "instructions", Role: agent.RoleWorker,
		RuntimeKind: agent.RuntimeKindCodex, RuntimeName: agent.RuntimeNameCodex, RuntimeID: "runtime-a",
		Status: string(runtime.StateStopped), RuntimeOptions: map[string]any{"approval": "never"},
		MCPServers: map[string]any{"docs": map[string]any{"command": "docs-server"}},
		AgentProfile: agent.AgentProfile{
			ModelProviderID: "provider-a", ModelID: "gpt-test", ReasoningEffort: "high",
			EnableFastMode: true, RequestOptions: map[string]any{"temperature": 0.1}, APIKey: "must-not-leak",
		},
	}}, runtimeImpl)
	engine := New(service)
	got, err := engine.Agents().Get(context.Background(), "A", AgentGetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "agent-a" || got.Spec.Runtime.Adapter != agent.RuntimeNameCodex ||
		got.Spec.Model.ProviderID != "provider-a" || got.Spec.Model.ModelID != "gpt-test" ||
		got.Spec.MCPServers["docs"]["command"] != "docs-server" || got.Status.State != AgentStateStopped {
		t.Fatalf("Agent = %+v", got)
	}
	if got.Spec.Runtime.Credentials != nil {
		t.Fatalf("Runtime credentials leaked: %+v", got.Spec.Runtime.Credentials)
	}
	updated, err := engine.Agents().Update(context.Background(), got.ID, AgentUpdateRequest{
		Spec: AgentSpec{
			Name: "A2", Description: "updated", Instructions: "new instructions", Role: AgentRoleWorker,
			Runtime: RuntimeSpec{Adapter: agent.RuntimeNameCodex, Options: map[string]any{"approval": "never"}},
			Model:   ModelSpec{ProviderID: "provider-a", ModelID: "gpt-test"},
		},
		ResourceVersion: got.ResourceVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Spec.Name != "A2" || updated.Spec.MCPServers["docs"] != nil || len(updated.Spec.MCPServers) != 0 {
		t.Fatalf("updated Agent = %+v", updated)
	}
	runtimeImpl.provision = func(_ context.Context, request runtime.ProvisionRequest) error {
		if request.Credentials["secrets/token"] != "secret" || request.InitShell != "test -f secrets/token" {
			t.Fatalf("Provision() request = %+v", request)
		}
		return nil
	}
	provisioned, err := engine.Agents().Update(context.Background(), got.ID, AgentUpdateRequest{
		Spec: AgentSpec{
			Name: "A2", Role: AgentRoleWorker,
			Runtime: RuntimeSpec{Adapter: agent.RuntimeNameCodex, Credentials: map[string]string{"secrets/token": "secret"}, InitShell: "test -f secrets/token"},
			Model:   ModelSpec{ProviderID: "provider-a", ModelID: "gpt-test"},
		},
		ResourceVersion: updated.ResourceVersion,
	})
	if err != nil || provisioned.Spec.Runtime.Credentials != nil || provisioned.Spec.Runtime.InitShell != "test -f secrets/token" {
		t.Fatalf("provisioned Agent = %+v, %v", provisioned, err)
	}
	runtimeImpl.provision = func(context.Context, runtime.ProvisionRequest) error { return errors.New("init failed") }
	if _, err := engine.Agents().Update(context.Background(), got.ID, AgentUpdateRequest{
		Spec: AgentSpec{
			Name: "A2", Role: AgentRoleWorker,
			Runtime: RuntimeSpec{Adapter: agent.RuntimeNameCodex, Credentials: map[string]string{"secrets/token": "replacement"}, InitShell: "exit 1"},
			Model:   ModelSpec{ProviderID: "provider-a", ModelID: "gpt-test"},
		},
		ResourceVersion: provisioned.ResourceVersion,
	}); err == nil || !strings.Contains(err.Error(), "init failed") {
		t.Fatalf("failed provisioning error = %v", err)
	}
	unchanged, err := engine.Agents().Get(context.Background(), got.ID, AgentGetOptions{})
	if err != nil || unchanged.Spec.Runtime.InitShell != "test -f secrets/token" {
		t.Fatalf("Agent changed after failed provisioning: %+v, %v", unchanged, err)
	}
}

func TestEngineAgentDesiredStateIsIndependentFromObservedRuntimeState(t *testing.T) {
	service := newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker,
		RuntimeKind: agent.RuntimeKindCodex, RuntimeName: agent.RuntimeNameCodex,
		Status: string(runtime.StateStopped),
	}}, &fakeConversationRuntime{state: runtime.StateStopped})
	if _, err := service.SetDesiredState("agent-a", agent.DesiredStateRunning, false); err != nil {
		t.Fatal(err)
	}
	engine := New(service)
	got, err := engine.Agents().Get(context.Background(), "agent-a", AgentGetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.DesiredState != AgentDesiredStateRunning || got.Status.State != AgentStateStopped {
		t.Fatalf("Agent desired/observed state = %q/%q, want running/stopped", got.Spec.DesiredState, got.Status.State)
	}
}

func TestEngineCreateManagerSkillFailurePreservesExistingManager(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{state: runtime.StateRunning}
	service := newTestAgentService(t, []agent.Agent{{
		ID: agent.ManagerUserID, Name: agent.ManagerName, Role: agent.RoleManager,
		RuntimeKind: agent.RuntimeKindCodex, RuntimeName: agent.RuntimeNameCodex,
		RuntimeID: "runtime-manager", BoxID: "manager-session", Status: string(runtime.StateRunning),
		AgentProfile: agent.AgentProfile{
			ModelProviderID: "provider-a", ModelID: "model-a", ProfileComplete: true,
		},
		ProfileComplete: true,
	}}, runtimeImpl)
	engine := New(service)
	_, err := engine.Agents().Create(context.Background(), AgentCreateRequest{
		ID: agent.ManagerUserID,
		Spec: AgentSpec{
			Name: agent.ManagerName, Role: AgentRoleManager,
			Runtime: RuntimeSpec{Adapter: agent.RuntimeNameCodex},
			Model:   ModelSpec{ProviderID: "provider-a", ModelID: "model-a"},
			Skills:  []string{"definitely-missing-skill"},
		},
	})
	if err == nil {
		t.Fatal("Create(manager) error = nil, want missing Skill failure")
	}
	manager, ok := service.Agent(agent.ManagerUserID)
	if !ok || manager.RuntimeID == "" {
		t.Fatalf("existing manager was removed: %+v, %t", manager, ok)
	}
	if runtimeImpl.deleteCalls != 0 {
		t.Fatalf("existing manager Runtime delete calls = %d", runtimeImpl.deleteCalls)
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
		ID: "turn-1", ConversationKey: "conversation-1", Input: textInput("hello"), Interaction: InteractionReject,
	}, nil)
	if result.Error == nil || result.Error.Code != ErrorInteractionUnsupported {
		t.Fatalf("result = %+v", result)
	}
}

func TestConversationResolveAddressesPendingUserInput(t *testing.T) {
	broker := codex.NewUserInputBroker(nil)
	runtimeImpl := &fakeConversationRuntime{userInput: broker}
	resolved := make(chan struct{})
	broker.AddDetachedHandler(func(codex.DetachedUserInputResolution) {
		close(resolved)
	})
	runtimeImpl.prompt = func(_ context.Context, runtimeID, sessionID, _ string) error {
		snapshot, err := broker.CreateDetached(codex.PendingUserInputRequest{
			Execution: activity.ExecutionRef{RuntimeID: runtimeID, SessionID: sessionID},
			Questions: []activity.UserInputQuestionSnapshot{{
				ID: "choice", Header: "Choice", Question: "Continue?",
				Options: []activity.UserInputOptionSnapshot{{Label: "Yes"}, {Label: "No"}},
			}},
		}, codex.DetachedUserInputContext{Channel: "test", RoomID: "room"})
		if err != nil {
			return err
		}
		runtimeImpl.publish(activity.RuntimeEvent{
			RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventUserInputRequest,
			UserInputID: snapshot.ID, Payload: snapshot,
		})
		<-resolved
		runtimeImpl.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptCompleted})
		return nil
	}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	interactionID := make(chan string, 1)
	runDone := make(chan TurnResult, 1)
	go func() {
		runDone <- engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
			ID: "turn-1", ConversationKey: "conversation-1", Input: textInput("hello"), Interaction: InteractionResolve,
		}, EventSinkFunc(func(_ context.Context, event TurnEvent) error {
			if event.Interaction != nil {
				interactionID <- event.Interaction.ID
			}
			return nil
		}))
	}()
	id := <-interactionID
	if err := engine.Conversations("agent-a").Resolve(context.Background(), InteractionResolution{
		ConversationKey: "conversation-1", InteractionID: id, ResponderID: "tester",
		Answers: map[string]InteractionAnswer{"choice": {Values: []string{"Yes"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if result := <-runDone; result.Status != TurnSucceeded || !result.Dispatched {
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
	promptStarted := make(chan struct{})
	runtimeImpl.prompt = func(ctx context.Context, _, _, _ string) error {
		close(promptStarted)
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
	<-promptStarted
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

func TestConversationRunRetainsDispatchedWhenFailureRacesAcceptance(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{}
	runtimeImpl.prompt = func(_ context.Context, runtimeID, sessionID, _ string) error {
		runtimeImpl.publish(activity.RuntimeEvent{
			RuntimeID: runtimeID, SessionID: sessionID,
			Kind: activity.RuntimeEventPromptFailed, Error: "runtime failed after acceptance",
		})
		return nil
	}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	for index := range 100 {
		result := engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
			ID:              TurnID(fmt.Sprintf("turn-%d", index)),
			ConversationKey: ConversationKey(fmt.Sprintf("conversation-%d", index)),
			Input:           textInput("hello"),
		}, nil)
		if result.Status != TurnFailed || result.Error == nil || result.Error.Code != ErrorRuntimeFailed || !result.Dispatched {
			t.Fatalf("Run(%d) = %+v", index, result)
		}
	}
}

func TestConversationRunCopiesCodexFileInput(t *testing.T) {
	content := []byte("report")
	runtimeImpl := &fakeConversationRuntime{workspace: t.TempDir()}
	var runtimePath string
	runtimeImpl.prompt = func(_ context.Context, runtimeID, sessionID, prompt string) error {
		runtimePath = strings.TrimSpace(strings.TrimPrefix(prompt, "Attached file \"report.txt\" is available in the Runtime workspace at "))
		info, err := os.Stat(runtimePath)
		if err != nil {
			t.Fatalf("stat Runtime-local file: %v", err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("Runtime-local file mode = %o, want 600", info.Mode().Perm())
		}
		runtimeImpl.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptCompleted})
		return nil
	}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	file := createEngineTestFile(t, engine, "agent-a", "report.txt", "text/plain", content)

	result := engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
		ID:              "turn-1",
		ConversationKey: "conversation-1",
		Input: []InputPart{{
			Kind: InputPartFile,
			File: &InputFile{ID: file.ID},
		}},
	}, nil)
	if result.Status != TurnSucceeded || result.Error != nil || !result.Dispatched {
		t.Fatalf("result = %+v", result)
	}
	if runtimeImpl.conversationKey != "conversation-1" {
		t.Fatalf("runtime conversation key = %q", runtimeImpl.conversationKey)
	}
	if runtimePath == "" {
		t.Fatal("Runtime-local path was not passed to Codex")
	}
	if _, err := os.Stat(runtimePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Runtime-local copy still exists after termination: %v", err)
	}
}

func TestConversationRunRejectsUnknownFileBeforeDispatch(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{workspace: t.TempDir()}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	result := engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
		ID: "turn-1", ConversationKey: "conversation-1",
		Input: []InputPart{{Kind: InputPartFile, File: &InputFile{ID: "missing-file"}}},
	}, nil)
	if result.Error == nil || result.Error.Code != ErrorFileNotFound || result.Dispatched {
		t.Fatalf("result = %+v", result)
	}
	if runtimeImpl.conversationKey != "" {
		t.Fatalf("Runtime mapping was touched before file resolution: %q", runtimeImpl.conversationKey)
	}
}

func TestConversationRunRejectsRuntimeInputDestinationSymlink(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, ".csgclaw")); err != nil {
		t.Fatal(err)
	}
	content := []byte("report")
	runtimeImpl := &fakeConversationRuntime{workspace: workspace}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	file := createEngineTestFile(t, engine, "agent-a", "report.txt", "text/plain", content)
	result := engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
		ID: "turn-1", ConversationKey: "conversation-1",
		Input: []InputPart{{Kind: InputPartFile, File: &InputFile{ID: file.ID}}},
	}, nil)
	if result.Error == nil || result.Error.Code != ErrorFileUnavailable || result.Dispatched {
		t.Fatalf("Run() = %+v", result)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("outside destination changed: entries=%v err=%v", entries, err)
	}
}

func TestCodexInputPreservesOrderedTextImageAndFileBlocks(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{workspace: t.TempDir()}
	runtimeImpl.prompt = func(_ context.Context, runtimeID, sessionID, _ string) error {
		runtimeImpl.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptCompleted})
		return nil
	}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	image := createEngineTestFile(t, engine, "agent-a", "image.png", "image/png", []byte("image"))
	notes := createEngineTestFile(t, engine, "agent-a", "notes.txt", "text/plain", []byte("notes"))
	result := engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
		ID: "caller-turn", ConversationKey: "conversation-1",
		Input: []InputPart{
			{Kind: InputPartText, Text: "before"},
			{Kind: InputPartFile, File: &InputFile{ID: image.ID}},
			{Kind: InputPartFile, File: &InputFile{ID: notes.ID}},
			{Kind: InputPartText, Text: "after"},
		},
	}, nil)
	if result.Status != TurnSucceeded {
		t.Fatalf("result = %+v", result)
	}
	runtimeImpl.mu.Lock()
	defer runtimeImpl.mu.Unlock()
	if runtimeImpl.turnID != "caller-turn" {
		t.Fatalf("clientUserMessageId = %q", runtimeImpl.turnID)
	}
	blocks := runtimeImpl.promptBlocks
	if len(blocks) != 4 || blocks[0].Text == nil || blocks[0].Text.Text != "before" ||
		blocks[1].LocalImage == nil || blocks[2].Text == nil || !strings.Contains(blocks[2].Text.Text, "notes.txt") ||
		blocks[3].Text == nil || blocks[3].Text.Text != "after" {
		t.Fatalf("prompt blocks = %#v", blocks)
	}
}

func TestConversationRunRejectsSameConversationOverlap(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	runtimeImpl.prompt = func(_ context.Context, runtimeID, sessionID, _ string) error {
		once.Do(func() { close(started) })
		<-release
		runtimeImpl.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptCompleted})
		return nil
	}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	firstDone := make(chan TurnResult, 1)
	go func() {
		firstDone <- engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
			ID: "turn-1", ConversationKey: "conversation-1", Input: textInput("first"),
		}, nil)
	}()
	<-started
	second := engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
		ID: "turn-2", ConversationKey: "conversation-1", Input: textInput("second"),
	}, nil)
	if second.Error == nil || second.Error.Code != ErrorConversationBusy || second.Dispatched {
		t.Fatalf("overlap result = %+v", second)
	}
	close(release)
	if result := <-firstDone; result.Status != TurnSucceeded {
		t.Fatalf("first result = %+v", result)
	}
}

func TestConversationRunRejectsMissingSupersedeFileWithoutCancelingActiveTurn(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{}
	started := make(chan struct{})
	release := make(chan struct{})
	var canceled atomic.Bool
	runtimeImpl.prompt = func(ctx context.Context, runtimeID, sessionID, _ string) error {
		close(started)
		select {
		case <-ctx.Done():
			canceled.Store(true)
			return ctx.Err()
		case <-release:
			runtimeImpl.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptCompleted})
			return nil
		}
	}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	firstDone := make(chan TurnResult, 1)
	go func() {
		firstDone <- engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
			ID: "turn-active", ConversationKey: "conversation-1", Input: textInput("active"),
		}, nil)
	}()
	<-started
	invalid := engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
		ID: "turn-invalid", ConversationKey: "conversation-1", Admission: AdmissionSupersede,
		Input: []InputPart{{Kind: InputPartFile, File: &InputFile{ID: "missing-file"}}},
	}, nil)
	if invalid.Error == nil || invalid.Error.Code != ErrorFileNotFound || invalid.Dispatched {
		t.Fatalf("invalid supersede result = %+v", invalid)
	}
	if canceled.Load() {
		t.Fatal("invalid supersede canceled the active turn")
	}
	close(release)
	if result := <-firstDone; result.Status != TurnSucceeded {
		t.Fatalf("active result = %+v", result)
	}
}

func TestConversationCancelIsExactAndWaitsForCleanup(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{}
	started := make(chan struct{})
	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	runtimeImpl.prompt = func(ctx context.Context, _, _, _ string) error {
		close(started)
		<-ctx.Done()
		close(cleanupStarted)
		<-releaseCleanup
		return ctx.Err()
	}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	runDone := make(chan TurnResult, 1)
	go func() {
		runDone <- engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
			ID: "turn-1", ConversationKey: "conversation-1", Input: textInput("hello"),
		}, nil)
	}()
	<-started
	if err := engine.Conversations("agent-a").Cancel(context.Background(), "conversation-1", "other-turn"); err != nil {
		t.Fatal(err)
	}
	cancelDone := make(chan error, 1)
	go func() {
		cancelDone <- engine.Conversations("agent-a").Cancel(context.Background(), "conversation-1", "turn-1")
	}()
	<-cleanupStarted
	select {
	case err := <-cancelDone:
		t.Fatalf("Cancel returned before cleanup: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseCleanup)
	if err := <-cancelDone; err != nil {
		t.Fatal(err)
	}
	if result := <-runDone; result.Status != TurnCanceled || result.Error == nil || result.Error.Code != ErrorCanceled {
		t.Fatalf("result = %+v", result)
	}
	if err := engine.Conversations("agent-a").Cancel(context.Background(), "conversation-1", "turn-1"); err != nil {
		t.Fatalf("idempotent Cancel error = %v", err)
	}
}

func TestConversationResetRemovesStrictContinuationMapping(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{}
	runtimeImpl.prompt = func(_ context.Context, runtimeID, sessionID, _ string) error {
		runtimeImpl.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptCompleted})
		return nil
	}
	engine := New(newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl))
	conversations := engine.Conversations("agent-a")
	first := conversations.Run(context.Background(), TurnRequest{
		ID: "turn-1", ConversationKey: "conversation-1", Input: textInput("hello"),
	}, nil)
	if first.Status != TurnSucceeded {
		t.Fatalf("first result = %+v", first)
	}
	if err := conversations.Reset(context.Background(), "conversation-1"); err != nil {
		t.Fatal(err)
	}
	strict := conversations.Run(context.Background(), TurnRequest{
		ID: "turn-2", ConversationKey: "conversation-1", Input: textInput("hello"), Continuation: ContinuationRequireExisting,
	}, nil)
	if strict.Error == nil || strict.Error.Code != ErrorConversationNotResumable || strict.Dispatched {
		t.Fatalf("strict result = %+v", strict)
	}
}

type directConversationResolver struct {
	runtime conversationRuntimeAdapter
}

func (r directConversationResolver) conversationRuntime(context.Context, string) (conversationRuntimeAdapter, func(), *TurnError) {
	return r.runtime, func() {}, nil
}

type interactionClaimRuntime struct {
	finishRun chan struct{}
	resolve   func(context.Context, InteractionRequest, InteractionResolution) *TurnError
}

func (r *interactionClaimRuntime) Run(ctx context.Context, _ TurnRequest, sink EventSink) TurnResult {
	if err := sink.Emit(ctx, TurnEvent{
		Kind:        TurnEventInteractionRequest,
		Interaction: &InteractionRequest{ID: "question-1", Kind: InteractionUserInput},
	}); err != nil {
		return TurnResult{Status: TurnFailed, Dispatched: true, Error: &TurnError{Code: ErrorRuntimeFailed, Message: err.Error()}}
	}
	<-r.finishRun
	return TurnResult{Status: TurnSucceeded, Dispatched: true}
}

func (*interactionClaimRuntime) Reset(context.Context, ConversationKey) *TurnError {
	return nil
}

func (r *interactionClaimRuntime) Resolve(ctx context.Context, request InteractionRequest, resolution InteractionResolution) *TurnError {
	return r.resolve(ctx, request, resolution)
}

func TestConversationResolveClaimsInteractionBeforeRuntimeCall(t *testing.T) {
	resolveStarted := make(chan struct{})
	releaseResolve := make(chan struct{})
	var resolveCalls atomic.Int32
	runtimeAdapter := &interactionClaimRuntime{
		finishRun: make(chan struct{}),
		resolve: func(context.Context, InteractionRequest, InteractionResolution) *TurnError {
			if resolveCalls.Add(1) == 1 {
				close(resolveStarted)
				<-releaseResolve
			}
			return nil
		},
	}
	engine := &Engine{runtimes: directConversationResolver{runtime: runtimeAdapter}}
	interactionReady := make(chan struct{})
	runDone := make(chan TurnResult, 1)
	go func() {
		runDone <- engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
			ID: "turn-1", ConversationKey: "conversation-1", Interaction: InteractionResolve, Input: textInput("hello"),
		}, EventSinkFunc(func(_ context.Context, event TurnEvent) error {
			if event.Interaction != nil {
				close(interactionReady)
			}
			return nil
		}))
	}()
	<-interactionReady
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- engine.Conversations("agent-a").Resolve(context.Background(), InteractionResolution{
			ConversationKey: "conversation-1", InteractionID: "question-1", ResponderID: "feishu:user-1",
		})
	}()
	<-resolveStarted
	second := engine.Conversations("agent-a").Resolve(context.Background(), InteractionResolution{
		ConversationKey: "conversation-1", InteractionID: "question-1", ResponderID: "feishu:user-1",
	})
	if ErrorCodeOf(second) != ErrorInteractionNotFound {
		t.Fatalf("duplicate Resolve error = %v", second)
	}
	if calls := resolveCalls.Load(); calls != 1 {
		t.Fatalf("Runtime Resolve calls = %d, want 1", calls)
	}
	close(releaseResolve)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	close(runtimeAdapter.finishRun)
	if result := <-runDone; result.Status != TurnSucceeded {
		t.Fatalf("Run() = %+v", result)
	}
}

func TestConversationResolveRestoresClaimAfterRuntimeFailure(t *testing.T) {
	var resolveCalls atomic.Int32
	runtimeAdapter := &interactionClaimRuntime{
		finishRun: make(chan struct{}),
		resolve: func(context.Context, InteractionRequest, InteractionResolution) *TurnError {
			if resolveCalls.Add(1) == 1 {
				return &TurnError{Code: ErrorRuntimeFailed, Message: "temporary failure"}
			}
			return nil
		},
	}
	engine := &Engine{runtimes: directConversationResolver{runtime: runtimeAdapter}}
	interactionReady := make(chan struct{})
	runDone := make(chan TurnResult, 1)
	go func() {
		runDone <- engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
			ID: "turn-1", ConversationKey: "conversation-1", Interaction: InteractionResolve, Input: textInput("hello"),
		}, EventSinkFunc(func(_ context.Context, event TurnEvent) error {
			if event.Interaction != nil {
				close(interactionReady)
			}
			return nil
		}))
	}()
	<-interactionReady
	resolution := InteractionResolution{ConversationKey: "conversation-1", InteractionID: "question-1", ResponderID: "feishu:user-1"}
	if err := engine.Conversations("agent-a").Resolve(context.Background(), resolution); ErrorCodeOf(err) != ErrorRuntimeFailed {
		t.Fatalf("first Resolve error = %v", err)
	}
	if err := engine.Conversations("agent-a").Resolve(context.Background(), resolution); err != nil {
		t.Fatalf("retried Resolve error = %v", err)
	}
	if calls := resolveCalls.Load(); calls != 2 {
		t.Fatalf("Runtime Resolve calls = %d, want 2", calls)
	}
	close(runtimeAdapter.finishRun)
	if result := <-runDone; result.Status != TurnSucceeded {
		t.Fatalf("Run() = %+v", result)
	}
}

func TestAgentStopDrainsActiveEngineTurn(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{}
	started := make(chan struct{})
	release := make(chan struct{})
	runtimeImpl.prompt = func(_ context.Context, runtimeID, sessionID, _ string) error {
		close(started)
		<-release
		runtimeImpl.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptCompleted})
		return nil
	}
	service := newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl)
	engine := New(service)
	runDone := make(chan TurnResult, 1)
	go func() {
		runDone <- engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
			ID: "turn-1", ConversationKey: "conversation-1", Input: textInput("hello"),
		}, nil)
	}()
	<-started
	stopDone := make(chan error, 1)
	go func() {
		current, err := engine.Agents().Get(context.Background(), "agent-a", AgentGetOptions{})
		if err == nil {
			current.Spec.DesiredState = AgentDesiredStateStopped
			_, err = engine.Agents().Update(context.Background(), "agent-a", AgentUpdateRequest{
				Spec: current.Spec, FieldMask: []string{"desired_state"}, ResourceVersion: current.ResourceVersion,
			})
		}
		stopDone <- err
	}()
	select {
	case err := <-stopDone:
		t.Fatalf("Stop returned before active Turn drained: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if result := <-runDone; result.Status != TurnSucceeded {
		t.Fatalf("run result = %+v", result)
	}
	if err := <-stopDone; err != nil {
		t.Fatal(err)
	}
}

func TestAgentLifecycleDrainTimeoutLeavesRuntimeUnchanged(t *testing.T) {
	runtimeImpl := &fakeConversationRuntime{}
	started := make(chan struct{})
	release := make(chan struct{})
	runtimeImpl.prompt = func(_ context.Context, runtimeID, sessionID, _ string) error {
		close(started)
		<-release
		runtimeImpl.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptCompleted})
		return nil
	}
	service := newTestAgentService(t, []agent.Agent{{
		ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
	}}, runtimeImpl)
	engine := New(service)
	runDone := make(chan TurnResult, 1)
	go func() {
		runDone <- engine.Conversations("agent-a").Run(context.Background(), TurnRequest{
			ID: "turn-1", ConversationKey: "conversation-1", Input: textInput("hello"),
		}, nil)
	}()
	<-started
	stopCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	current, err := engine.Agents().Get(context.Background(), "agent-a", AgentGetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	current.Spec.DesiredState = AgentDesiredStateStopped
	if _, err := engine.Agents().Update(stopCtx, "agent-a", AgentUpdateRequest{
		Spec: current.Spec, FieldMask: []string{"desired_state"}, ResourceVersion: current.ResourceVersion,
	}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop error = %v", err)
	}
	selected, ok := service.Agent("agent-a")
	if !ok || selected.RuntimeID != "runtime-a" || selected.Status != string(runtime.StateRunning) {
		t.Fatalf("Agent changed after drain timeout: %+v, %t", selected, ok)
	}
	close(release)
	if result := <-runDone; result.Status != TurnSucceeded {
		t.Fatalf("result = %+v", result)
	}
}

func TestPreserveWriteOnlyFieldsDistinguishesOmittedAndExplicitClear(t *testing.T) {
	current := AgentSpec{
		Runtime: RuntimeSpec{Credentials: map[string]string{"auth.json": "secret"}},
		Model:   ModelSpec{APIKey: "model-secret"},
	}
	preserved := preserveWriteOnlyFields(current, AgentSpec{})
	if preserved.Runtime.Credentials["auth.json"] != "secret" || preserved.Model.APIKey != "model-secret" {
		t.Fatalf("preserved write-only fields = %+v", preserved)
	}
	cleared := preserveWriteOnlyFields(current, AgentSpec{Runtime: RuntimeSpec{Credentials: map[string]string{}}})
	if cleared.Runtime.Credentials == nil || len(cleared.Runtime.Credentials) != 0 {
		t.Fatalf("explicit empty credentials = %#v, want explicit clear", cleared.Runtime.Credentials)
	}
}

func textInput(values ...string) []InputPart {
	parts := make([]InputPart, 0, len(values))
	for _, value := range values {
		parts = append(parts, InputPart{Kind: InputPartText, Text: value})
	}
	return parts
}

func createEngineTestFile(t testing.TB, engine Interface, agentID, name, mediaType string, content []byte) OutputFile {
	t.Helper()
	file, err := engine.Conversations(agentID).Files().Create(context.Background(), FileCreateRequest{
		Name: name, MIMEType: mediaType, SizeBytes: int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	return file
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
