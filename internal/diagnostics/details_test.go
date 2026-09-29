package diagnostics

import (
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
