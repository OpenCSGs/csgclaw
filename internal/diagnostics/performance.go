package diagnostics

import (
	"context"
	"time"
)

// Metrics are inclusive observations, not an additive wall-clock breakdown.
// For example im.lock_hold includes serialization, write and sync durations.
type Metric struct {
	Count   uint64  `json:"count"`
	TotalMS float64 `json:"total_ms"`
	MaxMS   float64 `json:"max_ms"`
	Bytes   int64   `json:"bytes,omitempty"`
	Items   int64   `json:"items,omitempty"`
}
type Operation struct {
	record     *Record
	name       string
	started    time.Time
	metrics    map[string]Metric
	attributes map[string]any
}

func BeginOperation(ctx context.Context, name string) *Operation {
	record := From(ctx)
	if record == nil {
		return nil
	}
	metrics := map[string]Metric{}
	for _, name := range []string{"im.lock_wait", "im.lock_hold", "im.snapshot", "im.normalize", "im.serialize", "im.compare_read", "im.write", "im.fsync", "im.rename", "im.file_skipped", "im.cleanup", "im.publish", "im.attachment_prepare", "im.rooms_saved"} {
		metrics[name] = Metric{}
	}
	return &Operation{record: record, name: name, started: time.Now(), metrics: metrics, attributes: map[string]any{}}
}
func OperationArg(values []*Operation) *Operation {
	if len(values) > 0 {
		return values[0]
	}
	return nil
}
func (o *Operation) Measure(name string) func() {
	if o == nil {
		return func() {}
	}
	start := time.Now()
	return func() { o.Observe(name, time.Since(start), 0, 0) }
}
func (o *Operation) Observe(name string, duration time.Duration, bytes, items int64) {
	if o == nil {
		return
	}
	metric := o.metrics[name]
	metric.Count++
	elapsed := float64(duration) / float64(time.Millisecond)
	metric.TotalMS += elapsed
	metric.MaxMS = max(metric.MaxMS, elapsed)
	metric.Bytes += bytes
	metric.Items += items
	o.metrics[name] = metric
}
func (o *Operation) Attribute(name string, value any) {
	if o != nil {
		o.attributes[name] = value
	}
}
func (o *Operation) Finish() {
	if o == nil {
		return
	}
	o.Observe(o.name, time.Since(o.started), 0, 0)
	o.record.mu.Lock()
	if o.record.data.Performance == nil {
		o.record.data.Performance = map[string]Metric{}
	}
	for name, metric := range o.metrics {
		total := o.record.data.Performance[name]
		total.Count += metric.Count
		total.TotalMS += metric.TotalMS
		total.MaxMS = max(total.MaxMS, metric.MaxMS)
		total.Bytes += metric.Bytes
		total.Items += metric.Items
		o.record.data.Performance[name] = total
	}
	o.record.mu.Unlock()
	o.record.Capture("performance.operation", map[string]any{"name": o.name, "metrics": o.metrics, "attributes": o.attributes, "inclusive": true})
	o.record.store.schedule(o.record)
}

func (s *Store) ActivityCounts() map[string]int {
	counts := map[string]int{"queued": 0, "running": 0}
	if s == nil {
		return counts
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.records {
		record.mu.Lock()
		state := record.data.Status
		record.mu.Unlock()
		if state == "queued" || state == "running" {
			counts[state]++
		}
	}
	return counts
}

func ActivityFor(ctx context.Context) map[string]int {
	if r := From(ctx); r != nil {
		return r.store.ActivityCounts()
	}
	return nil
}

func MeasureMetric(ctx context.Context, name string) func() {
	record := From(ctx)
	if record == nil {
		return func() {}
	}
	start := time.Now()
	return func() {
		elapsed := float64(time.Since(start)) / float64(time.Millisecond)
		record.mu.Lock()
		if record.data.Performance == nil {
			record.data.Performance = map[string]Metric{}
		}
		metric := record.data.Performance[name]
		metric.Count++
		metric.TotalMS += elapsed
		metric.MaxMS = max(metric.MaxMS, elapsed)
		record.data.Performance[name] = metric
		record.mu.Unlock()
	}
}
