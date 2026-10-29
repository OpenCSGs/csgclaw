package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogCoverageDistinguishesQuietIntervalsRestartGapsAndMissingShards(t *testing.T) {
	dir := t.TempDir()
	journal := NewLogJournal(dir)
	journal.started = time.Now().Add(-2 * time.Minute)
	now := time.Now().UTC()
	journal.Add(LogEntry{At: now.Add(-time.Minute), Message: "system log"})
	if err := journal.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	requested := []LogWindow{{Start: journal.started, End: now}}
	if reasons := journal.Coverage(requested); len(reasons) > 0 {
		t.Fatalf("quiet interval treated as a gap: %+v", reasons)
	}
	time.Sleep(3 * time.Millisecond)
	restored := NewLogJournal(dir)
	if reasons := restored.Coverage(requested); len(reasons) > 0 {
		t.Fatalf("persisted coverage lost: %+v", reasons)
	}
	if reasons := restored.Coverage([]LogWindow{{Start: journal.started, End: time.Now()}}); len(reasons) == 0 {
		t.Fatal("restart gap falsely reported complete")
	}
	path := filepath.Join(dir, "system-"+now.Add(-time.Minute).Format("20060102T15")+".gz")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := journal.Read(context.Background(), "", requested, func(LogEntry) error { return nil }); err == nil {
		t.Fatal("known log shard disappeared silently")
	}
	missing := NewLogJournal(dir)
	if reasons := missing.Coverage(requested); len(reasons) == 0 {
		t.Fatal("deleted shard falsely reported complete after restart")
	}
}
