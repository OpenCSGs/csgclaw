package llm

import (
	"bytes"
	"csgclaw/internal/modelusage"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
)

// Observe the upstream wire format before translation can synthesize zero
// counters. Observation never changes response bytes or delays streaming.
func observeResponseUsage(resp *http.Response, observer func(bool)) {
	if observer == nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return
	}
	resp.Body = &usageObservedBody{ReadCloser: resp.Body, stream: strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream"), observer: observer}
}

type usageObservedBody struct {
	io.ReadCloser
	mu           sync.Mutex
	once         sync.Once
	stream       bool
	line, data   []byte
	skipping     bool
	valid        bool
	messageUsage map[string]any
	messageDelta bool
	observer     func(bool)
}

func (b *usageObservedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	if n > 0 {
		b.inspect(p[:n])
	}
	if err != nil {
		if b.stream {
			b.consumeLine()
			b.consumeEvent()
		} else {
			b.valid = validReportedUsage(b.data)
		}
		b.report(err == io.EOF && b.valid)
	}
	b.mu.Unlock()
	return n, err
}

func (b *usageObservedBody) Close() error {
	err := b.ReadCloser.Close()
	b.mu.Lock()
	b.report(false) // A body closed before its final accounting is incomplete.
	b.mu.Unlock()
	return err
}

func (b *usageObservedBody) report(complete bool) { b.once.Do(func() { b.observer(complete) }) }

func (b *usageObservedBody) inspect(chunk []byte) {
	if !b.stream {
		if !b.skipping && len(b.data)+len(chunk) <= maxResponsesSSEEventBytes {
			b.data = append(b.data, chunk...)
		} else {
			b.data = nil
			b.skipping = true
		}
		return
	}
	for _, c := range chunk {
		if c == '\n' {
			b.consumeLine()
			continue
		}
		if len(b.line) < maxResponsesSSEEventBytes {
			b.line = append(b.line, c)
		} else {
			b.skipping = true
		}
	}
}

func (b *usageObservedBody) consumeLine() {
	line := bytes.TrimSpace(b.line)
	if len(line) == 0 {
		b.consumeEvent()
	} else if bytes.HasPrefix(line, []byte("data:")) && !b.skipping {
		data := bytes.TrimSpace(line[5:])
		if len(b.data)+len(data)+1 > maxResponsesSSEEventBytes {
			b.data = nil
			b.skipping = true
		} else {
			if len(b.data) > 0 {
				b.data = append(b.data, '\n')
			}
			b.data = append(b.data, data...)
		}
	}
	b.line = b.line[:0]
}

func (b *usageObservedBody) consumeEvent() {
	defer func() { b.data = b.data[:0]; b.skipping = false }()
	if b.skipping {
		return
	}
	if bytes.Equal(b.data, []byte("[DONE]")) {
		b.report(b.valid)
		return
	}
	var event map[string]any
	if json.Unmarshal(b.data, &event) != nil {
		return
	}
	kind, _ := event["type"].(string)
	switch kind {
	case "message_start":
		message, _ := event["message"].(map[string]any)
		b.messageUsage, _ = message["usage"].(map[string]any)
	case "message_delta":
		if usage, ok := event["usage"].(map[string]any); ok {
			if b.messageUsage == nil {
				b.messageUsage = make(map[string]any)
			}
			for k, v := range usage {
				b.messageUsage[k] = v
			}
			_, b.messageDelta = modelusage.Number(usage["output_tokens"])
		}
	case "message_stop":
		b.valid = b.messageDelta && modelusage.Parse(b.messageUsage, "") != nil
		b.report(b.valid)
	case "response.completed", "response.incomplete":
		b.valid = validReportedUsage(b.data)
		b.report(b.valid)
	case "error", "response.failed":
		b.report(false)
	default:
		if usage, ok := event["usage"].(map[string]any); ok {
			b.valid = modelusage.Parse(usage, "") != nil
		}
	}
}

func validReportedUsage(raw []byte) bool {
	var response map[string]any
	if json.Unmarshal(raw, &response) != nil {
		return false
	}
	if nested, ok := response["response"].(map[string]any); ok {
		response = nested
	}
	usage, _ := response["usage"].(map[string]any)
	return modelusage.Parse(usage, "") != nil
}
