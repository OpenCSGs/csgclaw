package codex

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"csgclaw/internal/diagnostics"
)

// Native exports arrive asynchronously, often after turn/completed. Correlation
// uses the process-owned native turn ID and trace ID, never the latest turn.
type nativeTelemetry struct {
	mu         sync.Mutex
	server     *http.Server
	endpoint   string
	turns      map[string]nativeDiagnostic
	traces     map[string]*nativeTrace
	initialize initializeTiming
}

// Each collector serves one Codex process. Its request span can be exported
// before or after the client records the successful handshake.
type initializeTiming struct {
	start, reply, notified time.Time
	dispatch, spanEnd      time.Time
	logger                 *slog.Logger
	logged                 bool
}

func (n *nativeTelemetry) recordHandshake(logger *slog.Logger, start, reply, notified time.Time) {
	if n == nil || logger == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	t := &n.initialize
	t.logger, t.start, t.reply, t.notified = logger, start, reply, notified
	t.logIfComplete()
}

func (t *initializeTiming) logIfComplete() {
	if t.logged || t.logger == nil || t.dispatch.Before(t.start) || t.dispatch.After(t.reply) {
		return
	}
	t.logged = true
	t.logger.Info("Codex initialize timing",
		"request_to_dispatch_us", t.dispatch.Sub(t.start).Microseconds(),
		"dispatch_to_reply_us", t.reply.Sub(t.dispatch).Microseconds(),
		"reply_to_initialized_us", t.notified.Sub(t.reply).Microseconds(),
		"native_request_span_us", t.spanEnd.Sub(t.dispatch).Microseconds())
}

type nativeDiagnostic struct {
	active bool
	record *diagnostics.Record
	at     time.Time
}
type nativeTrace struct {
	turn     string
	conflict bool
	at       time.Time
	spans    map[string]nativeSpan
}
type nativeSpan struct {
	id, parent, name, category string
	start, end                 time.Time
}
type otlpAttribute struct {
	Key   string `json:"key"`
	Value struct {
		String string `json:"stringValue"`
	} `json:"value"`
}
type otlpSpan struct {
	TraceID    string          `json:"traceId"`
	SpanID     string          `json:"spanId"`
	ParentID   string          `json:"parentSpanId"`
	Name       string          `json:"name"`
	Start      string          `json:"startTimeUnixNano"`
	End        string          `json:"endTimeUnixNano"`
	Attributes []otlpAttribute `json:"attributes"`
}

// Deliberately collect meaningful phase boundaries, not prompts, outputs, or
// arbitrary native attributes. Nested phases remain inclusive observations.
var nativePhases = map[string]string{
	"turn/start": "dispatch", "app_server.serialized_request_queue": "dispatch",
	"turn_context.build": "prepare", "regular_task.prepare_run_turn": "prepare",
	"record_context_updates_and_set_reference_context_item": "prepare",
	"build_skills_and_plugins":                              "prepare", "build_prompt": "prepare", "build_tool_router": "prepare",
	"run_turn.prepare_sampling_request_input": "prepare", "run_pre_sampling_compact": "prepare",
	"world_state.build": "prepare", "environments.wait_until_ready": "prepare",
	"exec_server.environment.wait_until_ready": "prepare", "mcp.runtime.resolve_for_step": "prepare",
	"run_hooks_and_record_inputs": "prepare", "run_turn_stop_hooks": "prepare",
	"model_client.stream_responses_api": "request", "responses.stream_request": "request",
	"receiving_stream": "stream", "drain_in_flight": "tools",
	"persist_rollout_items": "persist", "session.flush_rollout": "persist",
	"session_task.turn": "turn",
}

func newNativeTelemetry() (*nativeTelemetry, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := make([]byte, 24)
	if _, err = rand.Read(token); err != nil {
		listener.Close()
		return nil, err
	}
	path := "/" + hex.EncodeToString(token) + "/v1/traces"
	n := &nativeTelemetry{endpoint: "http://" + listener.Addr().String() + path, turns: map[string]nativeDiagnostic{}, traces: map[string]*nativeTrace{}}
	mux := http.NewServeMux()
	mux.HandleFunc(path, n.receive)
	n.server = &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, MaxHeaderBytes: 8192}
	go func() { _ = n.server.Serve(listener) }()
	return n, nil
}
func (n *nativeTelemetry) close() {
	if n != nil {
		_ = n.server.Close()
	}
}
func (n *nativeTelemetry) overrides() []string {
	if n == nil {
		return nil
	}
	return []string{"--config", `otel.trace_exporter={otlp-http={endpoint=` + strconv.Quote(n.endpoint) + `,protocol="json"}}`, "--config", "otel.log_user_prompt=false"}
}
func (n *nativeTelemetry) bind(turn string, record *diagnostics.Record) {
	if n == nil || turn == "" || record == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.prune()
	n.turns[turn] = nativeDiagnostic{active: true, record: record, at: time.Now()}
	for traceID, trace := range n.traces {
		n.deliver(traceID, trace)
	}
}
func (n *nativeTelemetry) prune() {
	for id, t := range n.traces {
		if time.Since(t.at) > 10*time.Minute && !n.turns[t.turn].active {
			delete(n.traces, id)
		}
	}
	for id, t := range n.turns {
		if !t.active && time.Since(t.at) > 10*time.Minute {
			delete(n.turns, id)
		}
	}
	// Keep a bounded window even when a runtime handles many concurrent rooms.
	for len(n.traces) >= 128 {
		var id string
		var oldest time.Time
		for k, v := range n.traces {
			if id == "" || v.at.Before(oldest) {
				id, oldest = k, v.at
			}
		}
		delete(n.traces, id)
	}
	for len(n.turns) >= 128 {
		var id string
		var oldest time.Time
		for k, v := range n.turns {
			if id == "" || v.at.Before(oldest) {
				id, oldest = k, v.at
			}
		}
		delete(n.turns, id)
	}
}
func (n *nativeTelemetry) receive(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || r.Header.Get("Origin") != "" {
		http.Error(w, "not allowed", http.StatusForbidden)
		return
	}
	var body struct {
		Resources []struct {
			Scopes []struct {
				Spans []otlpSpan `json:"spans"`
			} `json:"scopeSpans"`
		} `json:"resourceSpans"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&body) != nil {
		http.Error(w, "invalid telemetry", http.StatusBadRequest)
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.prune()
	for _, res := range body.Resources {
		for _, scope := range res.Scopes {
			for _, s := range scope.Spans {
				if !validTelemetryID(s.TraceID, 32) || !validTelemetryID(s.SpanID, 16) {
					continue
				}
				if s.Name == "initialize" {
					start, end, valid := s.times()
					if valid {
						n.initialize.dispatch, n.initialize.spanEnd = start, end
						n.initialize.logIfComplete()
					}
					continue
				}
				trace := n.traces[s.TraceID]
				if trace == nil {
					if len(n.traces) >= 128 {
						continue
					}
					trace = &nativeTrace{at: time.Now(), spans: map[string]nativeSpan{}}
					n.traces[s.TraceID] = trace
				}
				for _, a := range s.Attributes {
					if (a.Key == "turn.id" || a.Key == "turn_id" || a.Key == "submission.id") && a.Value.String != "" && len(a.Value.String) <= 128 {
						if trace.turn != "" && trace.turn != a.Value.String {
							trace.conflict = true
						}
						trace.turn = a.Value.String
					}
				}
				category, ok := nativePhases[s.Name]
				if !ok || len(trace.spans) >= 512 {
					continue
				}
				start, end, valid := s.times()
				if valid {
					trace.spans[s.SpanID] = nativeSpan{s.SpanID, s.ParentID, s.Name, category, start, end}
				}
			}
		}
	}
	for id, trace := range n.traces {
		n.deliver(id, trace)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{}`))
}
func (s otlpSpan) times() (time.Time, time.Time, bool) {
	start, e1 := strconv.ParseInt(s.Start, 10, 64)
	end, e2 := strconv.ParseInt(s.End, 10, 64)
	return time.Unix(0, start), time.Unix(0, end), e1 == nil && e2 == nil && start > 0 && end >= start
}

func validTelemetryID(value string, size int) bool {
	if len(value) != size {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func (n *nativeTelemetry) deliver(id string, t *nativeTrace) {
	d, ok := n.turns[t.turn]
	if !ok || t.conflict {
		return
	}
	for key, s := range t.spans {
		d.record.NativeSpan(fmt.Sprintf("native:%s:%s", id, s.id), s.name, s.category, id, s.parent, s.start, s.end)
		delete(t.spans, key)
	}
}

func (n *nativeTelemetry) finish(record *diagnostics.Record) {
	if n == nil || record == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	for id, d := range n.turns {
		if d.record == record {
			d.active = false
			d.at = time.Now()
			n.turns[id] = d
		}
	}
}
