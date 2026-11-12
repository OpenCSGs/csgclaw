package agentmemory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func historyFixture(count int) []HistoryMessage {
	var messages []HistoryMessage
	for index := range count {
		messages = append(messages, HistoryMessage{At: time.Unix(int64(index+1), 0),
			Message: Message{Source: Source{"private", fmt.Sprintf("history-%03d", index)}, Text: fmt.Sprintf("History text %d", index)}})
	}
	return messages
}

func historyReader(messages []HistoryMessage) func(context.Context, Scope, time.Time, time.Time, HistoryCursor) ([]HistoryMessage, error) {
	return func(_ context.Context, _ Scope, after, through time.Time, cursor HistoryCursor) ([]HistoryMessage, error) {
		var page []HistoryMessage
		for _, message := range messages {
			order := message.Cursor().Compare(cursor)
			if !message.At.After(after) || message.At.After(through) || order < 0 || order == 0 && cursor.Offset == 0 {
				continue
			}
			page = append(page, message)
			if len(page) == HistoryPageMessages {
				break
			}
		}
		return page, nil
	}
}

func TestHistorySegmentsRetainFullLongUnicodeTextAndRedactBeforeSplitting(t *testing.T) {
	history := historyFixture(3)
	history[0].Text = strings.Repeat("历史文字", 3000) + " password=fixture-secret " + strings.Repeat("tail ", 6000) + "END_PREFERENCE"
	history[1].Text = "\x00 "
	reader := historyReader(history)
	var cursor HistoryCursor
	parts := make(map[string]string)
	var calls int
	for {
		page, _ := reader(t.Context(), testScope, time.Time{}, time.Now(), cursor)
		if len(page) == 0 {
			break
		}
		messages, next := historySegment(page, cursor)
		calls++
		if calls > 10 {
			t.Fatal("history scan did not finish")
		}
		size := 0
		for _, message := range messages {
			if !utf8.ValidString(message.Text) || strings.Contains(message.Text, "fixture-secret") {
				t.Fatal("unsafe or invalid text reached extraction")
			}
			parts[message.MessageID] += message.Text
			size += len(message.Text)
		}
		if size > 32<<10 {
			t.Fatal("unbounded history segment")
		}
		cursor = next
	}
	if calls < 2 {
		t.Fatal("long text was not segmented")
	}
	for _, message := range history {
		if parts[message.MessageID] != cleanText(message.Text) {
			t.Fatalf("message tail was lost: %s", message.MessageID)
		}
	}
}

func TestHistoryLearningAccumulatesEveryPageAndNewerCorrections(t *testing.T) {
	store := openTestStore(t, t.TempDir(), "local")
	history := historyFixture(245)
	history[4].Text = strings.Repeat("long context ", 6000) + "LONG_TAIL_PREFERENCE"
	enableMemory(t, store, testScope)
	seen := make(map[string]string)
	var calls int
	worker := Worker{Store: store, History: historyReader(history), Extract: func(_ context.Context, _ string, input Extraction) ([]Change, error) {
		calls++
		if !input.History {
			t.Error("history segment not identified")
		}
		for _, message := range input.Messages {
			seen[message.MessageID] += message.Text
		}
		var changes []Change
		for _, message := range input.Messages {
			switch message.MessageID {
			case "history-000":
				changes = append(changes, Change{Kind: "preference", Text: "Brief reports", SourceMessageID: message.MessageID})
			case "history-100":
				if len(input.Entries) == 0 {
					t.Error("earlier segment's memory was not accumulated")
				}
				changes = append(changes, Change{Kind: "convention", Text: "Verify the completed work", SourceMessageID: message.MessageID})
			case "history-244":
				for _, entry := range input.Entries {
					if entry.Kind == "preference" {
						changes = append(changes, Change{EntryID: entry.ID, Kind: "preference", Text: "Detailed reports", SourceMessageID: message.MessageID})
					}
				}
			}
		}
		return changes, nil
	}}
	for range 20 {
		if getMemory(t, store, testScope).Status != "pending" {
			break
		}
		worker.process(t.Context(), testScope)
	}
	doc := getMemory(t, store, testScope)
	if doc.Status != "idle" || doc.Initialization != "complete" || len(doc.Entries) != 2 || calls < 8 || len(seen) != len(history) || !strings.Contains(doc.Content, "Detailed reports") {
		t.Fatalf("incomplete history learning: calls=%d sources=%d doc=%+v", calls, len(seen), doc)
	}
	for _, message := range history {
		if seen[message.MessageID] != cleanText(message.Text) {
			t.Fatalf("incomplete source %s", message.MessageID)
		}
	}
}

func TestHistoryPageFailureRetriesWithoutAdvancingOrLosingEarlierMemory(t *testing.T) {
	store := openTestStore(t, t.TempDir(), "local")
	enableMemory(t, store, testScope)
	var fail bool
	worker := Worker{Store: store, History: historyReader(historyFixture(40)), Extract: func(_ context.Context, _ string, input Extraction) ([]Change, error) {
		if fail {
			return nil, errors.New("temporary extraction failure")
		}
		return []Change{{Kind: "fact", Text: input.Messages[0].Text, SourceMessageID: input.Messages[0].MessageID}}, nil
	}}
	worker.process(t.Context(), testScope)
	before, err := store.load(testScope)
	if err != nil {
		t.Fatal(err)
	}
	fail = true
	worker.process(t.Context(), testScope)
	failed, err := store.load(testScope)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed.Entries) != 1 || failed.Jobs[0].HistoryCursor != before.Jobs[0].HistoryCursor || failed.LastError == nil {
		t.Fatal("failed page advanced or lost prior notes")
	}
	failed.Jobs[0].NextAttempt = time.Now()
	if err := store.commit(failed); err != nil {
		t.Fatal(err)
	}
	fail = false
	worker.process(t.Context(), testScope)
	worker.process(t.Context(), testScope)
	if doc := getMemory(t, store, testScope); doc.Status != "idle" || doc.LastError != nil || len(doc.Entries) != 2 {
		t.Fatalf("page retry failed: %+v", doc)
	}
}

func TestHistoryAbnormalExitResumesCommittedCursorAndKeepsTurnsSeparate(t *testing.T) {
	if root := os.Getenv("CSGCLAW_TEST_HISTORY_CRASH_ROOT"); root != "" {
		store := openTestStore(t, root, "local")
		enableMemory(t, store, testScope)
		worker := Worker{Store: store, History: historyReader(historyFixture(70)), Extract: func(ctx context.Context, _ string, input Extraction) ([]Change, error) {
			if input.Messages[0].MessageID == "history-032" {
				if err := os.WriteFile(filepath.Join(root, "history-started"), nil, 0600); err != nil {
					return nil, err
				}
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return []Change{{Kind: "fact", Text: input.Messages[0].Text, SourceMessageID: input.Messages[0].MessageID}}, nil
		}}
		worker.Run(context.Background())
		return
	}
	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^TestHistoryAbnormalExitResumesCommittedCursorAndKeepsTurnsSeparate$")
	child.Env = append(os.Environ(), "CSGCLAW_TEST_HISTORY_CRASH_ROOT="+root)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "history-started")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("history child did not reach second segment")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	store := openTestStore(t, root, "local")
	current, err := store.load(testScope)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Entries) != 1 || current.Jobs[0].HistoryCursor.MessageID != "history-031" {
		t.Fatal("committed history progress was lost")
	}
	enqueueMemory(t, store, "turn-a", "A separate live turn")
	enqueueMemory(t, store, "turn-b", "Another separate live turn")
	var turns int
	worker := Worker{Store: store, History: historyReader(historyFixture(70)), Extract: func(_ context.Context, _ string, input Extraction) ([]Change, error) {
		if input.History {
			for _, message := range input.Messages {
				if message.MessageID == "history-000" {
					t.Error("completed page replayed after crash")
				}
			}
		} else {
			turns++
			if len(input.Messages) != 1 {
				t.Error("daily turns were batched")
			}
		}
		return []Change{{Kind: "fact", Text: input.Messages[0].Text, SourceMessageID: input.Messages[0].MessageID}}, nil
	}}
	for range 8 {
		if getMemory(t, store, testScope).Status != "pending" {
			break
		}
		worker.process(t.Context(), testScope)
	}
	if doc := getMemory(t, store, testScope); doc.Status != "idle" || doc.Initialization != "complete" || len(doc.Entries) != 5 || turns != 2 {
		t.Fatalf("history recovery or live turns failed: turns=%d doc=%+v", turns, doc)
	}
}

func TestHistoryClearDuringPageCannotRestoreDeletedMemory(t *testing.T) {
	store := openTestStore(t, t.TempDir(), "local")
	enableMemory(t, store, testScope)
	worker := Worker{Store: store, History: historyReader(historyFixture(40)), Extract: func(_ context.Context, _ string, input Extraction) ([]Change, error) {
		return []Change{{Kind: "preference", Text: "Old preference", SourceMessageID: input.Messages[0].MessageID}}, nil
	}}
	worker.process(t.Context(), testScope)
	before, ok := store.next(testScope, time.Now())
	if !ok {
		t.Fatal("history continuation missing")
	}
	task := before.Jobs[0]
	doc := getMemory(t, store, testScope)
	if _, err := store.Update(testScope, Update{Revision: &doc.Revision, Entries: &[]Entry{}}); err != nil {
		t.Fatal(err)
	}
	next := HistoryCursor{At: time.Unix(40, 0), RoomID: "private", MessageID: "history-039"}
	if err := store.finish(before, task, []Message{{Source: Source{"private", "history-032"}, Text: "Old text"}},
		[]Change{{Kind: "preference", Text: "Must not return", SourceMessageID: "history-032"}}, nil, &next); err != nil {
		t.Fatal(err)
	}
	if doc, err := store.GetOrInitialize(testScope); err != nil || len(doc.Entries) != 0 || doc.Status != "idle" || doc.Initialization != "complete" {
		t.Fatalf("cleared history was restored: %+v, %v", doc, err)
	}
}
