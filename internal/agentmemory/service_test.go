package agentmemory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var testScope = Scope{AgentID: "agent-weekly", UserID: "user-admin"}

func openTestStore(t *testing.T, root, mode string) *Store {
	t.Helper()
	store, err := Open(root, filepath.Join(root, "global-state.json"), mode)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func historyOnce(id, text string) func(context.Context, Scope, time.Time, time.Time, HistoryCursor) ([]HistoryMessage, error) {
	return func(_ context.Context, _ Scope, _, _ time.Time, cursor HistoryCursor) ([]HistoryMessage, error) {
		if !cursor.At.IsZero() {
			return nil, nil
		}
		return []HistoryMessage{{At: time.Unix(1, 0), Message: Message{Source: Source{"private", id}, Text: text}}}, nil
	}
}

func getMemory(t *testing.T, store *Store, scope Scope) Document {
	t.Helper()
	doc, err := store.Get(scope)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func enableMemory(t *testing.T, store *Store, scope Scope) {
	t.Helper()
	revision := getMemory(t, store, scope).Revision
	enabled := true
	if _, err := store.Update(scope, Update{Revision: &revision, Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
}

func enqueueMemory(t *testing.T, store *Store, source, input string) {
	t.Helper()
	ticket, _, _, err := store.Snapshot(testScope, input)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Enqueue(ticket, source, Message{Source: Source{RoomID: "private", MessageID: source}, Text: input})
	if err != nil {
		t.Fatal(err)
	}
}

func runWorker(t *testing.T, worker Worker) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
}

func waitForMemory(t *testing.T, store *Store, condition func(Document) bool) Document {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		doc := getMemory(t, store, testScope)
		if condition(doc) {
			return doc
		}
		if time.Now().After(deadline) {
			t.Fatalf("unexpected memory status: %#v", doc)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestMemoryLearnsEachTurnAndForgetsByCurrentSource(t *testing.T) {
	store := openTestStore(t, t.TempDir(), "local")
	enableMemory(t, store, testScope)
	var extractions atomic.Int32
	worker := Worker{
		Store:   store,
		History: historyOnce("history", "Weekly reports should be short"),
		Extract: func(_ context.Context, _ string, input Extraction) ([]Change, error) {
			extractions.Add(1)
			if len(input.Messages) != 1 {
				return nil, fmt.Errorf("unexpected extraction batch: %d", len(input.Messages))
			}
			msg := input.Messages[0]
			switch msg.MessageID {
			case "history":
				return []Change{{Kind: "preference", Text: msg.Text, SourceMessageID: msg.MessageID}}, nil
			case "correction":
				if strings.Contains(msg.Text, "fixture-secret") || !strings.Contains(msg.Text, "[redacted]") {
					return nil, fmt.Errorf("credential reached model")
				}
				return []Change{{EntryID: input.Entries[0].ID, Kind: "preference", Text: "Weekly reports include risks", SourceMessageID: msg.MessageID}}, nil
			default:
				return nil, nil
			}
		},
	}
	runWorker(t, worker)
	waitForMemory(t, store, func(d Document) bool { return d.Initialization == "complete" && len(d.Entries) == 1 })
	_, oldBinding, _, _ := store.Snapshot(testScope, "")
	enqueueMemory(t, store, "correction", "Actually, include risks. password=fixture-secret")
	enqueueMemory(t, store, "routine", "Do this week's report")
	doc := waitForMemory(t, store, func(d Document) bool { return d.Status == "idle" && len(d.Entries) == 1 })
	if extractions.Load() != 3 || doc.Entries[0].Source.MessageID != "correction" {
		t.Fatalf("extractions=%d entries=%+v", extractions.Load(), doc.Entries)
	}
	_, binding, content, _ := store.Snapshot(testScope, "")
	if binding == oldBinding || !strings.Contains(content, "include risks") {
		t.Fatal("replacement did not rotate the native binding")
	}
	unchanged, err := store.Forget(testScope, Source{"private", "history"})
	if err != nil || len(unchanged.Entries) != 1 {
		t.Fatal("forgetting an older source changed the current item")
	}
	removed, err := store.Forget(testScope, Source{"private", "correction"})
	if err != nil || len(removed.Entries) != 0 || removed.Initialization != "complete" {
		t.Fatalf("forget failed: %+v %v", removed, err)
	}
	if other := getMemory(t, store, Scope{testScope.AgentID, "user-other"}); !other.Enabled || len(other.Entries) != 0 {
		t.Fatal("another user inherited memory")
	}
	file, err := os.ReadFile(filepath.Join(store.scopeDir(testScope), "MEMORY.md"))
	if err != nil || len(file) != 0 {
		t.Fatalf("forgotten text remains in MEMORY.md: %v", err)
	}
	if _, err := store.Update(testScope, Update{Revision: &doc.Revision, Entries: &[]Entry{}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update error = %v", err)
	}
	for _, enabled := range []bool{false, true} {
		current := getMemory(t, store, testScope)
		if _, err := store.Update(testScope, Update{Revision: &current.Revision, Enabled: &enabled}); err != nil {
			t.Fatal(err)
		}
	}
	if doc := getMemory(t, store, testScope); doc.Status != "idle" || doc.Initialization != "complete" {
		t.Fatalf("re-enable rescheduled history: %+v", doc)
	}
}

func TestMemoryAbnormalExitRestoresQueue(t *testing.T) {
	if root := os.Getenv("CSGCLAW_TEST_MEMORY_CRASH_ROOT"); root != "" {
		store := openTestStore(t, root, "local")
		enableMemory(t, store, testScope)
		worker := Worker{Store: store, Extract: func(ctx context.Context, _ string, input Extraction) ([]Change, error) {
			if input.Messages[0].MessageID != "crash" {
				return nil, fmt.Errorf("unexpected task")
			}
			if err := os.WriteFile(filepath.Join(root, "attempt-started"), nil, 0o600); err != nil {
				return nil, err
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}}
		enqueueMemory(t, store, "crash", "Remember durable pending information")
		worker.Run(context.Background())
		return
	}

	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^TestMemoryAbnormalExitRestoresQueue$")
	child.Env = append(os.Environ(), "CSGCLAW_TEST_MEMORY_CRASH_ROOT="+root)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "attempt-started")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not start the queued request")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("child did not exit abnormally")
	}
	store := openTestStore(t, root, "local")
	if got := getMemory(t, store, testScope); got.Status != "pending" || len(got.Entries) != 0 {
		t.Fatalf("pending update did not survive process exit: %+v", got)
	}
	var attempts atomic.Int32
	runWorker(t, Worker{Store: store, Extract: func(_ context.Context, _ string, input Extraction) ([]Change, error) {
		attempts.Add(1)
		return []Change{{Kind: "fact", Text: input.Messages[0].Text, SourceMessageID: input.Messages[0].MessageID}}, nil
	}})
	doc := waitForMemory(t, store, func(d Document) bool { return d.Status == "idle" && len(d.Entries) == 1 })
	enqueueMemory(t, store, "crash", "Remember durable pending information")
	if attempts.Load() != 1 || getMemory(t, store, testScope).Status != "idle" {
		t.Fatal("completed task was enqueued again")
	}
	md := filepath.Join(store.scopeDir(testScope), "MEMORY.md")
	if err := os.WriteFile(md, []byte("stale document"), 0o600); err != nil {
		t.Fatal(err)
	}
	recovered := openTestStore(t, root, "local")
	content, err := os.ReadFile(md)
	if err != nil || string(content) != doc.Content || len(getMemory(t, recovered, testScope).Entries) != 1 {
		t.Fatalf("projection recovery failed: %v", err)
	}
}

func TestMemoryDeletionAndDeploymentInvalidateInFlightJobs(t *testing.T) {
	root := t.TempDir()
	store := openTestStore(t, root, "local")
	enableMemory(t, store, testScope)
	Worker{Store: store}.process(context.Background(), testScope)
	// Model work can outlive its Agent or deployment. The captured state must
	// not commit to a recreated scope or a different policy.
	for _, transition := range []string{"delete", "mode"} {
		enqueueMemory(t, store, transition, "do not restore this")
		before, ok := store.next(testScope, time.Now())
		if !ok {
			t.Fatal("task missing")
		}
		task := before.Jobs[0]
		if transition == "delete" {
			if err := store.DeleteAgent(testScope.AgentID); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Dir(store.scopeDir(testScope))); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("memory directory survived deletion: %v", err)
			}
			enableMemory(t, store, testScope)
			Worker{Store: store}.process(context.Background(), testScope)
		} else {
			store = openTestStore(t, root, "disabled")
			if store.Policy() == before.Policy {
				t.Fatal("deployment policy did not rotate")
			}
		}
		if err := store.finish(before, task, []Message{*task.Message}, []Change{{
			Kind: "fact", Text: "stale", SourceMessageID: transition,
		}}, nil, nil); err != nil {
			t.Fatal(err)
		}
		if doc := getMemory(t, store, testScope); len(doc.Entries) != 0 {
			t.Fatal("stale task was committed")
		}
	}
}

func TestMemoryContextSelectsLongRelevantEntry(t *testing.T) {
	var entries []Entry
	for i := range 40 {
		entries = append(entries, Entry{Kind: "fact", Text: strings.Repeat("Ordinary project information. ", 20) + fmt.Sprint(i)})
	}
	entries = append(entries, Entry{Kind: "convention", Text: strings.Repeat("Detailed scope of a project. ", 400) + "周报使用风险矩阵。"})
	result := contextMarkdown(entries, "周报的风险矩阵")
	if len(result) > maxContextBytes || !strings.Contains(result, "风险矩阵") || len(markdown(entries)) <= maxContextBytes {
		t.Fatalf("unexpected context selection: %d bytes", len(result))
	}
}

func TestMemoryForgetBeforeLearningCommits(t *testing.T) {
	store := openTestStore(t, t.TempDir(), "local")
	enableMemory(t, store, testScope)
	Worker{Store: store}.process(context.Background(), testScope)
	enqueueMemory(t, store, "pending", "Keep this until I ask to forget")
	before, ok := store.next(testScope, time.Now())
	if !ok {
		t.Fatal("missing pending task")
	}
	task := before.Jobs[0]
	if _, err := store.Forget(testScope, task.Message.Source); err != nil {
		t.Fatal(err)
	}
	if err := store.finish(before, task, []Message{*task.Message}, []Change{{
		Kind: "fact", Text: "stale value", SourceMessageID: "pending",
	}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if doc := getMemory(t, store, testScope); len(doc.Entries) != 0 || doc.Status != "idle" {
		t.Fatalf("forget left a pending update: %+v", doc)
	}
}

func TestMemoryLocalDefaultsOnAndKeepsExplicitOptOut(t *testing.T) {
	root := t.TempDir()
	store := openTestStore(t, root, "local")
	doc, err := store.GetOrInitialize(testScope)
	if err != nil || !doc.Enabled || doc.Initialization != "pending" {
		t.Fatalf("first private use = %+v, %v", doc, err)
	}
	again, err := store.GetOrInitialize(testScope)
	if err != nil || again.Revision != doc.Revision {
		t.Fatalf("repeated read scheduled another job: %+v, %v", again, err)
	}
	enabled := false
	if _, err := store.Update(testScope, Update{Revision: &doc.Revision, Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	store = openTestStore(t, root, "local")
	doc, err = store.GetOrInitialize(testScope)
	if err != nil || doc.Enabled || doc.Status != "idle" {
		t.Fatalf("explicit opt-out after restart = %+v, %v", doc, err)
	}
}

func TestMemoryEmptyHistoryExtractionCanBeRetried(t *testing.T) {
	store := openTestStore(t, t.TempDir(), "local")
	enableMemory(t, store, testScope)
	worker := Worker{Store: store, History: historyOnce("historical", "A user message"), Extract: func(context.Context, string, Extraction) ([]Change, error) {
		return nil, nil
	}}
	worker.process(context.Background(), testScope)
	worker.process(context.Background(), testScope)
	doc := getMemory(t, store, testScope)
	if doc.Initialization != "empty" {
		t.Fatalf("initialization = %q", doc.Initialization)
	}
	if next, err := store.Update(testScope, Update{Revision: &doc.Revision, Retry: true}); err != nil || next.Initialization != "pending" {
		t.Fatalf("retry = %+v, %v", next, err)
	}
}

func TestMemoryExplicitHistoryLearningIncludesNewerHistoryAndKeepsManualEntries(t *testing.T) {
	store := openTestStore(t, t.TempDir(), "local")
	doc := getMemory(t, store, testScope)
	manual := []Entry{{Kind: "convention", Text: "Keep the manually edited agreement"}}
	if _, err := store.Update(testScope, Update{Revision: &doc.Revision, Entries: &manual}); err != nil {
		t.Fatal(err)
	}
	current, err := store.load(testScope)
	if err != nil {
		t.Fatal(err)
	}
	oldBoundary := time.Now().Add(-time.Hour)
	current.HistoryThrough = oldBoundary
	if err := store.commit(current); err != nil {
		t.Fatal(err)
	}
	// A normal refresh after clearing or editing never reimports deleted notes.
	for range 2 {
		if doc, err := store.GetOrInitialize(testScope); err != nil || doc.Status != "idle" || doc.Initialization != "complete" {
			t.Fatalf("ordinary read restarted history: %+v, %v", doc, err)
		}
	}
	doc = getMemory(t, store, testScope)
	since := time.Now()
	pending, err := store.Update(testScope, Update{Revision: &doc.Revision, Retry: true})
	if err != nil || pending.Status != "pending" || len(pending.Entries) != 1 {
		t.Fatalf("explicit history request failed: %+v, %v", pending, err)
	}
	if _, err := store.Update(testScope, Update{Revision: &pending.Revision, Retry: true}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate history request accepted: %v", err)
	}
	worker := Worker{Store: store, History: func(ctx context.Context, scope Scope, after, through time.Time, cursor HistoryCursor) ([]HistoryMessage, error) {
		if !after.Equal(store.instance.HistoryAfter) || through.Before(since) {
			t.Errorf("history boundary did not include newer messages: %v to %v", after, through)
		}
		return historyOnce("newer-history", "A preference newer than the original initialization")(ctx, scope, after, through, cursor)
	}, Extract: func(_ context.Context, _ string, input Extraction) ([]Change, error) {
		if len(input.Entries) != 1 || input.Entries[0].Text != manual[0].Text {
			t.Error("existing memory was not supplied to history extraction")
		}
		return []Change{{Kind: "preference", Text: input.Messages[0].Text, SourceMessageID: "newer-history"}}, nil
	}}
	worker.process(context.Background(), testScope)
	worker.process(context.Background(), testScope)
	done := getMemory(t, store, testScope)
	if done.Initialization != "complete" || done.Status != "idle" || len(done.Entries) != 2 || done.Entries[0].Text != manual[0].Text {
		t.Fatalf("history learning did not preserve and extend memory: %+v", done)
	}
	current, err = store.load(testScope)
	if err != nil || !current.HistoryThrough.After(oldBoundary) {
		t.Fatalf("history boundary was not saved: %v", err)
	}
}

func TestMemoryRecoveryAndCleanupStayWithinMemoryDirectory(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, testScope.AgentID, ".codex")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{"state.json", ".memory-runtime-owned"} {
		if err := os.WriteFile(filepath.Join(runtimeDir, filename), []byte("runtime-owned"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store := openTestStore(t, root, "local")
	enableMemory(t, store, testScope)
	store = openTestStore(t, root, "local")
	if err := store.DeleteAgent(testScope.AgentID); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{"state.json", ".memory-runtime-owned"} {
		data, err := os.ReadFile(filepath.Join(runtimeDir, filename))
		if err != nil || string(data) != "runtime-owned" {
			t.Fatalf("memory cleanup changed runtime file %s: %s, %v", filename, data, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, testScope.AgentID, "memory")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("memory directory survived cleanup: %v", err)
	}
}

func TestMemoryCleanupRejectsSymlinkedAgentHome(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(outside, "memory"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, testScope.AgentID)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	store := openTestStore(t, root, "local")
	if err := store.DeleteAgent(testScope.AgentID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("symlink cleanup error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "memory")); err != nil {
		t.Fatalf("cleanup crossed the Agent home: %v", err)
	}
}
