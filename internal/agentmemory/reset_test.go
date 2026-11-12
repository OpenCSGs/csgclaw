package agentmemory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResetAtomicallyClearsMemoryAndRestartsHistoryWithoutRevivingOldWork(t *testing.T) {
	root := t.TempDir()
	store := openTestStore(t, root, "local")
	doc := getMemory(t, store, testScope)
	notes := []Entry{{Kind: "preference", Text: "Manually edited old preference"}}
	doc, err := store.Update(testScope, Update{Revision: &doc.Revision, Entries: &notes})
	if err != nil {
		t.Fatal(err)
	}
	_, binding, _, err := store.Snapshot(testScope, "")
	if err != nil {
		t.Fatal(err)
	}
	enqueueMemory(t, store, "old-turn", "An old in-flight update")
	before, ok := store.next(testScope, time.Now())
	if !ok {
		t.Fatal("old job missing")
	}
	canceled := false
	if !store.beginAttempt(before, func() { canceled = true }) {
		t.Fatal("attempt did not start")
	}
	current := getMemory(t, store, testScope)
	since := time.Now()
	reset, err := store.Update(testScope, Update{Revision: &current.Revision, Reset: true})
	if err != nil || len(reset.Entries) != 0 || reset.Initialization != "pending" || reset.Status != "pending" || !reset.Enabled || !canceled {
		t.Fatalf("reset failed: %+v, %v", reset, err)
	}
	file, err := os.ReadFile(filepath.Join(store.scopeDir(testScope), "MEMORY.md"))
	if err != nil || len(file) != 0 {
		t.Fatalf("reset did not clear disk projection: %v", err)
	}
	queued, err := store.load(testScope)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued.Jobs) != 1 || queued.Jobs[0].Message != nil || queued.Jobs[0].ID == before.Jobs[0].ID || !queued.Jobs[0].HistoryCursor.At.IsZero() || queued.HistoryThrough.Before(since) {
		t.Fatal("reset did not create a fresh history task")
	}
	if err := store.finish(before, before.Jobs[0], []Message{*before.Jobs[0].Message}, []Change{{Kind: "fact", Text: "Must not return", SourceMessageID: "old-turn"}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	store = openTestStore(t, root, "local")
	_, afterBinding, _, err := store.Snapshot(testScope, "")
	if err != nil || afterBinding == binding {
		t.Fatal("reset retained the old runtime memory context")
	}
	worker := Worker{Store: store, History: historyOnce("history", "Relearned from private history"), Extract: func(_ context.Context, _ string, input Extraction) ([]Change, error) {
		if len(input.Entries) != 0 {
			t.Error("reset supplied deleted manual notes to extraction")
		}
		return []Change{{Kind: "fact", Text: input.Messages[0].Text, SourceMessageID: input.Messages[0].MessageID}}, nil
	}}
	worker.process(t.Context(), testScope)
	worker.process(t.Context(), testScope)
	if doc := getMemory(t, store, testScope); doc.Status != "idle" || doc.Initialization != "complete" || len(doc.Entries) != 1 || doc.Entries[0].Text != "Relearned from private history" {
		t.Fatalf("reset history did not finish: %+v", doc)
	}
	// Clearing is a completed edit, so reads cannot start historical learning again.
	doc = getMemory(t, store, testScope)
	if _, err := store.Update(testScope, Update{Revision: &doc.Revision, Entries: &[]Entry{}}); err != nil {
		t.Fatal(err)
	}
	if doc, err := store.GetOrInitialize(testScope); err != nil || doc.Status != "idle" || len(doc.Entries) != 0 || !doc.Enabled {
		t.Fatalf("clear restarted history or disabled future memory: %+v, %v", doc, err)
	}
}

func TestResetRejectsConflictsAndMixedMutations(t *testing.T) {
	store := openTestStore(t, t.TempDir(), "local")
	doc := getMemory(t, store, testScope)
	content := "A manual memory"
	for _, request := range []Update{
		{Revision: &doc.Revision, Reset: true, Content: &content},
		{Revision: &doc.Revision, Reset: true, Entries: &[]Entry{}},
		{Revision: &doc.Revision, Reset: true, Retry: true},
		{Revision: &doc.Revision, Reset: true, Enabled: new(bool)},
	} {
		if _, err := store.Update(testScope, request); !errors.Is(err, ErrInvalid) {
			t.Fatalf("mixed reset accepted: %v", err)
		}
	}
	stale := doc.Revision + 1
	if _, err := store.Update(testScope, Update{Revision: &stale, Reset: true}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale reset accepted: %v", err)
	}
	off := false
	doc, err := store.Update(testScope, Update{Revision: &doc.Revision, Enabled: &off})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(testScope, Update{Revision: &doc.Revision, Reset: true}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("reset scheduled learning while disabled: %v", err)
	}
}
