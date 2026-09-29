package codex

import (
	"bytes"
	"csgclaw/internal/diagnostics"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestNativeTelemetryLateCorrelationAndIsolation(t *testing.T) {
	collector, err := newNativeTelemetry()
	if err != nil {
		t.Fatal(err)
	}
	defer collector.close()
	store := diagnostics.New("")
	r := store.Begin("room", "source", "", "agent", "engine-turn")
	r.Running()
	r.RuntimeStart()
	r.RuntimeEndAt(time.Now())
	r.Finish("succeeded")
	origin := r.Snapshot().StartedAt
	trace := "12345678901234567890123456789012"
	post := func(turn string) {
		t.Helper()
		body := map[string]any{"resourceSpans": []any{map[string]any{"scopeSpans": []any{map[string]any{"spans": []any{
			map[string]any{"traceId": trace, "spanId": "1234567890123456", "name": "build_prompt", "startTimeUnixNano": fmt.Sprint(origin.Add(time.Millisecond).UnixNano()), "endTimeUnixNano": fmt.Sprint(origin.Add(2 * time.Millisecond).UnixNano()), "attributes": []any{map[string]any{"key": "prompt", "value": map[string]any{"stringValue": "SECRET USER CONTENT"}}}},
			map[string]any{"traceId": trace, "spanId": "1234567890123457", "name": "turn/start", "startTimeUnixNano": fmt.Sprint(origin.UnixNano()), "endTimeUnixNano": fmt.Sprint(origin.Add(time.Millisecond).UnixNano()), "attributes": []any{map[string]any{"key": "turn.id", "value": map[string]any{"stringValue": turn}}}},
		}}}}}}
		raw, _ := json.Marshal(body)
		resp, err := http.Post(collector.endpoint, "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatal(resp.Status)
		}
	}
	before := len(r.Snapshot().Spans)
	post("native-turn")
	collector.bind("another-turn", store.Begin("room", "other-source", "", "agent", "other-engine-turn"))
	if len(r.Snapshot().Spans) != before {
		t.Fatal("unbound trace attributed to latest turn")
	}
	collector.bind("native-turn", r)
	snap := r.Snapshot()
	if len(snap.Spans) != before+2 || snap.Status != "succeeded" {
		t.Fatalf("late telemetry missing: %+v", snap)
	}
	post("native-turn")
	if len(r.Snapshot().Spans) != before+2 {
		t.Fatal("OTLP retry duplicated spans")
	}
	raw, _ := json.Marshal(r.Snapshot())
	if bytes.Contains(raw, []byte("SECRET")) {
		t.Fatal("unapproved telemetry attribute persisted")
	}
	req, _ := http.NewRequest("POST", collector.endpoint, bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Origin", "https://example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatal("browser-origin telemetry accepted")
	}
}
