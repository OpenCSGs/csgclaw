package api

import (
	"bytes"
	"context"
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentengine/enginetest"
	"csgclaw/internal/agentsession"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSessionEngine struct {
	run       func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult
	beforeRun func(context.Context)
	client    *enginetest.MemoryClient
}

func (e *fakeSessionEngine) Agents() agentengine.AgentInterface {
	return e.client.Agents()
}

func (e *fakeSessionEngine) Conversations(agentID string) agentengine.ConversationInterface {
	return fakeSessionConversations{engine: e, delegate: e.client.Conversations(agentID)}
}

func (e *fakeSessionEngine) RuntimeExtensions(agentID string) agentengine.RuntimeExtensionInterface {
	return e.client.RuntimeExtensions(agentID)
}

type fakeSessionConversations struct {
	engine   *fakeSessionEngine
	delegate agentengine.ConversationInterface
}

func (c fakeSessionConversations) Files() agentengine.FileInterface {
	return c.delegate.Files()
}

func (c fakeSessionConversations) Run(ctx context.Context, request agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
	if c.engine.beforeRun != nil {
		c.engine.beforeRun(ctx)
	}
	return c.delegate.Run(ctx, request, sink)
}

func (c fakeSessionConversations) Cancel(ctx context.Context, key agentengine.ConversationKey, turnID agentengine.TurnID) error {
	return c.delegate.Cancel(ctx, key, turnID)
}

func (c fakeSessionConversations) Reset(ctx context.Context, key agentengine.ConversationKey) error {
	return c.delegate.Reset(ctx, key)
}

func (c fakeSessionConversations) Resolve(ctx context.Context, resolution agentengine.InteractionResolution) error {
	return c.delegate.Resolve(ctx, resolution)
}

func TestAgentSessionResponsesUsesEngineWithoutCreatingIMEntities(t *testing.T) {
	agentItem := sessionCodexAgent("agent-alpha", "Alpha")
	var gotAgentID string
	var gotRequest agentengine.TurnRequest
	engine := &fakeSessionEngine{run: func(_ context.Context, agentID string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		gotAgentID = agentID
		gotRequest = request
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "final answer"}
	}}
	handler, imSvc, bindings, _ := newAgentSessionTestHandler(t, []agent.Agent{agentItem}, engine, "")
	before := imSvc.Bootstrap()

	recorder := performAgentSessionRequest(t, handler, "Alpha", "session-123", map[string]any{"input": "Review this"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var response agentSessionResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "completed" || response.Model != agentItem.ID || response.Output[0].Content[0].Text != "final answer" {
		t.Fatalf("response = %+v", response)
	}
	if response.Metadata["session_id"] != "session-123" || response.Metadata["room_id"] != "" || response.Metadata["agent_id"] != agentItem.ID {
		t.Fatalf("metadata = %#v", response.Metadata)
	}
	if gotAgentID != agentItem.ID || len(gotRequest.Input) != 1 || gotRequest.Input[0].Kind != agentengine.InputPartText ||
		gotRequest.Input[0].Text != "Review this" || gotRequest.Input[0].File != nil || gotRequest.ID == "" || gotRequest.ConversationKey == "" {
		t.Fatalf("engine call = agent %q, request %+v", gotAgentID, gotRequest)
	}
	if got, want := len(bindings.Bindings()), 1; got != want {
		t.Fatalf("binding count = %d, want %d", got, want)
	}
	after := imSvc.Bootstrap()
	if len(after.Rooms) != len(before.Rooms) || len(after.Users) != len(before.Users) {
		t.Fatalf("IM changed: before=%+v after=%+v", before, after)
	}
}

func TestAgentSessionResponsesSupportsMessageInput(t *testing.T) {
	var input string
	engine := &fakeSessionEngine{run: func(_ context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		input = sessionTurnText(request)
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "ok"}
	}}
	handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	recorder := performAgentSessionRequest(t, handler, "agent-alpha", "message-input", map[string]any{
		"input": []map[string]any{
			{"type": "message", "role": "user", "content": "First"},
			{"role": "user", "content": []map[string]any{{"type": "input_text", "text": "Second"}}},
		},
	})
	if recorder.Code != http.StatusOK || input != "First\n\nSecond" {
		t.Fatalf("status = %d, input = %q, body=%s", recorder.Code, input, recorder.Body.String())
	}
}

func TestAgentSessionResponsesRejectsOverlapAndScopesBusyByAgent(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	engine := &fakeSessionEngine{run: func(_ context.Context, agentID string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		if agentID == "agent-alpha" && sessionTurnText(request) == "wait" {
			once.Do(func() { close(started) })
			<-release
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: agentID}
	}}
	handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{
		sessionCodexAgent("agent-alpha", "Alpha"),
		sessionCodexAgent("agent-beta", "Beta"),
	}, engine, "")

	firstResult := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstResult <- performAgentSessionRequest(t, handler, "agent-alpha", "shared", map[string]any{"input": "wait"})
	}()
	<-started

	overlap := performAgentSessionRequest(t, handler, "agent-alpha", "shared", map[string]any{"input": "overlap"})
	if overlap.Code != http.StatusConflict || !strings.Contains(overlap.Body.String(), "session_busy") {
		t.Fatalf("overlap status = %d, body=%s", overlap.Code, overlap.Body.String())
	}
	otherSession := performAgentSessionRequest(t, handler, "agent-alpha", "other", map[string]any{"input": "parallel"})
	if otherSession.Code != http.StatusOK {
		t.Fatalf("other Session status = %d, body=%s", otherSession.Code, otherSession.Body.String())
	}
	otherAgent := performAgentSessionRequest(t, handler, "agent-beta", "shared", map[string]any{"input": "parallel"})
	if otherAgent.Code != http.StatusOK {
		t.Fatalf("other Agent status = %d, body=%s", otherAgent.Code, otherAgent.Body.String())
	}

	close(release)
	select {
	case result := <-firstResult:
		if result.Code != http.StatusOK {
			t.Fatalf("first status = %d, body=%s", result.Code, result.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first response did not finish")
	}
}

func TestAgentSessionResponsesRejectsOverlapUntilCanceledTurnCleanup(t *testing.T) {
	started := make(chan struct{})
	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	engine := &fakeSessionEngine{run: func(ctx context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		if sessionTurnText(request) != "first" {
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "second finished"}
		}
		close(started)
		<-ctx.Done()
		close(cleanupStarted)
		<-releaseCleanup
		return agentengine.TurnResult{
			Status: agentengine.TurnCanceled,
			Error:  &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: ctx.Err().Error()},
		}
	}}
	handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	router := handler.Routes()

	firstContext, cancelFirst := context.WithCancel(context.Background())
	firstRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/agents/agent-alpha/sessions/cancel-cleanup/responses",
		strings.NewReader(`{"input":"first"}`),
	).WithContext(firstContext)
	firstRecorder := httptest.NewRecorder()
	firstDone := make(chan struct{})
	go func() {
		router.ServeHTTP(firstRecorder, firstRequest)
		close(firstDone)
	}()
	<-started
	cancelFirst()
	<-cleanupStarted

	secondRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/agents/agent-alpha/sessions/cancel-cleanup/responses",
		strings.NewReader(`{"input":"second"}`),
	)
	secondRecorder := httptest.NewRecorder()
	router.ServeHTTP(secondRecorder, secondRequest)
	if secondRecorder.Code != http.StatusConflict || !strings.Contains(secondRecorder.Body.String(), "session_busy") {
		t.Fatalf("overlap status=%d body=%s", secondRecorder.Code, secondRecorder.Body.String())
	}

	close(releaseCleanup)
	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled response did not finish cleanup")
	}
	retry := performAgentSessionRequest(t, handler, "agent-alpha", "cancel-cleanup", map[string]any{"input": "second"})
	if retry.Code != http.StatusOK || !strings.Contains(retry.Body.String(), "second finished") {
		t.Fatalf("retry status=%d body=%s", retry.Code, retry.Body.String())
	}
}

func TestAgentSessionResponsesCancelEndpointWaitsForRuntimeCleanup(t *testing.T) {
	started := make(chan struct{})
	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	engine := &fakeSessionEngine{run: func(ctx context.Context, _ string, _ agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		close(started)
		<-ctx.Done()
		close(cleanupStarted)
		<-releaseCleanup
		return agentengine.TurnResult{
			Status: agentengine.TurnCanceled,
			Error:  &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: ctx.Err().Error()},
		}
	}}
	handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	router := handler.Routes()

	responseDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		responseDone <- performAgentSessionRequest(t, handler, "agent-alpha", "explicit-cancel", map[string]any{"input": "wait"})
	}()
	<-started

	cancelRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/agents/agent-alpha/sessions/explicit-cancel/responses/cancel",
		nil,
	)
	cancelRecorder := httptest.NewRecorder()
	cancelDone := make(chan struct{})
	go func() {
		router.ServeHTTP(cancelRecorder, cancelRequest)
		close(cancelDone)
	}()
	<-cleanupStarted
	select {
	case <-cancelDone:
		t.Fatalf("cancel returned before cleanup: status=%d", cancelRecorder.Code)
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseCleanup)
	select {
	case <-cancelDone:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel endpoint did not return after runtime cleanup")
	}
	if cancelRecorder.Code != http.StatusNoContent {
		t.Fatalf("cancel status=%d body=%s", cancelRecorder.Code, cancelRecorder.Body.String())
	}
	select {
	case <-responseDone:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled response did not return")
	}
}

func TestAgentSessionCancelBeforeEngineRegistrationPreventsDispatch(t *testing.T) {
	beforeRun := make(chan struct{})
	releaseRun := make(chan struct{})
	engine := &fakeSessionEngine{}
	engine.beforeRun = func(context.Context) {
		close(beforeRun)
		<-releaseRun
	}
	handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	router := handler.Routes()

	responseDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		responseDone <- performAgentSessionRequest(t, handler, "agent-alpha", "pre-registration-cancel", map[string]any{"input": "must not run"})
	}()
	<-beforeRun

	cancelRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/agents/agent-alpha/sessions/pre-registration-cancel/responses/cancel",
		nil,
	)
	cancelRecorder := httptest.NewRecorder()
	router.ServeHTTP(cancelRecorder, cancelRequest)
	if cancelRecorder.Code != http.StatusNoContent {
		t.Fatalf("cancel status=%d body=%s", cancelRecorder.Code, cancelRecorder.Body.String())
	}
	close(releaseRun)
	select {
	case <-responseDone:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled response did not return")
	}
	if calls := engine.client.Calls(); len(calls) != 0 {
		t.Fatalf("pre-registration cancellation dispatched Engine calls: %+v", calls)
	}
}

func TestAgentSessionResponsesStreamsExistingSSEShapeAndToolBlocks(t *testing.T) {
	longQuery := strings.Repeat("q", 300)
	engine := &fakeSessionEngine{run: func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
		events := []agentengine.TurnEvent{
			{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{
				ID: "call-1", Kind: "mcp_tool_call", InputSummary: `{"truncated":true}`,
				Payload: map[string]any{"server": "wiki", "tool": "search", "arguments": map[string]any{"query": longQuery, "api_key": "sk-secret"}},
			}},
			{Kind: agentengine.TurnEventToolCallUpdate, Tool: &agentengine.ToolActivity{ID: "call-1", OutputSummary: "/workspace"}},
			{Kind: agentengine.TurnEventTextDelta, Text: "hello"},
			{Kind: agentengine.TurnEventTextDelta, Text: " world"},
		}
		for _, event := range events {
			if err := sink.Emit(ctx, event); err != nil {
				return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: err.Error()}}
			}
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "hello world"}
	}}
	handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	recorder := performAgentSessionRequest(t, handler, "agent-alpha", "stream", map[string]any{"input": "hello", "stream": true})
	if recorder.Code != http.StatusOK || !strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status = %d, content-type = %q", recorder.Code, recorder.Header().Get("Content-Type"))
	}
	body := recorder.Body.String()
	for _, want := range []string{
		"event: message_start",
		`"content_block":{"id":"call-1","input":{},"name":"search","type":"tool_use"}`,
		`\"api_key\":\"[redacted]\"`,
		longQuery,
		`"content_block":{"content":"","tool_use_id":"call-1","type":"tool_result"}`,
		`"delta":{"content":"/workspace","type":"tool_result_delta"}`,
		`"delta":{"text":"hello","type":"text_delta"}`,
		`"delta":{"text":" world","type":"text_delta"}`,
		"event: message_stop",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("SSE body missing %q: %s", want, body)
		}
	}
	if strings.Count(body, "hello world") != 0 {
		t.Fatalf("completed text was duplicated in SSE: %s", body)
	}
}

func TestAgentSessionResponsesStreamsTerminalErrorShape(t *testing.T) {
	engine := &fakeSessionEngine{run: func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
		return agentengine.TurnResult{
			Status: agentengine.TurnFailed,
			Error:  &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: "runtime failed"},
		}
	}}
	handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	recorder := performAgentSessionRequest(t, handler, "agent-alpha", "stream-error", map[string]any{"input": "hello", "stream": true})
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "event: error") ||
		!strings.Contains(body, `"stop_reason":"error"`) || !strings.Contains(body, "event: message_stop") {
		t.Fatalf("response = %d %s", recorder.Code, body)
	}
}

func TestAgentSessionEventStreamEmitsAndStopsHeartbeat(t *testing.T) {
	recorder := httptest.NewRecorder()
	stream := newAgentSessionEventStream(recorder)
	stream.startHeartbeat(5 * time.Millisecond)
	time.Sleep(18 * time.Millisecond)
	stream.stopHeartbeat()
	body := recorder.Body.String()
	if count := strings.Count(body, ": heartbeat\n\n"); count < 2 {
		t.Fatalf("heartbeat count = %d, body = %q", count, body)
	}
	time.Sleep(10 * time.Millisecond)
	if recorder.Body.String() != body {
		t.Fatal("heartbeat continued after stop")
	}
}

func TestAgentSessionResponsesReturnsValidationAndTimeoutErrors(t *testing.T) {
	engine := &fakeSessionEngine{run: func(ctx context.Context, _ string, _ agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		<-ctx.Done()
		return agentengine.TurnResult{Status: agentengine.TurnCanceled, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: ctx.Err().Error()}}
	}}
	handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	invalid := performAgentSessionRequest(t, handler, "agent-alpha", "bad!session", map[string]any{"input": "hello"})
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "invalid_session_id") {
		t.Fatalf("invalid response = %d %s", invalid.Code, invalid.Body.String())
	}
	unknown := performAgentSessionRequest(t, handler, "agent-alpha", "valid", map[string]any{"input": "hello", "unknown": true})
	if unknown.Code != http.StatusBadRequest || !strings.Contains(unknown.Body.String(), "invalid_request") {
		t.Fatalf("unknown-field response = %d %s", unknown.Code, unknown.Body.String())
	}
	large := performAgentSessionRequest(t, handler, "agent-alpha", "large", map[string]any{"input": strings.Repeat("x", agentSessionResponseBodyLimit+1)})
	if large.Code != http.StatusBadRequest || !strings.Contains(large.Body.String(), "invalid_request") {
		t.Fatalf("large response = %d %s", large.Code, large.Body.String())
	}

	previousTimeout := agentSessionResponseTimeout
	agentSessionResponseTimeout = 10 * time.Millisecond
	defer func() { agentSessionResponseTimeout = previousTimeout }()
	timedOut := performAgentSessionRequest(t, handler, "agent-alpha", "timeout", map[string]any{"input": "hello"})
	if timedOut.Code != http.StatusGatewayTimeout || !strings.Contains(timedOut.Body.String(), "response_timeout") {
		t.Fatalf("timeout response = %d %s", timedOut.Code, timedOut.Body.String())
	}
}

func TestAgentSessionResponsesRejectsUnsupportedRuntimeBeforeBinding(t *testing.T) {
	var calls atomic.Int32
	engine := &fakeSessionEngine{run: func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
		calls.Add(1)
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded}
	}}
	unsupported := completeWorkerAgent("agent-alpha", "Alpha")
	handler, _, bindings, _ := newAgentSessionTestHandler(t, []agent.Agent{unsupported}, engine, "")
	recorder := performAgentSessionRequest(t, handler, "agent-alpha", "unsupported", map[string]any{"input": "hello"})
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "runtime_adapter_unavailable") {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
	if calls.Load() != 0 || len(bindings.Bindings()) != 0 {
		t.Fatalf("engine calls = %d, bindings = %#v", calls.Load(), bindings.Bindings())
	}
}

func TestAgentSessionBindingSurvivesHandlerRestart(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "session-bindings")
	var keys []agentengine.ConversationKey
	engine := &fakeSessionEngine{run: func(_ context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		keys = append(keys, request.ConversationKey)
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "ok"}
	}}
	agents := []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}
	first, _, _, _ := newAgentSessionTestHandler(t, agents, engine, statePath)
	if recorder := performAgentSessionRequest(t, first, "agent-alpha", "persistent", map[string]any{"input": "one"}); recorder.Code != http.StatusOK {
		t.Fatalf("first response = %d %s", recorder.Code, recorder.Body.String())
	}
	second, _, _, _ := newAgentSessionTestHandler(t, agents, engine, statePath)
	if recorder := performAgentSessionRequest(t, second, "agent-alpha", "persistent", map[string]any{"input": "two"}); recorder.Code != http.StatusOK {
		t.Fatalf("second response = %d %s", recorder.Code, recorder.Body.String())
	}
	if len(keys) != 2 || keys[0] == "" || keys[0] != keys[1] {
		t.Fatalf("conversation keys = %#v", keys)
	}
}

func TestParseAgentSessionResponseRequestHonorsStreamFlag(t *testing.T) {
	for _, test := range []struct {
		body string
		want bool
	}{
		{body: `{"input":"hello"}`, want: false},
		{body: `{"input":"hello","stream":false}`, want: false},
		{body: `{"input":"hello","stream":true}`, want: true},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
		_, got, err := parseAgentSessionResponseRequest(recorder, request)
		if err != nil || got != test.want {
			t.Fatalf("body %s: stream = %t, err = %v", test.body, got, err)
		}
	}
}

func sessionCodexAgent(id, name string) agent.Agent {
	item := completeWorkerAgent(id, name)
	item.RuntimeKind = agent.RuntimeKindCodex
	return item
}

func sessionTurnText(request agentengine.TurnRequest) string {
	text := make([]string, 0, len(request.Input))
	for _, part := range request.Input {
		if part.Kind == agentengine.InputPartText {
			text = append(text, part.Text)
		}
	}
	return strings.Join(text, "\n\n")
}

func newAgentSessionTestHandler(
	t *testing.T,
	agents []agent.Agent,
	engine *fakeSessionEngine,
	bindingPath string,
) (*Handler, *im.Service, *agentsession.Store, string) {
	t.Helper()
	root := t.TempDir()
	agentPath := filepath.Join(root, "agents.json")
	data, err := json.Marshal(map[string]any{"agents": agents})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentPath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	agentSvc, err := agent.NewController(
		config.ModelConfig{}, config.ServerConfig{}, "manager:test", agentPath,
		agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindPicoClawSandbox}),
		agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatal(err)
	}
	seeded, err := agentengine.New(agentSvc).Agents().List(context.Background(), agentengine.AgentListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine.client = enginetest.NewMemoryClient(seeded...)
	engine.client.SetTurnBehavior(func(ctx context.Context, agentID string, request agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
		if engine.run != nil {
			return engine.run(ctx, agentID, request, sink)
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "final answer", Dispatched: true}
	})
	if bindingPath == "" {
		bindingPath = filepath.Join(root, "session-bindings")
	}
	bindings, err := agentsession.NewStore(bindingPath)
	if err != nil {
		t.Fatal(err)
	}
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{{ID: im.AdminUserID, Name: "admin", Role: "admin"}}})
	handler := NewHandler(AgentServices{Records: agentSvc, Workspace: agentSvc.Workspace(), Models: agentSvc.Models(), Runtime: agentSvc}, agentengine.New(agentSvc), imSvc, nil, im.NewParticipantBridge(""), nil, nil)
	handler.SetAgentEngine(engine, bindings)
	return handler, imSvc, bindings, bindingPath
}

func performAgentSessionRequest(t *testing.T, handler *Handler, agentSelector, sessionID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/agents/" + agentSelector + "/sessions/" + sessionID + "/responses"
	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload)))
	return recorder
}

func (fakeSessionConversations) GetInteraction(context.Context, agentengine.ConversationKey, string) (agentengine.InteractionRequest, error) {
	return agentengine.InteractionRequest{}, &agentengine.TurnError{Code: agentengine.ErrorInteractionNotFound, Message: "no interaction in this test fixture"}
}
