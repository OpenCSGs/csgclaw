package source

import (
	"context"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/execution"
	"csgclaw/internal/conversationresource"
	"csgclaw/internal/im"
	"csgclaw/internal/knowledgebase"
)

type resourceEngine struct {
	agentengine.ConversationInterface
	calls   int
	request agentengine.TurnRequest
}

func (e *resourceEngine) Conversations(string) agentengine.ConversationInterface { return e }

func (e *resourceEngine) Run(_ context.Context, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
	e.calls++
	e.request = request
	return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
}

func TestBridgeResourceSelectionReachesEngine(t *testing.T) {
	for _, runtime := range []string{"codex", "dsh"} {
		for _, test := range []struct {
			name    string
			content string
			refs    []apitypes.ResourceReference
		}{
			{name: "plain", content: "Continue"},
			{name: "skill", content: "Review", refs: []apitypes.ResourceReference{{Source: "skill", ID: "review"}}},
			{name: "skill_only", refs: []apitypes.ResourceReference{{Source: "skill", ID: "review"}}},
			{name: "connector", content: "Search", refs: []apitypes.ResourceReference{{Source: "mcp", ID: "search"}}},
			{name: "knowledge_base", content: "?", refs: []apitypes.ResourceReference{{Source: "mcp", ID: "knowledge"}}},
		} {
			t.Run(runtime+"/"+test.name, func(t *testing.T) {
				agent := agentengine.Agent{ID: "agent", Spec: agentengine.AgentSpec{
					Runtime: agentengine.RuntimeSpec{Adapter: runtime},
					Skills:  []string{"review"},
					MCPServers: map[string]contract.MCPServerConfig{
						"search": {"url": "https://fixture.invalid/mcp"},
						"knowledge": {"_meta": map[string]any{
							knowledgebase.ManagedMetaNamespace: map[string]any{
								"type": knowledgebase.ManagedMCPType, "auth_type": knowledgebase.ManagedAuthType,
								"resource_id": "1", "content_id": "knowledge",
							},
						}},
					},
				}}
				resources := conversationresource.New(fakeAgents{items: map[string]agentengine.Agent{"agent": agent}}, nil)
				resolved, err := resources.Resolve(t.Context(), "agent", test.refs)
				if err != nil {
					t.Fatal(err)
				}
				var selection *apitypes.ResourceSelection
				if len(test.refs) > 0 {
					selection = resolved.Snapshot()
				}
				bridge := im.NewParticipantBridge("")
				events, cancel := bridge.Subscribe("u-bot")
				defer cancel()
				binding := channel.Binding{ID: "binding", ParticipantID: "u-bot", AgentID: "agent"}
				room := im.Room{ID: "room", IsDirect: true, Members: []string{"u-admin", "u-bot"}}
				message := im.Message{
					ID: "message", SenderID: "u-admin", Content: test.content,
					Metadata: apitypes.WithResourceSelection(nil, selection),
				}
				publish := func() channel.Event {
					t.Helper()
					bridge.PublishMessageEvent(room, im.User{ID: "u-admin", Name: "Admin"}, message)
					select {
					case event := <-events:
						return botEvent(binding, event)
					case <-time.After(time.Second):
						t.Fatal("bridge did not publish the message")
						return channel.Event{}
					}
				}
				engine := &resourceEngine{}
				adapter, err := execution.New(engine, nil, execution.WithResourceInputProvider(resources.PrepareInput))
				if err != nil {
					t.Fatal(err)
				}
				outcome, err := adapter.Handle(t.Context(), binding, publish())
				if err != nil || outcome.Result.Status != agentengine.TurnSucceeded || engine.calls != 1 {
					t.Fatalf("direct message failed: result=%+v err=%v calls=%d", outcome.Result, err, engine.calls)
				}
				prompt := engine.request.Input[len(engine.request.Input)-1].Text
				if selection == nil {
					if !strings.Contains(prompt, `"resources":[]`) {
						t.Fatal("ordinary message did not clear resource selection")
					}
					return
				}
				if !strings.Contains(prompt, `"id":"`+test.refs[0].ID+`"`) {
					t.Fatalf("selected resource missing from engine input: %q", prompt)
				}

				room.IsDirect, room.NotifyAllAgents = false, true
				message.ID = "group-message"
				outcome, err = adapter.Handle(t.Context(), binding, publish())
				if err != nil || outcome.Result.Error == nil || outcome.Result.Error.Code != agentengine.ErrorInvalidRequest || engine.calls != 1 {
					t.Fatalf("group resource selection reached the engine: result=%+v err=%v calls=%d", outcome.Result, err, engine.calls)
				}
			})
		}
	}
}
