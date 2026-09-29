package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBoundariesPersistAndIsolate(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	s.Source("room", "message", time.Now().Add(-20*time.Millisecond))
	r := s.Begin("room", "message", "thread", "agent", "turn")
	r.Running()
	r.RuntimeStart()
	received := time.Now().Add(15 * time.Millisecond)
	r.RuntimeEndAt(received)
	time.Sleep(25 * time.Millisecond)
	r.Failure("upstream_error", "runtime", "Authorization: Bearer opaque-token api_key=credential https://user:pass@example.test/?token=secret")
	r.Finish("failed")
	v := r.Snapshot()
	if v.RuntimeStartMS == nil || v.RuntimeEndMS == nil || *v.RuntimeStartMS < 20 || v.TotalMS-*v.RuntimeEndMS < 5 {
		t.Fatalf("boundaries=%+v", v)
	}
	if _, ok := s.Get("other-room", v.ID); ok {
		t.Fatal("cross-room lookup succeeded")
	}
	for _, secret := range []string{"opaque-token", "credential", "user:pass", "token=secret"} {
		if strings.Contains(v.Error.Message, secret) {
			t.Fatalf("secret retained: %s", v.Error.Message)
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		raw, err := os.ReadFile(filepath.Join(dir, v.ID+".json"))
		if err == nil && strings.Contains(string(raw), `"status":"failed"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not persisted")
		}
		time.Sleep(time.Millisecond * 10)
	}
	restored := New(dir)
	got, ok := restored.Get("room", v.ID)
	if !ok || got.Status != "failed" || got.TurnID != "turn" {
		t.Fatalf("restore=%+v", got)
	}
	s.DeleteRoom("room")
	if _, ok := s.Get("room", v.ID); ok {
		t.Fatal("deleted record visible")
	}
	if _, err := os.Stat(filepath.Join(dir, v.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("disk removal=%v", err)
	}
}
func TestSpanUnionInputsAndBoundedRecords(t *testing.T) {
	s := New("")
	r := s.Begin("r", "s", "", "a", "t")
	r.Running()
	ctx := WithRecord(context.Background(), r)
	finish := Measure(ctx, "input.prepare", "csgclaw")
	finish()
	finish()
	for i := 0; i < maxSpans+5; i++ {
		r.Start("tool", "tool", "")
	}
	r.Finish("succeeded")
	v := r.Snapshot()
	if len(v.Spans) > maxSpans || !v.Incomplete {
		t.Fatalf("unbounded spans: %d", len(v.Spans))
	}
	for _, span := range v.Spans {
		if span.EndMS == nil {
			t.Fatal("unfinished span after terminal")
		}
	}
}
func BenchmarkTurnRecording(b *testing.B) {
	s := New("")
	for i := 0; i < b.N; i++ {
		r := s.Begin("r", "s", "", "a", time.Now().String())
		r.Running()
		for j := 0; j < 20; j++ {
			id := r.Start("event.deliver", "csgclaw", "")
			r.End(id, "completed")
		}
		r.RuntimeStart()
		r.RuntimeEndAt(time.Now())
		r.Finish("succeeded")
		s.DeleteRoom("r")
	}
}

func TestRestartInterruptedAndCapacityCleanup(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	s.Source("r", "s", time.Now())
	r := s.Begin("r", "s", "", "a", "pending")
	r.Running()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	reloaded := New(dir)
	v, ok := reloaded.Get("r", r.Snapshot().ID)
	if !ok || v.Status != "interrupted" || !v.Incomplete {
		t.Fatalf("restart=%+v", v)
	}
	r.Finish("succeeded")
	if err := s.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.sizes[r.data.ID] = MaxBytes + 1
	s.bytes = MaxBytes + 1
	s.mu.Unlock()
	s.prune()
	if _, ok := s.Get("r", r.Snapshot().ID); ok {
		t.Fatal("capacity cap did not evict terminal record")
	}
}
func TestPersistenceFailureDoesNotFailTurn(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parent, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	s := New(filepath.Join(parent, "diagnostics"))
	s.Source("r", "s", time.Now())
	r := s.Begin("r", "s", "", "a", "t")
	r.Running()
	r.Finish("succeeded")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	v := r.Snapshot()
	if v.Status != "succeeded" || !v.Incomplete {
		t.Fatalf("write failure changed result: %+v", v)
	}
}
