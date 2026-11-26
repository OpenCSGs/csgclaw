package codex

import (
	"bytes"
	"csgclaw/internal/diagnostics"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
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

func TestInitializeTimingPairsNativeAndClientBoundaries(t *testing.T) {
	for _, order := range []string{"client-first", "span-first"} {
		t.Run(order, func(t *testing.T) {
			n, err := newNativeTelemetry()
			if err != nil {
				t.Fatal(err)
			}
			defer n.close()
			start := time.Unix(10, 0)
			lines := make(chan string, 2)
			logger := slog.New(slog.NewTextHandler(turnModelWriter(func(p []byte) (int, error) {
				lines <- string(p)
				return len(p), nil
			}), nil))
			done := make(chan struct{})
			defer close(done)
			record := func() {
				n.logInitializeAsync(logger, initializeTiming{start, start.Add(8100 * time.Microsecond), start.Add(8300 * time.Microsecond)}, done)
			}
			post := func(spanStart, spanEnd time.Duration) {
				t.Helper()
				span := otlpSpan{Name: "initialize", TraceID: strings.Repeat("a", 32), SpanID: strings.Repeat("b", 16),
					Start: fmt.Sprint(start.Add(spanStart).UnixNano()), End: fmt.Sprint(start.Add(spanEnd).UnixNano())}
				body, err := json.Marshal(map[string]any{"resourceSpans": []any{map[string]any{"scopeSpans": []any{map[string]any{"spans": []otlpSpan{span}}}}}})
				if err != nil {
					t.Fatal(err)
				}
				resp, err := http.Post(n.endpoint, "application/json", bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Fatal(resp.Status)
				}
			}
			if order == "client-first" {
				record()
			}
			post(time.Millisecond, 0) // Invalid native interval.
			select {
			case line := <-lines:
				t.Fatal(line)
			default:
			}
			post(8*time.Millisecond, 8200*time.Microsecond)
			if order == "span-first" {
				select {
				case line := <-lines:
					t.Fatal(line)
				default:
				}
				record()
			}
			post(8*time.Millisecond, 8200*time.Microsecond) // Export retry.
			select {
			case line := <-lines:
				if !strings.Contains(line, "request_to_dispatch_us=8000 dispatch_to_reply_us=100 reply_to_initialized_us=200 native_request_span_us=200") {
					t.Fatalf("unexpected initialize timing: %s", line)
				}
			case <-time.After(time.Second):
				t.Fatal("missing initialize timing")
			}
			select {
			case line := <-lines:
				t.Fatalf("duplicate initialize timing: %s", line)
			default:
			}
		})
	}
}

func TestInitializeTimingWithSlowLogs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var timing initializeTiming
		sink := newBlockingWriter()
		defer sink.unblock()
		logger := slog.New(slog.NewTextHandler(turnModelWriter(func(p []byte) (int, error) {
			if bytes.Contains(p, []byte("Codex initialize timing")) {
				return sink.Write(p)
			}
			time.Sleep(time.Second)
			return len(p), nil
		}), nil))
		var sent, reply time.Time
		client := newAppServerClient(nil, logger)
		client.stdin = turnModelWriter(func(p []byte) (int, error) {
			if bytes.Contains(p, []byte(`"method":"initialize"`)) {
				sent = time.Now()
				time.Sleep(time.Millisecond)
				reply = time.Now()
				client.handleLine(`{"id":1,"result":{}}`)
			}
			return len(p), nil
		})
		manager := &appServerManager{}
		if err := manager.initializeHandshake(t.Context(), &liveSession{appClient: client}, &timing); err != nil {
			t.Fatal(err)
		}
		if !timing.sent.Equal(sent) || !timing.reply.Equal(reply) {
			t.Fatalf("RPC log writes shifted the measured boundaries: got %v, want %v to %v", timing, sent, reply)
		}

		n := &nativeTelemetry{initialize: make(chan nativeSpan, 1)}
		n.initialize <- nativeSpan{start: sent, end: reply}
		returned := make(chan struct{})
		go func() {
			n.logInitializeAsync(logger, timing, t.Context().Done())
			close(returned)
		}()
		synctest.Wait()
		select {
		case <-sink.started:
		default:
			t.Fatal("timing log did not start")
		}
		select {
		case <-returned:
		default:
			t.Fatal("startup waited for the timing log writer")
		}
	})
}
