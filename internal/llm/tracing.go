package llm

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func (s *Service) setGatewayTracer(provider *sdktrace.TracerProvider) {
	s.gatewayTracer = provider
	s.gatewayClient = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport,
		otelhttp.WithTracerProvider(provider), otelhttp.WithPropagators(propagation.TraceContext{}))}
}

func (s *Service) TraceModelBridge(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "model bridge",
		otelhttp.WithTracerProvider(s.gatewayTracer),
		otelhttp.WithPropagators(propagation.TraceContext{}),
		otelhttp.WithSpanNameFormatter(func(operation string, _ *http.Request) string { return operation }),
	)
}

// ConfigureGatewayTracing enables optional OTLP/HTTP export before serving.
// The local SDK also creates client span contexts when export is disabled.
func (s *Service) ConfigureGatewayTracing(ctx context.Context) error {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("OTEL_TRACES_EXPORTER")), "none") ||
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		return nil
	}
	res, err := resource.Merge(resource.NewSchemaless(attribute.String("service.name", "csgclaw")), resource.Environment())
	if err != nil {
		return err
	}
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return err
	}
	s.setGatewayTracer(sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithBatcher(exporter)))
	return nil
}

func (s *Service) ShutdownGatewayTracing(ctx context.Context) error {
	return s.gatewayTracer.Shutdown(ctx)
}

func modelTraceID(ctx context.Context) string {
	id := trace.SpanContextFromContext(ctx).TraceID()
	if !id.IsValid() {
		return ""
	}
	return id.String()
}

func endTraceWithResponse(span trace.Span, response *UpstreamResponse) {
	if response == nil || response.Body == nil {
		span.End()
		return
	}
	response.Body = &tracedResponseBody{ReadCloser: response.Body, span: span}
}

type tracedResponseBody struct {
	io.ReadCloser
	span trace.Span
}

func (b *tracedResponseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.span.End()
	}
	return n, err
}

func (b *tracedResponseBody) Close() error {
	defer b.span.End()
	return b.ReadCloser.Close()
}

// Gorilla's upgrade uses a dialer, so its client span is created explicitly.
func (s *Service) traceGatewayWebsocket(ctx context.Context, headers http.Header) (context.Context, trace.Span) {
	ctx, span := s.gatewayTracer.Tracer("csgclaw/llm").Start(ctx, "openCSG responses websocket", trace.WithSpanKind(trace.SpanKindClient))
	propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(headers))
	return ctx, span
}
