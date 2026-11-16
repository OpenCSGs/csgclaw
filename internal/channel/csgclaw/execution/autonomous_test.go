package execution

import (
	"context"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
	"csgclaw/internal/roomtask"
	"testing"
)

func TestAutonomousAdmissionSkipsRuntimeAndDelivery(t *testing.T) {
	engine := &fakeEngine{run: func(context.Context, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
		t.Fatal("skipped turn reached Runtime")
		return agentengine.TurnResult{}
	}}
	renderer := &fakeRenderer{}
	adapter, err := New(engine, renderer, WithRoomContextProvider(func(roomtask.TurnContextRequest) (roomtask.PrivateTurnContext, error) {
		return roomtask.PrivateTurnContext{Skip: true}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := adapter.Run(context.Background(), channel.Binding{AgentID: "agent-dev", ParticipantID: "pt-dev"}, channel.Event{RoomID: "room", MessageID: "source", Text: "Do work"})
	if err != nil || outcome.Result.Status != agentengine.TurnSucceeded || len(renderer.events)+len(renderer.complete) != 0 {
		t.Fatalf("skipped turn produced effects: %+v %v %+v", outcome, err, renderer)
	}
}
