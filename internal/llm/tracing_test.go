package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/config"
	"csgclaw/internal/diagnostics"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestGatewayTraceWithoutIngressSurvivesRetries(t *testing.T) {
	spans := tracetest.NewSpanRecorder()
	service := NewService(config.ModelConfig{}, nil)
	service.setGatewayTracer(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans)))
	outbound := make(chan trace.SpanContext, 2)
	var calls atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sc := trace.SpanContextFromContext(propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header)))
		outbound <- sc
		w.Header().Set("X-Request-ID", sc.TraceID().String())
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		io.WriteString(w, `{"status":"completed"}`)
	}))
	defer gateway.Close()
	record := diagnostics.New("").Begin("room", "source", "", "agent", "turn")
	record.Running()
	ctx := WithTaskProfile(diagnostics.WithRecord(t.Context(), record), openCSGTestProfile(gateway.URL))
	resp, err := service.Responses(ctx, "agent", []byte(`{"input":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK || calls.Load() != 2 {
		t.Fatalf("unexpected model result: %d calls, status %d, read %v", calls.Load(), resp.StatusCode, err)
	}
	first, second := <-outbound, <-outbound
	if !first.IsValid() || first.TraceID() != second.TraceID() || first.SpanID() == second.SpanID() || resp.Header.Get("X-Request-ID") != first.TraceID().String() {
		t.Fatalf("retry lost its common trace: %v, %v", first, second)
	}
	ended := spans.Ended()
	if len(ended) != 3 || ended[2].Name() != "OpenCSG model request" || ended[2].SpanContext().TraceID() != first.TraceID() {
		t.Fatalf("expected a completed request span after the two client spans: %v", ended)
	}
	for _, span := range ended[:2] {
		if span.Parent().SpanID() != ended[2].SpanContext().SpanID() {
			t.Fatal("client attempt did not attach to the request span")
		}
	}
	assertTraceTurnID(t, ended[2], "turn")
	if calls := modelSpans(record); len(calls) != 2 || calls[0].Details == nil || calls[1].Details == nil || calls[0].Details.TraceID != first.TraceID().String() || calls[1].Details.TraceID != first.TraceID().String() {
		t.Fatalf("local diagnostics lost trace ID: %v", calls)
	}
	next, err := service.Responses(ctx, "agent", []byte(`{"input":"another model call in the same turn"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(io.Discard, next.Body)
	next.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	later := <-outbound
	if later.TraceID() == first.TraceID() {
		t.Fatal("expected another model request without ingress to have its own trace")
	}
	ended = spans.Ended()
	assertTraceTurnID(t, ended[len(ended)-1], "turn")
}

func assertTraceTurnID(t *testing.T, span sdktrace.ReadOnlySpan, want string) {
	t.Helper()
	for _, a := range span.Attributes() {
		if string(a.Key) == "csgclaw.turn_id" && a.Value.AsString() == want {
			return
		}
	}
	t.Errorf("span %s did not export turn %q", span.Name(), want)
}

func TestGatewayTracingExportsThroughOTLPHTTP(t *testing.T) {
	payloads := make(chan *collectortrace.ExportTraceServiceRequest, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			t.Errorf("OTLP export used %s", r.URL.Path)
		}
		data, err := io.ReadAll(r.Body)
		var payload collectortrace.ExportTraceServiceRequest
		if err != nil {
			t.Error(err)
		} else if err := proto.Unmarshal(data, &payload); err != nil {
			t.Error(err)
		} else {
			payloads <- &payload
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer collector.Close()
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", collector.URL+"/v1/traces")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_COMPRESSION", "none")
	t.Setenv("OTEL_SERVICE_NAME", "csgclaw-fixture")
	service := NewService(config.ModelConfig{}, nil)
	if err := service.ConfigureGatewayTracing(t.Context()); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	service.TraceModelBridge(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(httptest.NewRecorder(), request)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := service.ShutdownGatewayTracing(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case payload := <-payloads:
		if len(payload.ResourceSpans) != 1 || len(payload.ResourceSpans[0].ScopeSpans) != 1 || len(payload.ResourceSpans[0].ScopeSpans[0].Spans) != 1 {
			t.Fatalf("unexpected OTLP payload: %v", payload)
		}
		span := payload.ResourceSpans[0].ScopeSpans[0].Spans[0]
		if span.Name != "model bridge" || len(span.TraceId) != 16 || len(span.SpanId) != 8 || len(span.ParentSpanId) != 0 {
			t.Fatalf("bridge did not export a root server span: %v", span)
		}
	case <-ctx.Done():
		t.Fatal("collector did not receive a trace")
	}
}

func TestGatewayTraceContextAcrossBothHTTPServices(t *testing.T) {
	bridgeSpans, gatewaySpans := tracetest.NewSpanRecorder(), tracetest.NewSpanRecorder()
	bridgeTracer := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(bridgeSpans))
	gatewayTracer := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(gatewaySpans))
	service := NewService(config.ModelConfig{}, nil)
	service.setGatewayTracer(bridgeTracer)
	var calls atomic.Int32
	upstream := httptest.NewServer(otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Request-ID") != "" || r.Header.Get("Tracestate") != "qa=burst" {
			t.Errorf("unexpected outbound trace headers: ID=%q state=%q", r.Header.Get("X-Request-ID"), r.Header.Get("Tracestate"))
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, "busy")
			return
		}
		io.WriteString(w, `{"status":"completed"}`)
	}), "StarHub", otelhttp.WithTracerProvider(gatewayTracer), otelhttp.WithPropagators(propagation.TraceContext{})))
	defer upstream.Close()
	record := diagnostics.New("").Begin("room", "source", "", "agent", "csgclaw-turn")
	record.Running()
	bridge := httptest.NewServer(service.TraceModelBridge(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := diagnostics.WithRecord(r.Context(), record)
		response, err := service.Responses(WithTaskProfile(ctx, openCSGTestProfile(upstream.URL)), "agent", []byte(`{"input":"test"}`))
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		w.WriteHeader(response.StatusCode)
		io.Copy(w, response.Body)
	})))
	defer bridge.Close()
	const incoming = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	request, err := http.NewRequest(http.MethodPost, bridge.URL, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Traceparent", incoming)
	request.Header.Set("Tracestate", "qa=burst")
	response, err := bridge.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	bridge.Close()
	upstream.Close()
	if response.StatusCode != http.StatusOK || calls.Load() != 2 {
		t.Fatalf("received status %d after %d attempts", response.StatusCode, calls.Load())
	}
	parent := trace.SpanContextFromContext(propagation.TraceContext{}.Extract(t.Context(), propagation.HeaderCarrier(request.Header)))
	var bridgeID trace.SpanID
	clientIDs := map[trace.SpanID]trace.SpanID{}
	for _, span := range bridgeSpans.Ended() {
		if span.SpanContext().TraceID() != parent.TraceID() || span.SpanContext().TraceState().String() != "qa=burst" {
			t.Errorf("bridge did not inherit trace context: %v", span.SpanContext())
		}
		switch span.SpanKind() {
		case trace.SpanKindServer:
			assertTraceTurnID(t, span, "csgclaw-turn")
			bridgeID = span.SpanContext().SpanID()
			if span.Parent().SpanID() != parent.SpanID() {
				t.Error("bridge server span did not attach to the incoming parent")
			}
		case trace.SpanKindClient:
			clientIDs[span.SpanContext().SpanID()] = span.Parent().SpanID()
		}
	}
	if !bridgeID.IsValid() || len(clientIDs) != 2 || len(gatewaySpans.Ended()) != 2 {
		t.Fatalf("unexpected spans: bridge %v, client %v, Gateway %d", bridgeID, clientIDs, len(gatewaySpans.Ended()))
	}
	for _, span := range gatewaySpans.Ended() {
		clientID := span.Parent().SpanID()
		if span.SpanContext().TraceID() != parent.TraceID() || clientIDs[clientID] != bridgeID {
			t.Fatalf("Gateway span did not attach to the correct bridge client attempt: %v", span.Parent())
		}
		delete(clientIDs, clientID)
	}
}
