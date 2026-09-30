package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/auth"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/binding"
	"csgclaw/internal/channel/csgclaw/delivery"
	"csgclaw/internal/channel/csgclaw/execution"
	"csgclaw/internal/config"
	"csgclaw/internal/diagnostics"
	"csgclaw/internal/im"
	"csgclaw/internal/modelcap"
	"csgclaw/internal/modelprovider"
	"csgclaw/internal/participant"
	webui "csgclaw/web"
)

type videoTimingRuntime struct{ uiFixtureRuntime }

func (r *videoTimingRuntime) Conversation(string) agentengine.RuntimeConversation { return r }
func (r *videoTimingRuntime) Run(ctx context.Context, req agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
	record := diagnostics.From(ctx)
	record.RuntimeInfo("codex", "fixture-model")
	record.RuntimeStart()
	time.Sleep(150 * time.Millisecond)
	_ = sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "video-call", Kind: "tool", Title: "generate video", Status: "running"}})
	if err := contract.GenerateVideo(ctx, "video-"+string(req.ID), "apple flowers", modelprovider.VideoGenerationOptions{}); err != nil {
		return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: err.Error()}}
	}
	_ = sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallUpdate, Tool: &agentengine.ToolActivity{ID: "video-call", Kind: "tool", Status: "completed"}})
	record.RuntimeEndAt(time.Now())
	return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "视频请求已提交。", Dispatched: true}
}

// Real HTTP submission, engine, async video provider polling, persisted message
// delivery, SSE, diagnostics API and embedded Web UI. Only external services
// are replaced, and the browser controls when the video provider completes.
func TestVideoTimingBrowserFixture(t *testing.T) {
	ready := os.Getenv("CSGCLAW_VIDEO_TIMING_E2E_READY")
	if ready == "" {
		t.Skip("set CSGCLAW_VIDEO_TIMING_E2E_READY")
	}
	t.Cleanup(stubAuthStatus(func(*http.Request) (auth.Status, error) { return auth.Status{}, nil }))
	var complete atomic.Bool
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/videos":
			json.NewEncoder(w).Encode(map[string]string{"id": "video-fixture", "status": "queued"})
		case "/v1/videos/video-fixture":
			state := "in_progress"
			if complete.Load() {
				state = "completed"
			}
			json.NewEncoder(w).Encode(map[string]string{"id": "video-fixture", "status": state})
		case "/v1/videos/video-fixture/content":
			time.Sleep(1500 * time.Millisecond)
			w.Header().Set("Content-Type", "video/mp4")
			w.Write([]byte("fixture-video-content"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer model.Close()
	rt := &videoTimingRuntime{uiFixtureRuntime: uiFixtureRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: agent.RuntimeKindCodex}}}
	controller := mustNewSeededServiceWithOptions(t, []agent.Agent{{ID: "agent-video", Name: "视频验证 Agent", Role: agent.RoleWorker, AgentProfile: agent.AgentProfile{Provider: agent.ProviderAPI, BaseURL: model.URL + "/v1", ModelID: "fixture-model", ProfileComplete: true, VideoGeneration: &modelprovider.VideoGenerationConfig{ProviderID: "fixture", ModelID: "video-model"}}, RuntimeKind: agent.RuntimeKindCodex, RuntimeID: "rt-video", Status: "running", ProfileComplete: true}}, agent.WithRuntime(rt))
	controller.SetLLMConfig(config.LLMConfig{
		Providers: map[string]config.ProviderConfig{
			"fixture": {
				BaseURL:     model.URL + "/v1",
				VideoModels: []string{"video-model"},
				VideoMetadata: map[string]modelcap.VideoGeneration{
					"video-model": {Sizes: []string{"768P"}, Seconds: []int{6}},
				},
			},
		},
	})
	engine := agentengine.New(controller)
	participants := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{ID: "pt-video", Channel: "csgclaw", Type: participant.TypeAgent, Name: "视频验证 Agent", AgentID: "agent-video", ChannelUserRef: "user-video", ChannelUserKind: participant.ChannelUserKindLocalUserID, Mentionable: true}}), participant.WithAgentEngine(engine))
	bus := im.NewBus()
	bootstrap := im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{{ID: im.AdminUserID, Name: "验收用户", Role: "admin"}, {ID: "user-video", Name: "视频验证 Agent", Role: "worker"}}, Rooms: []im.Room{{ID: "room-video", Title: "视频计时验收", Members: []string{im.AdminUserID, "user-video"}, NotifyAllAgents: true}}}
	statePath := filepath.Join(t.TempDir(), "state.json")
	state, err := json.Marshal(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(statePath, state, 0600); err != nil {
		t.Fatal(err)
	}
	svc, err := im.NewServiceFromPathWithBus(statePath, bus)
	if err != nil {
		t.Fatal(err)
	}
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
	bind := channel.Binding{ID: "pt-video", ParticipantID: "pt-video", AgentID: "agent-video", Channel: "csgclaw"}
	if err = manager.Ensure(bind); err != nil {
		t.Fatal(err)
	}
	events, unsubscribe := bus.Subscribe()
	defer unsubscribe()
	go func() {
		for event := range events {
			if event.Type == im.EventTypeMessageCreated && event.Message != nil && event.Message.SenderID == im.AdminUserID {
				if err := manager.Submit(bind, channel.Event{RoomID: event.RoomID, MessageID: event.Message.ID, Text: event.Message.Content, Locale: "zh"}); err != nil {
					t.Error(err)
				}
			}
		}
	}()
	h := NewHandlerWithAuth(AgentServices{Records: controller, Workspace: controller.Workspace(), Models: controller.Models(), Runtime: controller}, engine, svc, bus, im.NewParticipantBridge(""), nil, nil, "fixture-token", true)
	h.SetParticipantService(participants)
	router := h.Routes()
	finished := make(chan struct{})
	var once sync.Once
	router.Post("/__e2e/video-complete", func(w http.ResponseWriter, _ *http.Request) { complete.Store(true); w.WriteHeader(204) })
	router.Post("/__e2e/finish", func(w http.ResponseWriter, _ *http.Request) { once.Do(func() { close(finished) }); w.WriteHeader(204) })
	router.Handle("/*", webui.Handler())
	server := httptest.NewServer(router)
	defer server.Close()
	raw, _ := json.Marshal(map[string]string{"url": server.URL})
	if err = os.WriteFile(ready, raw, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(5 * time.Minute):
		t.Fatal("browser did not finish")
	}
}
