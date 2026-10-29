package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRuntimeProcessWindowKeepsUnattributedErrorsAndExcludesOtherTurns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stderr.log")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	observer := NewRuntimeObserver(path, "runtime", 123, 0, file)
	store := New("")
	a := store.Begin("r", "a", "", "a", "turn-a")
	b := store.Begin("r", "b", "", "b", "turn-b")
	a.Running()
	b.Running()
	ctxA := WithRecord(context.Background(), a)
	observer.Bind(ctxA)
	observer.Bind(WithRecord(context.Background(), b))
	at := time.Now().UTC()
	for _, line := range []string{"turn_id=turn-a exact", "turn_id=turn-ab wrong prefix", "turn_id=turn-b other turn", "ERROR MCP transport reset", "  nested stack line"} {
		fmt.Fprintf(file, "%s %s\n", at.Format(time.RFC3339Nano), line)
	}
	fmt.Fprintf(file, "[turn_id=turn-a] time=%s level=INFO msg=\"prefixed exact log\"\n", at.Format(time.RFC3339Nano))
	fmt.Fprintf(file, `{"timestamp":"%s","level":"ERROR","fields":{"message":"native structured process error"}}`+"\n", at.Format(time.RFC3339Nano))
	observer.Observe("stdout_decode_failed", errors.New("invalid protocol JSON"), nil)
	observer.End(ctxA)
	a.Finish("failed")
	observer.Exit(2, errors.New("process exited"), nil)
	source := a.Snapshot().RuntimeLogSource
	var entries []RuntimeLogEntry
	report := ReadRuntimeLogs(context.Background(), *source, LogWindow{at.Add(-time.Minute), time.Now().Add(time.Minute)}, []string{"turn-a"}, func(entry RuntimeLogEntry) error { entries = append(entries, entry); return nil })
	if len(report.Reasons) > 0 {
		t.Fatalf("report=%+v", report)
	}
	var output strings.Builder
	for _, entry := range entries {
		output.WriteString(entry.Line)
		if strings.Contains(entry.Line, "MCP transport") && entry.Association != "runtime_process_window" {
			t.Fatal("unattributed line mislabeled exact-turn")
		}
	}
	if !strings.Contains(output.String(), "MCP transport reset") || strings.Contains(output.String(), "wrong prefix") || strings.Contains(output.String(), "other turn") {
		t.Fatalf("incorrect runtime context: %s", output.String())
	}
	for _, record := range []*Record{a, b} {
		kinds := 0
		for _, entry := range contextEntries(t, store, record.Snapshot().ID) {
			if entry.Kind == "runtime.process_event" {
				kinds++
			}
		}
		if kinds != 2 {
			t.Fatalf("process evidence missing from affected turn: %d", kinds)
		}
	}
	// Appending a new instance cannot extend the old instance's byte boundary.
	NewRuntimeObserver(path, "runtime", 124, source.ProcessEndOffset, file)
	fmt.Fprintf(file, "%s ERROR new process private text\n", time.Now().UTC().Format(time.RFC3339Nano))
	report = ReadRuntimeLogs(context.Background(), *source, LogWindow{at.Add(-time.Minute), time.Now().Add(time.Minute)}, []string{"turn-a"}, func(entry RuntimeLogEntry) error {
		if strings.Contains(entry.Line, "new process") {
			t.Fatal("new process leaked into old source")
		}
		return nil
	})
	if len(report.Reasons) > 0 {
		t.Fatal(report)
	}
}
func TestRuntimeLogRotationAndUntimedLinesAreMarked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stderr.log")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	observer := NewRuntimeObserver(path, "r", 123, 0, file)
	store := New("")
	record := store.Begin("room", "source", "", "a", "turn")
	record.Running()
	ctx := WithRecord(context.Background(), record)
	observer.Bind(ctx)
	fmt.Fprintln(file, "untimed raw runtime error")
	observer.End(ctx)
	file.Close()
	report := ReadRuntimeLogs(context.Background(), *record.Snapshot().RuntimeLogSource, LogWindow{time.Now().Add(-time.Minute), time.Now().Add(time.Minute)}, []string{"turn"}, func(RuntimeLogEntry) error { return nil })
	// Marker timestamps can time following stack lines; explicit byte offsets
	// remain available even when native stderr supplies no timestamp of its own.
	if report.Lines == 0 {
		t.Fatal("untimed process context disappeared")
	}
	if err := os.WriteFile(path, []byte("different process"), 0600); err != nil {
		t.Fatal(err)
	}
	report = ReadRuntimeLogs(context.Background(), *record.Snapshot().RuntimeLogSource, LogWindow{time.Now().Add(-time.Minute), time.Now().Add(time.Minute)}, []string{"turn"}, func(RuntimeLogEntry) error { t.Fatal("rotated file exported"); return nil })
	if len(report.Reasons) == 0 {
		t.Fatal("rotation silently reported complete")
	}
}
