package agentmemory

import (
	"context"
	"testing"
	"time"
)

func TestForgetKeepsHistoryCursorAndUnrelatedWork(t *testing.T) {
	store := openTestStore(t, t.TempDir(), "local")
	if _, err := store.GetOrInitialize(testScope); err != nil {
		t.Fatal(err)
	}
	current, err := store.load(testScope)
	if err != nil {
		t.Fatal(err)
	}
	cursor := HistoryCursor{At: time.Unix(1, 0), RoomID: "private", MessageID: "previous"}
	current.Jobs[0].HistoryCursor = cursor
	if err := store.commit(current); err != nil {
		t.Fatal(err)
	}
	enqueueMemory(t, store, "B", "Keep unrelated queued message")
	before, _ := store.next(testScope, time.Now())
	forgotten := Source{"private", "A"}
	if _, err := store.Forget(testScope, forgotten); err != nil {
		t.Fatal(err)
	}
	after, err := store.load(testScope)
	if err != nil {
		t.Fatal(err)
	}
	if after.Initialization != "pending" || len(after.Jobs) != 2 || after.Jobs[0].HistoryCursor.Compare(cursor) != 0 || after.Jobs[0].HistoryCursor.Offset != cursor.Offset {
		t.Fatalf("history or queue was discarded: %+v", after)
	}
	// A model result already in flight must not resurrect the selected source.
	if err := store.finish(before, before.Jobs[0], []Message{{Source: forgotten, Text: "Forget me"}}, []Change{{Kind: "fact", Text: "stale A", SourceMessageID: "A"}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	worker := Worker{Store: store, History: func(_ context.Context, _ Scope, _, _ time.Time, c HistoryCursor) ([]HistoryMessage, error) {
		if c.MessageID == "C" {
			return nil, nil
		}
		return []HistoryMessage{{At: time.Unix(2, 0), Message: Message{Source: forgotten, Text: "Forgotten A"}}, {At: time.Unix(3, 0), Message: Message{Source: Source{"private", "C"}, Text: "Keep historical C"}}}, nil
	}, Extract: func(_ context.Context, _ string, input Extraction) ([]Change, error) {
		var changes []Change
		for _, m := range input.Messages {
			if m.Source == forgotten {
				t.Fatal("forgotten source sent to model")
			}
			changes = append(changes, Change{Kind: "fact", Text: m.Text, SourceMessageID: m.MessageID})
		}
		return changes, nil
	}}
	for range 3 {
		worker.process(context.Background(), testScope)
	}
	doc := getMemory(t, store, testScope)
	if doc.Initialization != "complete" || doc.Status != "idle" || len(doc.Entries) != 2 {
		t.Fatalf("unrelated work did not survive: %+v", doc)
	}
	// The exclusion survives restart and rejects a late enqueue with an old ticket.
	restarted := openTestStore(t, store.root, "local")
	ticket, _, _, _ := restarted.Snapshot(testScope, "")
	if err := restarted.Enqueue(ticket, "late-A", Message{Source: forgotten, Text: "Late A"}); err != nil {
		t.Fatal(err)
	}
	if getMemory(t, restarted, testScope).Status != "idle" {
		t.Fatal("late enqueue revived forgotten work")
	}
}
