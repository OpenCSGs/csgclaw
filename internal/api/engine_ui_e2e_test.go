package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/auth"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/delivery"
	channelinteraction "csgclaw/internal/channel/csgclaw/interaction"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	webui "csgclaw/web"
)

type uiFixtureRuntime struct {
	fakeCompatRuntime
	approved    chan struct{}
	once        sync.Once
	resolutions atomic.Int32
}

func (r *uiFixtureRuntime) Conversation(string) agentengine.RuntimeConversation { return r }
func (r *uiFixtureRuntime) Reset(context.Context, agentengine.ConversationKey) *agentengine.TurnError {
	return nil
}
func (r *uiFixtureRuntime) Resolve(_ context.Context, _ agentengine.InteractionRequest, resolution agentengine.InteractionResolution) *agentengine.TurnError {
	if resolution.OptionID != "allow" {
		return &agentengine.TurnError{Code: agentengine.ErrorInvalidRequest, Message: "Unexpected option"}
	}
	r.resolutions.Add(1)
	r.once.Do(func() { close(r.approved) })
	return nil
}
func (r *uiFixtureRuntime) Run(ctx context.Context, request agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
	if request.ID == "approval" {
		snapshot := activity.ActivitySnapshot{ID: "e2e-permission", Title: "允许执行验证命令？", Status: activity.ActionStatusPending, Options: []activity.ActionOptionSnapshot{{ID: "allow", Label: "允许一次", Kind: "allow_once"}, {ID: "deny", Label: "拒绝", Kind: "reject"}}}
		if err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventInteractionRequest, Interaction: &agentengine.InteractionRequest{ID: snapshot.ID, Kind: agentengine.InteractionPermission, Payload: snapshot}}); err != nil {
			return agentengine.TurnResult{Status: agentengine.TurnFailed}
		}
		select {
		case <-r.approved:
		case <-ctx.Done():
			return agentengine.TurnResult{Status: agentengine.TurnCanceled, Dispatched: true}
		}
		snapshot.Status = activity.ActionStatusAllowed
		_ = sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventActivityUpdate, Activity: &agentengine.ActivityUpdate{ID: snapshot.ID, Kind: string(activity.RuntimeEventActionDecision), Payload: snapshot}})
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true, Output: "权限确认已通过 Agent Engine。"}
	}
	args := activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{ID: "verification", Header: "验收", Question: "确认最终架构的交互回复可用？", Options: []activity.RequestUserInputOption{{Label: "验证通过", Description: "通过 Engine 回复并保存记录。"}}}}}
	if err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventOutputItem, Output: &agentengine.OutputItem{Kind: agentengine.OutputItemRequestUserInput, Payload: args}}); err != nil {
		return agentengine.TurnResult{Status: agentengine.TurnFailed}
	}
	return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true, Output: "Agent Engine 端到端验证。"}
}

// Opt-in browser fixture: real Engine/HTTP/Channel renderer and built Web UI,
// with a deterministic Runtime instead of spending model calls or using Bot secrets.
func TestFinalEngineBrowserFixture(t *testing.T) {
	readyFile := os.Getenv("CSGCLAW_E2E_READY_FILE")
	if readyFile == "" {
		t.Skip("set CSGCLAW_E2E_READY_FILE for the headless browser fixture")
	}
	t.Cleanup(stubAuthStatus(func(*http.Request) (auth.Status, error) { return auth.Status{}, nil }))
	rt := &uiFixtureRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: agent.RuntimeKindCodex}, approved: make(chan struct{})}
	controller := mustNewSeededServiceWithOptions(t, []agent.Agent{{ID: "agent-e2e", Name: "Engine 验证", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex, RuntimeID: "rt-agent-e2e", Status: "running", ProfileComplete: true}}, agent.WithRuntime(rt))
	engine := agentengine.New(controller)
	participants := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{ID: "pt-e2e", Channel: "csgclaw", Type: participant.TypeAgent, Name: "Engine 验证", AgentID: "agent-e2e", ChannelUserRef: "user-e2e", ChannelUserKind: participant.ChannelUserKindLocalUserID, Mentionable: true}}), participant.WithAgentEngine(engine))
	bus := im.NewBus()
	imService := im.NewServiceFromBootstrapWithBus(im.Bootstrap{CurrentUserID: "user-admin", Users: []im.User{{ID: "user-admin", Name: "验收用户", Role: "admin"}, {ID: "user-e2e", Name: "Engine 验证", Role: "worker"}}, Rooms: []im.Room{{ID: "room-final", Title: "Agent Engine 最终验收", Members: []string{"user-admin", "user-e2e"}}}}, bus)
	coordinator := channelinteraction.NewCoordinator(engine, participants)
	store, err := delivery.NewIMTranscriptStore(imService, participants, engine)
	if err != nil {
		t.Fatal(err)
	}
	renderer := delivery.NewTranscriptRenderer(store, delivery.WithInteractionProjector(coordinator))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := func(id string) agentengine.TurnResult {
		turn := channel.TurnContext{AgentID: "agent-e2e", BindingID: "binding-e2e", ParticipantID: "pt-e2e", RoomID: "room-final", ConversationKey: agentengine.ConversationKey(id), TurnID: agentengine.TurnID(id), Locale: "zh"}
		result := engine.Conversations(turn.AgentID).Run(ctx, agentengine.TurnRequest{ID: turn.TurnID, ConversationKey: turn.ConversationKey, Input: []agentengine.InputPart{{Kind: agentengine.InputPartText, Text: id}}, Interaction: agentengine.InteractionResolve}, agentengine.EventSinkFunc(func(ctx context.Context, event agentengine.TurnEvent) error { return renderer.Emit(ctx, turn, event) }))
		if err := renderer.Complete(ctx, turn, result); err != nil {
			t.Error(err)
		}
		return result
	}
	question := run("question")
	if len(question.Interactions) != 1 {
		t.Fatalf("question=%+v", question)
	}
	approvalDone := make(chan agentengine.TurnResult, 1)
	go func() { approvalDone <- run("approval") }()
	h := NewHandlerWithAuth(AgentServices{Records: controller, Workspace: controller.Workspace(), Models: controller.Models(), Runtime: controller}, engine, imService, bus, nil, nil, nil, "", true)
	h.SetParticipantService(participants)
	h.SetUserInputResponder(coordinator)
	h.SetActivityDecider(coordinator)
	router := h.Routes()
	finished := make(chan struct{})
	var finishOnce sync.Once
	router.Post("/__e2e/finish", func(w http.ResponseWriter, _ *http.Request) {
		finishOnce.Do(func() { close(finished) })
		w.WriteHeader(http.StatusNoContent)
	})
	router.Handle("/*", webui.Handler())
	server := httptest.NewServer(router)
	defer server.Close()
	data, _ := json.Marshal(map[string]string{"url": server.URL, "question_id": question.Interactions[0].ID})
	if err := os.WriteFile(readyFile, data, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(150 * time.Second):
		t.Fatal("browser verification did not finish")
	}
	select {
	case result := <-approvalDone:
		if result.Status != agentengine.TurnSucceeded {
			t.Fatalf("approval=%+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("permission was not resolved through Engine")
	}
	if rt.resolutions.Load() != 1 {
		t.Fatal("native Runtime resolution was not exactly once")
	}
	item, err := engine.Conversations("agent-e2e").GetInteraction(ctx, "question", question.Interactions[0].ID)
	if err != nil || item.Payload.(activity.UserInputSnapshot).Status != activity.UserInputStatusAnswered {
		t.Fatalf("question not answered through Engine: %+v %v", item, err)
	}
}
