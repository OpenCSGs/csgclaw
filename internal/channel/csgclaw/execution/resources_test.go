package execution

import (
	"context"
	"strings"
	"testing"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	"csgclaw/internal/conversationresource"
)

type resourceAgentReader struct {
	disabled bool
	runtime  string
}

func (r *resourceAgentReader) Get(context.Context, string, contract.AgentGetOptions) (contract.Agent, error) {
	item := contract.Agent{ID: "agent", Spec: contract.AgentSpec{Runtime: contract.RuntimeSpec{Adapter: r.runtime}, Skills: []string{"review"}}}
	if r.disabled {
		item.Spec.Skills = nil
	}
	return item, nil
}

func TestResourceInputUsesExistingTurnAndRevalidates(t *testing.T) {
	for _, runtime := range []string{"codex", "dsh"} {
		t.Run(runtime, func(t *testing.T) {
			reader := &resourceAgentReader{runtime: runtime}
			resources := conversationresource.New(reader, nil)
			engine := &fakeEngine{}
			renderer := &fakeRenderer{}
			adapter, err := New(engine, renderer, WithResourceInputProvider(resources.PrepareInput))
			if err != nil {
				t.Fatal(err)
			}
			binding := channel.Binding{ID: "binding", ParticipantID: "participant", AgentID: "agent"}
			selection := &apitypes.ResourceSelection{Version: 1, AgentID: "agent", Refs: []apitypes.ResourceSnapshot{{ResourceReference: apitypes.ResourceReference{Source: "skill", ID: "review"}, Type: "skill", Name: "review"}}}
			event := channel.Event{MessageID: "selected", RoomID: "room", ChatType: "p2p", ResourceSelection: selection}
			if _, err = adapter.Handle(t.Context(), binding, event); err != nil {
				t.Fatal(err)
			}
			if len(engine.request.Input) == 0 || !strings.Contains(engine.request.Input[len(engine.request.Input)-1].Text, `"id":"review"`) {
				t.Fatalf("技能选择未进入 Run：%+v", engine.request)
			}
			event.MessageID, event.ResourceSelection, event.Text = "ordinary", nil, "下一条消息"
			reader.disabled = true
			event.ThreadRootID = "selected"
			event.ThreadContext = &channel.ThreadContext{RootMessageID: "selected", Context: []channel.ThreadContextMessage{{
				ID: "selected", ResourceSelection: selection,
			}}}

			if _, err = adapter.Handle(t.Context(), binding, event); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(engine.request.Input[len(engine.request.Input)-1].Text, `"resources":[]`) {
				t.Fatal("普通消息没有清空选择")
			}
			if !strings.Contains(engine.request.Input[0].Text, `"id":"review"`) || !strings.Contains(engine.request.Input[0].Text, "context only") {
				t.Fatal("historical resource context was lost")
			}
			engine.request.Input = nil
			event.MessageID, event.ResourceSelection, event.Text = "removed", selection, ""
			if _, err = adapter.Handle(t.Context(), binding, event); err != nil {
				t.Fatal(err)
			}
			last := renderer.complete[len(renderer.complete)-1]
			if len(engine.request.Input) > 0 || last.Error == nil || string(last.Error.Code) != conversationresource.ResourceNotAttached {
				t.Fatalf("失效资源没有通过既有失败结果返回：%+v", last)
			}
		})
	}
}

func TestGroupLegacyInputKeepsExistingHandling(t *testing.T) {
	engine := &fakeEngine{}
	called := false
	adapter, err := New(engine, &fakeRenderer{}, WithResourceInputProvider(func(context.Context, string, string, *apitypes.ResourceSelection) (string, string, error) {
		called = true
		return "", "", nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	event := channel.Event{MessageID: "message", RoomID: "room", ChatType: "group", Text: `<slash-command name="use-skill" arg="review"></slash-command> 任务`}
	_, err = adapter.Handle(t.Context(), channel.Binding{ID: "binding", ParticipantID: "participant", AgentID: "agent"}, event)
	if err != nil || called || len(engine.request.Input) == 0 || !strings.Contains(engine.request.Input[len(engine.request.Input)-1].Text, "slash-command") {
		t.Fatalf("群聊入口改变：%+v %v", engine.request, err)
	}
}
