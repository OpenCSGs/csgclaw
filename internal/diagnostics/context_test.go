package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func contextEntries(t *testing.T, s *Store, id string) []ContextEntry {
	t.Helper()
	entries := []ContextEntry{}
	if err := s.ReadContext(context.Background(), id, func(entry ContextEntry) error { entries = append(entries, entry); return nil }); err != nil {
		t.Fatal(err)
	}
	return entries
}
func TestFullContextSurvivesRestartWithoutTruncationOrCredentials(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	record := store.Begin("r", "s", "", "a", "turn")
	text := strings.Repeat("完整上下文 ", 4000) + "END"
	record.Capture("test", map[string]any{"content": text, "api_key": "private-key", "headers": map[string]any{"Cookie": "session=private-cookie", "Authorization": "Bearer hidden"}, "arguments": `{"download_token":"private-download","content":"keep me"}`})
	record.Running()
	record.Finish("succeeded")
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	restored := New(dir)
	entries := contextEntries(t, restored, record.Snapshot().ID)
	raw, _ := json.Marshal(entries)
	for _, secret := range []string{"private-key", "private-cookie", "private-download", "Bearer hidden"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatalf("leaked %s", secret)
		}
	}
	var value map[string]any
	if err := json.Unmarshal(entries[1].Data, &value); err != nil {
		t.Fatal(err)
	}
	if value["content"] != text {
		t.Fatal("complete context was truncated")
	}
	if !bytes.Contains(raw, []byte("keep me")) {
		t.Fatal("tool argument content lost")
	}
	info, _ := os.Stat(filepath.Join(dir, "context", record.Snapshot().ID+".jsonl.gz"))
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%v", info.Mode())
	}
}
func TestDeletedRoomCannotBeResurrectedByQueuedCapture(t *testing.T) {
	store := New(t.TempDir())
	record := store.Begin("r", "s", "", "a", "turn")
	for i := 0; i < 30; i++ {
		record.Capture("queued", map[string]any{"i": i})
	}
	store.DeleteRoom("r")
	record.Capture("late", map[string]string{"content": "must not survive"})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get("r", record.Snapshot().ID); ok {
		t.Fatal("deleted record restored")
	}
	if _, err := os.Stat(filepath.Join(store.context.dir, record.Snapshot().ID+".jsonl.gz")); !os.IsNotExist(err) {
		t.Fatalf("deleted context restored: %v", err)
	}
}
func TestContextCapacityProtectsActiveTurnAndReportsLoss(t *testing.T) {
	store := New(t.TempDir())
	old := store.Begin("r", "old", "", "a", "old")
	old.Running()
	old.Finish("succeeded")
	active := store.Begin("r", "active", "", "b", "active")
	active.Running()
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	store.context.disk.Lock()
	store.context.limit = store.context.total + 64
	store.context.disk.Unlock()
	active.Capture("large", map[string]any{"text": strings.Repeat("123456789abcdefghijk", 1000)})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.context.dir, active.Snapshot().ID+".jsonl.gz")); err != nil {
		t.Fatal("active context removed", err)
	}
	store.context.disk.Lock()
	store.context.limit = 1
	store.context.disk.Unlock()
	active.Capture("overflow", map[string]string{"text": "exceeds budget"})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(active.Snapshot().CollectionIssues) == 0 {
		t.Fatal("capacity loss was silent")
	}
}
func TestLateNativeBindingAndSplitSSEAreCapturedSafely(t *testing.T) {
	store := New("")
	record := store.Begin("r", "s", "", "a", "turn")
	record.Running()
	ctx := WithNativeLookup(context.Background(), store, "a", "session", "native")
	recorder := NewRecorder(ctx, "custom-private-key")
	recorder.Capture("request", map[string]string{"content": "custom-private-key"})
	body := ObserveBody(io.NopCloser(strings.NewReader("data: {\"text\":\"Bearer split-token\"}\n\ndata: [DONE]\n\n")), recorder, "response", nil, "text/event-stream")
	// Simulate the model request preceding the native turn/start response.
	record.RuntimeRef("session", "native", "")
	record.Finish("succeeded")
	if _, err := io.CopyBuffer(io.Discard, body, make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	body.Close()
	entries := contextEntries(t, store, record.Snapshot().ID)
	raw, _ := json.Marshal(entries)
	if bytes.Contains(raw, []byte("split-token")) || bytes.Contains(raw, []byte("custom-private-key")) {
		t.Fatal("split or configured secret leaked")
	}
	if !bytes.Contains(raw, []byte("[DONE]")) || !bytes.Contains(raw, []byte("request")) {
		t.Fatal("late native evidence lost")
	}
}
func TestTurnLogPrefixConcurrentIsolationAndSystemWindow(t *testing.T) {
	journal := NewLogJournal(t.TempDir())
	var output bytes.Buffer
	logger := slog.New(NewServerLogHandler(&output, slog.LevelInfo, journal))
	store := New("")
	a := store.Begin("r", "a", "", "a", "turn-a")
	b := store.Begin("r", "b", "", "b", "turn-b")
	var group sync.WaitGroup
	for _, record := range []*Record{a, b} {
		group.Add(1)
		go func(record *Record) {
			defer group.Done()
			for i := 0; i < 20; i++ {
				logger.InfoContext(WithRecord(context.Background(), record), "turn log", "index", i)
			}
		}(record)
	}
	group.Wait()
	now := time.Now()
	logger.Warn("system failure", "api_key", "secret-value")
	journal.Add(LogEntry{At: now.Add(-5 * time.Minute), Message: "outside window"})
	for _, turn := range []string{"turn-a", "turn-b"} {
		count := 0
		if err := journal.Read(context.Background(), turn, nil, func(entry LogEntry) error {
			if entry.TurnID != turn {
				t.Fatal("cross-turn log")
			}
			count++
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if count != 20 {
			t.Fatalf("count=%d", count)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		if strings.Contains(line, "turn log") && !strings.HasPrefix(line, "[turn_id=turn-") {
			t.Fatalf("missing prefix: %s", line)
		}
	}
	count := 0
	err := journal.Read(context.Background(), "", []LogWindow{{now.Add(-time.Minute), now.Add(time.Minute)}}, func(entry LogEntry) error {
		if entry.Message != "system failure" {
			t.Fatalf("wrong system log %+v", entry)
		}
		raw, _ := json.Marshal(entry)
		if bytes.Contains(raw, []byte("secret-value")) {
			t.Fatal("log secret leaked")
		}
		count++
		return nil
	})
	if err != nil || count != 1 {
		t.Fatalf("system logs=%d %v", count, err)
	}
	restored := NewLogJournal(journal.dir)
	count = 0
	_ = restored.Read(context.Background(), "turn-a", nil, func(LogEntry) error { count++; return nil })
	if count != 20 {
		t.Fatal("logs did not survive restart")
	}
}

func TestRuntimeConfigurationFilesRedactEnvironmentAndCustomHeaders(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, []byte("model = 'fixture'\n[mcp_servers.tools.env]\nCUSTOM_PASSWORD = 'opaque-env-value'\n[mcp_servers.tools.http_headers]\nX-Custom-Auth = 'opaque-header-value'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := New("")
	record := store.Begin("r", "s", "", "a", "turn")
	record.CaptureRuntimeFiles(path)
	entries := contextEntries(t, store, record.Snapshot().ID)
	raw, _ := json.Marshal(entries)
	for _, value := range []string{"opaque-env-value", "opaque-header-value"} {
		if bytes.Contains(raw, []byte(value)) {
			t.Fatalf("config credential leaked: %s", value)
		}
	}
	if !bytes.Contains(raw, []byte("fixture")) {
		t.Fatal("non-sensitive model configuration lost")
	}
}

func TestLogCollectionFailureRemainsVisibleAfterRestart(t *testing.T) {
	journal := NewLogJournal(t.TempDir())
	journal.limit = 1
	journal.Add(LogEntry{At: time.Now(), TurnID: "turn", Message: "cannot fit"})
	if err := journal.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	restored := NewLogJournal(journal.dir)
	if err := restored.Read(context.Background(), "turn", nil, func(LogEntry) error { return nil }); err == nil {
		t.Fatal("storage loss was silently forgotten on restart")
	}
}
