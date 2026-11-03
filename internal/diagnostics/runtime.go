package diagnostics

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
)

// InstanceID and byte boundaries distinguish successive processes appending to
// the same stderr file. Shared process evidence is never labeled exact-turn.
type RuntimeLogSource struct {
	Path               string    `json:"path"`
	RuntimeID          string    `json:"runtime_id"`
	InstanceID         string    `json:"instance_id"`
	PID                int       `json:"pid"`
	StartedAt          time.Time `json:"started_at"`
	ProcessStartOffset int64     `json:"process_start_offset"`
	ProcessEndOffset   int64     `json:"process_end_offset,omitempty"`
	TurnStartOffset    int64     `json:"turn_start_offset"`
	TurnEndOffset      int64     `json:"turn_end_offset,omitempty"`
}
type RuntimeObserver struct {
	mu      sync.Mutex
	source  RuntimeLogSource
	records map[string]*Record
}

func NewRuntimeObserver(path, runtimeID string, pid int, startOffset int64, writer io.Writer) *RuntimeObserver {
	source := RuntimeLogSource{Path: path, RuntimeID: runtimeID, InstanceID: rand.Text(), PID: pid, StartedAt: time.Now().UTC(), ProcessStartOffset: startOffset}
	if writer != nil {
		_, _ = fmt.Fprintf(writer, "time=%s level=INFO msg=%q runtime_id=%q pid=%d instance_id=%s\n", source.StartedAt.Format(time.RFC3339Nano), "runtime diagnostic process started", runtimeID, pid, source.InstanceID)
	}
	return &RuntimeObserver{source: source, records: map[string]*Record{}}
}
func logFileSize(path string) int64 {
	if info, err := os.Stat(path); err == nil {
		return info.Size()
	}
	return -1
}
func (o *RuntimeObserver) Bind(ctx context.Context) {
	if o == nil {
		return
	}
	record := From(ctx)
	if record == nil {
		return
	}
	o.mu.Lock()
	source := o.source
	source.TurnStartOffset = logFileSize(source.Path)
	for id, item := range o.records {
		if !runtimeRecordRelevant(item, time.Now()) {
			delete(o.records, id)
		}
	}
	if len(o.records) >= 128 {
		record.Issue("runtime_observer_capacity")
	} else {
		o.records[record.data.ID] = record
	}
	o.mu.Unlock()
	record.mu.Lock()
	if old := record.data.RuntimeLogSource; old != nil && old.InstanceID == source.InstanceID {
		source.TurnStartOffset = min(source.TurnStartOffset, old.TurnStartOffset)
	}
	record.data.RuntimeLogSource = &source
	record.mu.Unlock()
	record.Capture("runtime.log_source", source)
	record.store.schedule(record)
}
func (o *RuntimeObserver) End(ctx context.Context) {
	if o == nil {
		return
	}
	record := From(ctx)
	if record == nil {
		return
	}
	size := logFileSize(o.source.Path)
	record.mu.Lock()
	if record.data.RuntimeLogSource != nil {
		record.data.RuntimeLogSource.TurnEndOffset = size
	}
	record.mu.Unlock()
	record.store.schedule(record)
}
func runtimeRecordRelevant(record *Record, now time.Time) bool {
	record.mu.Lock()
	defer record.mu.Unlock()
	if record.data.Status == "running" || record.data.Status == "queued" {
		return true
	}
	end := record.data.StartedAt.Add(time.Duration(record.data.TotalMS * float64(time.Millisecond)))
	return !now.After(end.Add(time.Minute))
}
func (o *RuntimeObserver) Observe(kind string, err error, fields map[string]any) {
	if o == nil {
		return
	}
	now := time.Now().UTC()
	o.mu.Lock()
	records := make([]*Record, 0, len(o.records))
	for _, record := range o.records {
		records = append(records, record)
	}
	source := o.source
	o.mu.Unlock()
	detail := map[string]any{"kind": kind, "observed_at": now, "runtime_id": source.RuntimeID, "instance_id": source.InstanceID, "pid": source.PID, "association": "shared_runtime_process"}
	for key, value := range fields {
		detail[key] = value
	}
	if err != nil {
		detail["error"] = err.Error()
		detail["error_type"] = fmt.Sprintf("%T", err)
	}
	for _, record := range records {
		if !runtimeRecordRelevant(record, now) {
			continue
		}
		record.Capture("runtime.process_event", detail)
		ctx := WithRecord(context.Background(), record)
		slog.WarnContext(ctx, "runtime process event", "kind", kind, "runtime_id", source.RuntimeID, "pid", source.PID, "association", "shared_runtime_process", "details", Protect(detail))
	}
}
func (o *RuntimeObserver) Exit(code int, err, cleanupErr error) {
	if o == nil {
		return
	}
	size := logFileSize(o.source.Path)
	o.mu.Lock()
	o.source.ProcessEndOffset = size
	records := make([]*Record, 0, len(o.records))
	for _, record := range o.records {
		records = append(records, record)
	}
	o.mu.Unlock()
	for _, record := range records {
		record.mu.Lock()
		if record.data.RuntimeLogSource != nil {
			record.data.RuntimeLogSource.ProcessEndOffset = size
		}
		record.mu.Unlock()
		record.store.schedule(record)
	}
	fields := map[string]any{"exit_code": code}
	if cleanupErr != nil {
		fields["cleanup_error"] = cleanupErr.Error()
	}
	o.Observe("process_exit", err, fields)
}
