package diagnostics

import (
	"encoding/json"
	"time"
)

// NativeSpan imports a separately clocked native observation. It may arrive
// after delivery; its identity makes OTLP retries idempotent. It never changes
// the measured CSGClaw boundaries or interprets async idle time as CPU time.
func (r *Record) NativeSpan(id, name, category, trace, parent string, start, end time.Time) {
	if r == nil {
		return
	}
	r.mu.Lock()
	for _, span := range r.data.Spans {
		if span.ID == id {
			r.mu.Unlock()
			return
		}
	}
	if len(r.data.Spans) >= maxSpans {
		r.data.Incomplete = true
		r.mu.Unlock()
		return
	}
	left, right := ms(start.Sub(r.data.StartedAt)), ms(end.Sub(r.data.StartedAt))
	if left < 0 || right < left {
		r.mu.Unlock()
		return
	}
	details := &SpanDetails{Label: name, Source: "codex_otel", Category: category, TraceID: trace, ParentSpanID: parent}
	raw, _ := json.Marshal(details)
	if r.detailBytes+len(raw) > 256<<10 {
		r.data.Incomplete = true
		r.mu.Unlock()
		return
	}
	r.detailBytes += len(raw)
	r.data.Spans = append(r.data.Spans, Span{ID: id, Name: "runtime.native", Owner: "runtime", StartMS: left, EndMS: &right, Status: "completed", Details: details})
	r.mu.Unlock()
	r.store.schedule(r)
}
