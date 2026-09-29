package api

import (
	"context"
	"csgclaw/internal/config"
	"csgclaw/internal/llm"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/auth"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/binding"
	"csgclaw/internal/channel/csgclaw/delivery"
	"csgclaw/internal/channel/csgclaw/execution"
	"csgclaw/internal/diagnostics"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	webui "csgclaw/web"
)

type diagnosticFixtureRuntime struct {
	uiFixtureRuntime
	gateway atomic.Value
}

func (r *diagnosticFixtureRuntime) Conversation(string) agentengine.RuntimeConversation { return r }
func (r *diagnosticFixtureRuntime) Run(ctx context.Context, req agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
	record := diagnostics.From(ctx)
	prepare := diagnostics.Measure(ctx, "session.prepare", "runtime")
	time.Sleep(40 * time.Millisecond)
	prepare()
	record.RuntimeInfo("codex", "fixture-model")
	record.RuntimeStart()
	session, native := "session-"+string(req.ID), "native-"+string(req.ID)
	record.RuntimeRef(session, native, "")
	nativeStart := time.Now()
	time.Sleep(10 * time.Millisecond)
	record.NativeSpan("native-fixture", "build_prompt", "prepare", "fixture-trace", "", nativeStart, time.Now())
	if err := r.requestModel(ctx, session, native); err != nil {
		return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: err.Error()}}
	}
	_ = sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "正在核对性能数据。"})
	_ = sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "fixture-tool", Kind: "command", Title: "Read demo file", Status: "running", Payload: map[string]any{"rawInput": map[string]any{"cmd": "cat demo.txt", "workdir": "/workspace", "api_key": "fixture-sensitive-argument"}}}})
	time.Sleep(100 * time.Millisecond)
	_ = sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallUpdate, Tool: &agentengine.ToolActivity{ID: "fixture-tool", Kind: "command", Status: "completed"}})
	if err := r.requestModel(ctx, session, native); err != nil {
		return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: err.Error()}}
	}
	record.RuntimeEndAt(time.Now())
	after := diagnostics.Measure(ctx, "result.process", "csgclaw")
	time.Sleep(80 * time.Millisecond)
	after()
	if len(req.Input) > 0 && strings.Contains(req.Input[len(req.Input)-1].Text, "fail") {
		return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: "Fixture upstream unavailable; Authorization: Bearer fixture-sensitive-token"}}
	}
	return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "已完成本次性能检查。", Dispatched: true}
}

func (r *diagnosticFixtureRuntime) requestModel(ctx context.Context, session, turn string) error {
	gateway, _ := r.gateway.Load().(string)
	req, err := http.NewRequestWithContext(ctx, "POST", gateway+"/api/v1/agents/agent-diag/llm/responses", strings.NewReader(`{"model":"fixture-model","input":"fixture","stream":true}`))
	if err != nil {
		return err
	}
	meta, _ := json.Marshal(map[string]string{"thread_id": session, "turn_id": turn})
	req.Header.Set("X-Codex-Turn-Metadata", string(meta))
	req.Header.Set("Authorization", "Bearer diagnostic-fixture-token")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("fixture model bridge returned %d", response.StatusCode)
	}
	_, err = io.Copy(io.Discard, response.Body)
	return err
}

// Uses real HTTP, IM, Channel scheduling, Engine and transcript delivery.
// Only the external Runtime is replaced by a deterministic delayed fixture.
func TestDiagnosticsBrowserFixture(t *testing.T) {
	ready := os.Getenv("CSGCLAW_DIAGNOSTICS_E2E_READY")
	if ready == "" {
		t.Skip("set CSGCLAW_DIAGNOSTICS_E2E_READY")
	}
	t.Cleanup(stubAuthStatus(func(*http.Request) (auth.Status, error) { return auth.Status{}, nil }))
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(30 * time.Millisecond)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"fixture\"}}\n\n")
		w.(http.Flusher).Flush()
		time.Sleep(120 * time.Millisecond)
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"fixture\"}}\n\n")
	}))
	defer model.Close()
	rt := &diagnosticFixtureRuntime{uiFixtureRuntime: uiFixtureRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: agent.RuntimeKindCodex}}}
	controller := mustNewSeededServiceWithOptions(t, []agent.Agent{{ID: "agent-diag", Name: "性能验证 Agent", Role: agent.RoleWorker, AgentProfile: agent.AgentProfile{Provider: agent.ProviderAPI, BaseURL: model.URL + "/v1", APIKey: "fixture-key", ModelID: "fixture-model", ProfileComplete: true}, RuntimeKind: agent.RuntimeKindCodex, RuntimeID: "rt-diag", Status: "running", ProfileComplete: true}}, agent.WithRuntime(rt))
	engine := agentengine.New(controller)
	participants := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{ID: "pt-diag", Channel: "csgclaw", Type: participant.TypeAgent, Name: "性能验证 Agent", AgentID: "agent-diag", ChannelUserRef: "user-diag", ChannelUserKind: participant.ChannelUserKindLocalUserID, Mentionable: true}}), participant.WithAgentEngine(engine))
	bus := im.NewBus()
	bridge := im.NewParticipantBridge("")
	svc := im.NewServiceFromBootstrapWithBus(im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{{ID: im.AdminUserID, Name: "验收用户", Role: "admin"}, {ID: "user-diag", Name: "性能验证 Agent", Role: "worker"}, {ID: "user-other", Name: "其他成员", Role: "user"}}, Rooms: []im.Room{{ID: "room-diag", Title: "Turn 性能诊断验收", Members: []string{im.AdminUserID, "user-diag", "user-other"}, NotifyAllAgents: true}}}, bus)
	store, err := delivery.NewIMTranscriptStore(svc, participants, engine)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := execution.New(engine, delivery.NewTranscriptRenderer(store), execution.WithDiagnostics(svc.Diagnostics()))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := binding.NewManager(adapter)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := manager.Ensure(channel.Binding{ID: "pt-diag", ParticipantID: "pt-diag", AgentID: "agent-diag", Channel: "csgclaw"}); err != nil {
		t.Fatal(err)
	}
	events, unsubscribe := bus.Subscribe()
	defer unsubscribe()
	go func() {
		for event := range events {
			if event.Type != im.EventTypeMessageCreated || event.Message == nil || event.Message.SenderID != im.AdminUserID {
				continue
			}
			thread := ""
			if event.Message.RelatesTo != nil {
				thread = event.Message.RelatesTo.EventID
			}
			if err := manager.Submit(channel.Binding{ID: "pt-diag", ParticipantID: "pt-diag", AgentID: "agent-diag", Channel: "csgclaw"}, channel.Event{RoomID: event.RoomID, MessageID: event.Message.ID, Text: event.Message.Content, Locale: "zh", ThreadRootID: thread}); err != nil {
				t.Error(err)
			}
		}
	}()
	h := NewHandlerWithAuth(AgentServices{Records: controller, Workspace: controller.Workspace(), Models: controller.Models(), Runtime: controller}, engine, svc, bus, bridge, nil, llm.NewService(config.ModelConfig{}, controller), "diagnostic-fixture-token", true)
	h.SetParticipantService(participants)
	router := h.Routes()
	finished := make(chan struct{})
	var once sync.Once
	router.Post("/__e2e/finish", func(w http.ResponseWriter, _ *http.Request) { once.Do(func() { close(finished) }); w.WriteHeader(204) })
	router.Handle("/*", webui.Handler())
	server := httptest.NewServer(router)
	rt.gateway.Store(server.URL)
	defer server.Close()
	data, _ := json.Marshal(map[string]string{"url": server.URL})
	if err := os.WriteFile(ready, data, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(5 * time.Minute):
		t.Fatal("browser verification did not finish")
	}
	records := svc.Diagnostics().List("room-diag", "", "", "", "")
	if len(records) < 2 {
		t.Fatalf("expected multiple traced submissions: %+v", records)
	}
	for _, v := range records {
		full, _ := svc.Diagnostics().Get(v.RoomID, v.ID)
		calls := 0
		for _, span := range full.Spans {
			if span.Owner == "llm" {
				calls++
			}
		}
		if calls != 2 {
			t.Errorf("model bridge request count=%d", calls)
		}
		if v.RuntimeStartMS == nil || v.RuntimeEndMS == nil || v.TotalMS-*v.RuntimeEndMS < 70 {
			t.Fatalf("incorrect runtime boundary: %+v", v)
		}
	}
}
