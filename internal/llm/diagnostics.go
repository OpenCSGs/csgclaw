package llm

import (
	"bytes"
	"context"
	"csgclaw/internal/diagnostics"
	"csgclaw/internal/modelprovider"
	"csgclaw/internal/modelusage"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// Measure every upstream attempt through response consumption, including
// streaming bodies and retries. This is client-observed LLM service latency,
// including network/gateway time, not pure model compute or first-token time.
func (s *Service) modelRequestAttempt(client *http.Client, req *http.Request, stream bool) (*http.Response, error) {
	exchange, _ := req.Context().Value(modelExchangeKey{}).(*modelExchange)
	managed := exchange != nil && exchange.openCSG
	var attempt uint64
	if exchange != nil {
		attempt = exchange.attempt.Add(1)
		fields := map[string]any{"request_id": exchange.id, "attempt": attempt, "method": req.Method, "url": req.URL.String()}
		if exchange.openCSG {
			fields["trace_id"] = modelTraceID(req.Context())
		}
		if req.GetBody != nil {
			if body, err := req.GetBody(); err == nil {
				raw, readErr := io.ReadAll(io.LimitReader(body, 128<<20))
				body.Close()
				if readErr == nil && json.Valid(raw) {
					fields["body"] = json.RawMessage(raw)
				}
			}
		}
		exchange.recorder.Capture("model.upstream_request", fields)
	}
	logModelRequest(req)
	operation := "responses"
	if strings.HasSuffix(req.URL.Path, "/chat/completions") {
		operation = "chat/completions"
	}
	var update func(int, *float64)
	var finish func(string)
	if diagnostics.Enabled(req.Context()) {
		update, finish = diagnostics.ObserveRequest(req.Context(), operation)
	}
	start := time.Now()
	resp, err := s.doModelRequest(client, req, managed, stream)
	observeModelCapture(req.Context(), exchange, attempt, resp, err)
	if err != nil {
		logModelTransportFailure(req, err)
		if finish != nil {
			finish("failed")
		}
		return resp, err
	}
	if update != nil {
		update(resp.StatusCode, nil)
	}
	observeResponseUsage(resp, stream, modelusage.RequestObserver(req.Context()))
	if finish != nil || managed {
		resp.Body = &observedModelBody{
			ReadCloser: resp.Body, started: start, status: resp.StatusCode, stream: strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream"),
			update: update, finish: finish, response: resp, ctx: req.Context(), strict: managed, chat: operation == "chat/completions",
		}
	}
	return resp, nil
}

func (s *Service) doModelRequest(client *http.Client, req *http.Request, managed, stream bool) (*http.Response, error) {
	if !managed {
		return client.Do(req)
	}
	// The instrumented transport injects its own client span, including after
	// every retry. Ignore any manually configured propagation headers.
	req.Header.Del("Traceparent")
	req.Header.Del("Tracestate")
	if s.modelHeaderTimeout <= 0 {
		return client.Do(req)
	}
	ctx, cancel := context.WithCancelCause(req.Context())
	deadline := time.AfterFunc(s.modelHeaderTimeout, func() { cancel(context.DeadlineExceeded) })
	response, err := client.Do(req.WithContext(ctx))
	deadline.Stop()
	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			err = cause
		}
		cancel(nil)
		return response, err
	}
	idle := s.modelHeaderTimeout
	if stream && s.modelIdleTimeout > 0 {
		idle = s.modelIdleTimeout
	}
	response.Body = &modelIdleBody{ReadCloser: response.Body, ctx: ctx, cancel: cancel, timeout: idle, lastProgress: time.Now(),
		sse: stream && strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream")}
	return response, nil
}

type modelIdleBody struct {
	io.ReadCloser
	ctx          context.Context
	cancel       context.CancelCauseFunc
	timeout      time.Duration
	lastProgress time.Time
	sse          bool
	linePrefix   int
}

func (b *modelIdleBody) Read(p []byte) (int, error) {
	remaining := time.Until(b.lastProgress.Add(b.timeout))
	if remaining <= 0 {
		b.cancel(context.DeadlineExceeded)
		return 0, context.DeadlineExceeded
	}
	timer := time.AfterFunc(remaining, func() { b.cancel(context.DeadlineExceeded) })
	n, err := b.ReadCloser.Read(p)
	timer.Stop()
	if n > 0 && (!b.sse || b.hasStreamData(p[:n])) {
		b.lastProgress = time.Now()
	}
	if err != nil {
		if cause := context.Cause(b.ctx); errors.Is(cause, context.DeadlineExceeded) {
			err = cause
		}
	}
	return n, err
}

// SSE comments are transport keepalives, not model progress.
func (b *modelIdleBody) hasStreamData(data []byte) bool {
	const prefix = "data:"
	progress := false
	for _, ch := range data {
		switch {
		case ch == '\n':
			b.linePrefix = 0
		case b.linePrefix < 0:
			continue
		case b.linePrefix == len(prefix):
			progress = progress || (ch != ' ' && ch != '\r' && ch != '\t')
		case ch == prefix[b.linePrefix]:
			b.linePrefix++
		default:
			b.linePrefix = -1
		}
	}
	return progress
}

func (b *modelIdleBody) Close() error {
	defer b.cancel(nil)
	return b.ReadCloser.Close()
}

type observedModelBody struct {
	io.ReadCloser
	mu             sync.Mutex
	stream         bool
	line           []byte
	skipLine       bool
	protocolFailed bool
	errorEvent     []byte
	terminal       bool
	chatFinished   bool
	strict         bool
	chat           bool
	response       *http.Response
	ctx            context.Context
	started        time.Time
	status         int
	first          sync.Once
	ended          sync.Once
	update         func(int, *float64)
	finish         func(string)
}

func (b *observedModelBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.inspect(p[:n])
		if b.update != nil {
			b.first.Do(func() {
				elapsed := float64(time.Since(b.started)) / float64(time.Millisecond)
				b.update(b.status, &elapsed)
			})
		}
		b.mu.Lock()
		terminal := b.terminal
		b.mu.Unlock()
		if terminal {
			status := "completed"
			if b.failed() {
				status = "failed"
			}
			b.ended.Do(func() { b.complete(status, nil) })
		}
	}
	if err != nil {
		b.mu.Lock()
		truncated := b.stream && b.status < 400 && !b.terminal && !b.chatFinished
		b.mu.Unlock()
		if b.strict && err == io.EOF && truncated {
			err = io.ErrUnexpectedEOF
		}
		status := "completed"
		if err != io.EOF || b.failed() || truncated {
			status = "failed"
		}
		b.ended.Do(func() { b.complete(status, err) })
	}
	return n, err
}
func (b *observedModelBody) Close() error {
	err := b.ReadCloser.Close()
	b.ended.Do(func() {
		status := "interrupted"
		if err != nil || b.failed() {
			status = "failed"
		}
		if b.finish != nil {
			b.finish(status)
		}
	})
	return err
}

func (b *observedModelBody) complete(outcome string, err error) {
	if b.finish != nil {
		b.finish(outcome)
	}
	if outcome != "failed" || !b.stream || b.status >= 400 || b.ctx == nil {
		return
	}
	b.mu.Lock()
	data, protocolFailed := b.errorEvent, b.protocolFailed
	b.mu.Unlock()
	status := modelRequestFailure(b.ctx, b.response, data, err)
	if status.Category != "timeout" && status.Category != "rate_limit" {
		status.Category = "stream_interrupted"
		if protocolFailed && status.Code != "" && status.Source == "model" {
			status.Category = "rejected"
		}
	}
	reportModelRequestFailure(b.ctx, status)
}

// Inspect bounded event metadata only. Large output events are skipped and no
// model content is retained in diagnostic data.
func (b *observedModelBody) inspect(data []byte) {
	if !b.stream {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for len(data) > 0 {
		piece, rest, found := bytes.Cut(data, []byte{'\n'})
		if !b.skipLine {
			if len(b.line)+len(piece) > 64*1024 {
				b.skipLine = true
				b.line = nil
			} else {
				b.line = append(b.line, piece...)
			}
		}
		if !found {
			return
		}
		if !b.skipLine {
			line := bytes.TrimSpace(b.line)
			if bytes.Equal(line, []byte("data: [DONE]")) {
				b.terminal = true
			}
			if bytes.Equal(line, []byte("event: error")) || bytes.Equal(line, []byte("event: response.failed")) || bytes.Equal(line, []byte("event: response.incomplete")) {
				b.protocolFailed = true
			}
			if bytes.HasPrefix(line, []byte("data:")) {
				data := bytes.TrimSpace(line[5:])
				var event struct {
					Type    string `json:"type"`
					Choices []struct {
						FinishReason string `json:"finish_reason"`
					} `json:"choices"`
				}
				parseErr := json.Unmarshal(data, &event)
				if parseErr == nil && event.Type == "response.completed" {
					b.terminal = true
				}
				_, errorPayload := modelprovider.UpstreamError(data)
				if parseErr == nil && (b.protocolFailed || event.Type == "response.failed" || event.Type == "response.incomplete" || event.Type == "error" || errorPayload) {
					b.protocolFailed = true
					b.terminal = true
					if len(data) <= 16<<10 {
						b.errorEvent = bytes.Clone(data)
					}
				}
				if b.chat && parseErr == nil {
					for _, choice := range event.Choices {
						b.chatFinished = b.chatFinished || choice.FinishReason != ""
					}
				}
			}
		}
		b.line = nil
		b.skipLine = false
		data = rest
	}
}
func (b *observedModelBody) failed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status >= 400 || b.protocolFailed
}

func observeModelCapture(ctx context.Context, exchange *modelExchange, attempt uint64, resp *http.Response, err error) {
	if exchange == nil {
		return
	}
	metadata := map[string]any{"request_id": exchange.id, "attempt": attempt}
	if traceID := modelTraceID(ctx); exchange.openCSG && traceID != "" {
		metadata["trace_id"] = traceID
	}
	if err != nil {
		metadata["error"] = err.Error()
		exchange.recorder.Capture("model.upstream_error", metadata)
		return
	}
	if resp == nil {
		return
	}
	metadata["status"] = resp.StatusCode
	metadata["upstream_request_id"] = resp.Header.Get("X-Request-Id")
	metadata["content_type"] = resp.Header.Get("Content-Type")
	metadata["retry_after"] = resp.Header.Get("Retry-After")
	if exchange.openCSG {
		clientSpanID := ""
		if resp.Request != nil {
			if spanID := trace.SpanContextFromContext(resp.Request.Context()).SpanID(); spanID.IsValid() {
				clientSpanID = spanID.String()
				metadata["client_span_id"] = clientSpanID
			}
		}
		slog.InfoContext(ctx, "OpenCSG model response received", "request_id", exchange.id, "attempt", attempt,
			"trace_id", modelTraceID(ctx), "client_span_id", clientSpanID, "gateway_request_id", diagnostics.ProtectText(upstreamRequestID(resp.Header)), "status", resp.StatusCode)
	}
	exchange.recorder.Capture("model.upstream_headers", metadata)
	resp.Body = diagnostics.ObserveBody(resp.Body, exchange.recorder, "model.upstream_response", metadata, resp.Header.Get("Content-Type"))
}
