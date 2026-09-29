package llm

import (
	"bytes"
	"csgclaw/internal/diagnostics"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Measure every upstream attempt through response consumption, including
// streaming bodies and retries. This is client-observed LLM service latency,
// including network/gateway time, not pure model compute or first-token time.
func (s *Service) modelRequest(req *http.Request) (*http.Response, error) {
	if !diagnostics.Enabled(req.Context()) {
		return s.client.Do(req)
	}
	operation := "responses"
	if strings.HasSuffix(req.URL.Path, "/chat/completions") {
		operation = "chat/completions"
	}
	update, finish := diagnostics.ObserveRequest(req.Context(), operation)
	start := time.Now()
	resp, err := s.client.Do(req)
	if err != nil {
		finish("failed")
		return resp, err
	}
	update(resp.StatusCode, nil)
	resp.Body = &observedModelBody{ReadCloser: resp.Body, started: start, status: resp.StatusCode, stream: strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream"), update: update, finish: finish}
	return resp, nil
}

type observedModelBody struct {
	io.ReadCloser
	mu             sync.Mutex
	stream         bool
	line           []byte
	skipLine       bool
	protocolFailed bool
	terminal       bool
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
		b.first.Do(func() {
			elapsed := float64(time.Since(b.started)) / float64(time.Millisecond)
			b.update(b.status, &elapsed)
		})
		b.mu.Lock()
		terminal := b.terminal
		b.mu.Unlock()
		if terminal {
			status := "completed"
			if b.failed() {
				status = "failed"
			}
			b.ended.Do(func() { b.finish(status) })
		}
	}
	if err != nil {
		status := "completed"
		if err != io.EOF || b.failed() {
			status = "failed"
		}
		b.ended.Do(func() { b.finish(status) })
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
		b.finish(status)
	})
	return err
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
			if bytes.Equal(line, []byte("event: error")) || bytes.Equal(line, []byte("event: response.failed")) {
				b.protocolFailed = true
				b.terminal = true
			}
			if bytes.HasPrefix(line, []byte("data:")) {
				var event struct {
					Type  string          `json:"type"`
					Error json.RawMessage `json:"error"`
				}
				parseErr := json.Unmarshal(bytes.TrimSpace(line[5:]), &event)
				if parseErr == nil && event.Type == "response.completed" {
					b.terminal = true
				}
				if parseErr == nil && (event.Type == "response.failed" || event.Type == "error" || len(event.Error) > 0 && string(event.Error) != "null") {
					b.protocolFailed = true
					b.terminal = true
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
