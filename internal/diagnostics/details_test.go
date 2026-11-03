package diagnostics

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestToolDetailsWhitelistsAndRedactsInputs(t *testing.T) {
	details := ToolDetails("Run command", "", map[string]any{"rawInput": map[string]any{"cmd": "cat demo.txt", "workdir": "/workspace", "api_key": "private-key", "nested": map[string]any{"password": "private-password"}}, "output": "private-output"})
	data, _ := json.Marshal(details)
	text := string(data)
	for _, secret := range []string{"private-key", "private-password", "private-output"} {
		if strings.Contains(text, secret) {
			t.Fatalf("retained secret: %s", text)
		}
	}
	if details.Command != "cat demo.txt" || details.Directory != "/workspace" || !strings.Contains(details.Arguments, "[redacted]") {
		t.Fatalf("details=%+v", details)
	}
}

func TestToolDetailsPreservesNumericArgumentTokens(t *testing.T) {
	arguments := `{"decimal":1.000000000000000000001,"record_id":9007199254740993}`
	details := ToolDetails("Lookup", "", map[string]any{"arguments": arguments})
	if details.Arguments != arguments {
		t.Fatalf("tool argument summary changed numeric evidence: got %s, want %s", details.Arguments, arguments)
	}
}

func TestEventFloodReservesSpaceForModelAndToolTiming(t *testing.T) {
	store := New("")
	r := store.Begin("r", "s", "", "a", "t")
	r.Running()
	for i := 0; i < maxSpans*2; i++ {
		id := r.Start("event.deliver", "csgclaw", "")
		r.Annotate(id, SpanDetails{EventType: "text_delta"})
		r.End(id, "completed")
	}
	if id := r.Start("llm.request", "llm", ""); id == "" {
		t.Fatal("progress flood hid model requests")
	}
	if id := r.Start("tool.exec", "tool", "tool:1"); id == "" {
		t.Fatal("progress flood hid tools")
	}
	if !r.Snapshot().Incomplete {
		t.Fatal("truncation not visible")
	}
}

func TestNativeLookupRejectsCompletedTurn(t *testing.T) {
	store := New("")
	record := store.Begin("r", "s", "", "a", "t")
	record.Running()
	record.RuntimeRef("session", "native", "")
	if store.Native("a", "session", "native") != record {
		t.Fatal("active native identity not found")
	}
	record.Finish("canceled")
	if store.Native("a", "session", "native") != nil {
		t.Fatal("stale HTTP request attached to a completed turn")
	}
}

func TestRequestModelSurvivesLateNativeBinding(t *testing.T) {
	store := New("")
	record := store.Begin("room", "source", "", "agent", "turn")
	record.Running()
	record.RuntimeInfo("codex", "old-model")
	ctx := WithNativeLookup(WithRequestModel(context.Background(), "glm-5.3-flash"), store, "agent", "session", "native-turn")
	update, finish := ObserveRequest(ctx, "responses")
	update(400, nil)
	record.RuntimeRef("session", "native-turn", "")
	finish("failed")
	snapshot := record.Snapshot()
	if snapshot.Runtime != "codex" || snapshot.Model != "glm-5.3-flash" {
		t.Fatalf("wrong execution identity: %+v", snapshot)
	}
	if len(snapshot.Spans) == 0 || snapshot.Spans[len(snapshot.Spans)-1].Details.HTTPStatus != 400 {
		t.Fatalf("lost delayed request observation: %+v", snapshot.Spans)
	}
}
