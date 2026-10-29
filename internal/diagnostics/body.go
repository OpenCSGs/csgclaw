package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
)

const maxBodyCaptureBytes = 128 << 20

// Recorder holds a bounded number of early protocol events until the native
// turn/start response supplies the exact identity. No Agent-level guessing.
type Recorder struct {
	mu      sync.Mutex
	ctx     context.Context
	pending []struct {
		kind  string
		value json.RawMessage
	}
	bytes   int
	secrets []string
}

func NewRecorder(ctx context.Context, secrets ...string) *Recorder {
	if !Enabled(ctx) {
		return nil
	}
	return &Recorder{ctx: ctx, secrets: secrets}
}
func (r *Recorder) Capture(kind string, value any) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	raw := Protect(value)
	// Replace configured credentials even when they occur in arbitrary text.
	for _, secret := range r.secrets {
		if len(secret) >= 4 {
			var quoted string
			encoded, _ := json.Marshal(secret)
			quoted = string(encoded[1 : len(encoded)-1])
			raw = bytes.ReplaceAll(raw, []byte(quoted), []byte("[redacted]"))
		}
	}
	r.pending = append(r.pending, struct {
		kind  string
		value json.RawMessage
	}{kind, raw})
	r.bytes += len(raw)
	for len(r.pending) > 0 {
		item := r.pending[0]
		if !Capture(r.ctx, item.kind, item.value) {
			break
		}
		r.pending[0] = struct {
			kind  string
			value json.RawMessage
		}{}
		r.pending = r.pending[1:]
		r.bytes -= len(item.value)
	}
	if r.bytes > maxBodyCaptureBytes {
		r.pending = nil
		r.bytes = 0
		Capture(r.ctx, "collection.issue", map[string]string{"reason": "native_identity_unavailable"})
	}
}

// ObserveBody observes bytes as the consumer reads them. It does not pre-read,
// delay delivery, or alter close/cancellation behavior. SSE is recorded by full
// data lines so a secret split across network chunks is redacted correctly.
func ObserveBody(body io.ReadCloser, recorder *Recorder, kind string, metadata map[string]any, contentType string) io.ReadCloser {
	if body == nil || recorder == nil {
		return body
	}
	return &capturedBody{ReadCloser: body, recorder: recorder, kind: kind, metadata: metadata, stream: strings.HasPrefix(strings.ToLower(contentType), "text/event-stream")}
}

type capturedBody struct {
	io.ReadCloser
	mu        sync.Mutex
	recorder  *Recorder
	kind      string
	metadata  map[string]any
	pending   []byte
	stream    bool
	ended     bool
	truncated bool
}

func (b *capturedBody) capture(data []byte) {
	value := map[string]any{}
	for k, v := range b.metadata {
		value[k] = v
	}
	if b.stream {
		line := bytes.TrimSpace(data)
		if !bytes.HasPrefix(line, []byte("data:")) {
			if len(line) > 0 {
				value["event"] = string(line)
				b.recorder.Capture(b.kind, value)
			}
			return
		}
		data = bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
	}
	if json.Valid(data) {
		value["body"] = json.RawMessage(append([]byte(nil), data...))
	} else {
		value["body"] = string(data)
	}
	b.recorder.Capture(b.kind, value)
}
func (b *capturedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.ended && !b.truncated {
		data := p[:n]
		for len(data) > 0 {
			piece, rest, found := bytes.Cut(data, []byte{'\n'})
			if !b.stream {
				piece = data
				found = false
			}
			if len(b.pending)+len(piece) > maxBodyCaptureBytes {
				b.truncated = true
				b.pending = nil
				b.recorder.Capture("collection.issue", map[string]any{"reason": "model_event_too_large", "stage": b.kind})
				break
			}
			b.pending = append(b.pending, piece...)
			if !found {
				break
			}
			b.capture(b.pending)
			b.pending = nil
			data = rest
		}
	}
	if err != nil {
		b.finish(err == io.EOF)
	}
	return n, err
}
func (b *capturedBody) finish(complete bool) {
	if b.ended {
		return
	}
	b.ended = true
	if len(b.pending) > 0 {
		b.capture(b.pending)
		b.pending = nil
	}
	value := map[string]any{"stage": b.kind, "complete": complete && !b.truncated}
	for k, v := range b.metadata {
		value[k] = v
	}
	b.recorder.Capture("model.body_end", value)
}
func (b *capturedBody) Close() error {
	err := b.ReadCloser.Close()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.finish(false)
	return err
}
