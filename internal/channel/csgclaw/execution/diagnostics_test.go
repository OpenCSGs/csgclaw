package execution

import (
	"context"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
	"csgclaw/internal/diagnostics"
	"testing"
	"time"
)

type slowDiagnosticRenderer struct{ fakeRenderer }

func (r *slowDiagnosticRenderer) Complete(context.Context, channel.TurnContext, agentengine.TurnResult) error {
	time.Sleep(20 * time.Millisecond)
	return nil
}
func TestDiagnosticsCapturePreparationAndPostRuntimeDelivery(t *testing.T) {
	store := diagnostics.New("")
	store.Source("room", "source", time.Now().Add(-15*time.Millisecond))
	engine := &fakeEngine{run: func(ctx context.Context, _ agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		r := diagnostics.From(ctx)
		r.RuntimeInfo("fixture", "fixture-model")
		r.RuntimeStart()
		time.Sleep(10 * time.Millisecond)
		r.RuntimeEndAt(time.Now())
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "done"}
	}}
	adapter, err := New(engine, &slowDiagnosticRenderer{}, WithDiagnostics(store))
	if err != nil {
		t.Fatal(err)
	}
	binding := channel.Binding{ID: "binding", AgentID: "agent", ParticipantID: "participant"}
	event := channel.Event{MessageID: "source", RoomID: "room", Text: "hello"}
	if _, err := adapter.Run(context.Background(), binding, event); err != nil {
		t.Fatal(err)
	}
	list := store.List("room", "source", "", "", "")
	if len(list) != 1 {
		t.Fatalf("records=%+v", list)
	}
	v, _ := store.Get("room", list[0].ID)
	if v.RuntimeStartMS == nil || v.RuntimeEndMS == nil || *v.RuntimeStartMS < 15 || *v.RuntimeEndMS-*v.RuntimeStartMS < 10 || v.TotalMS-*v.RuntimeEndMS < 20 {
		t.Fatalf("wrong attribution: %+v", v)
	}
	if v.Status != "succeeded" || v.Incomplete {
		t.Fatalf("incomplete trace: %+v", v)
	}
}
