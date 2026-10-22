package im

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestUnchangedWriteRepairsPermissionsByAtomicReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows implements only the read-only attribute")
	}
	path := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(path, []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFileIfChanged(path, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) || after.Mode().Perm() != 0o600 {
		t.Fatalf("permission repair did not use a private atomic replacement: %v", after.Mode())
	}
}

func TestUnchangedWritePreservesReadOnlyReplacementBehavior(t *testing.T) {
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "atomic"), filepath.Join(dir, "deduplicated")}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("saved"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o400); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(path, 0o600)
	}
	wantErr := atomicWriteFile(paths[0], []byte("saved"), 0o600)
	gotErr := writeFileIfChanged(paths[1], []byte("saved"), 0o600)
	if (wantErr == nil) != (gotErr == nil) {
		t.Fatalf("read-only replacement behavior changed: atomic=%v, deduplicated=%v", wantErr, gotErr)
	}
	wantInfo, err := os.Stat(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	gotInfo, err := os.Stat(paths[1])
	if err != nil {
		t.Fatal(err)
	}
	if wantInfo.Mode().Perm() != gotInfo.Mode().Perm() {
		t.Fatalf("read-only attributes changed: atomic=%v, deduplicated=%v", wantInfo.Mode(), gotInfo.Mode())
	}
}

func TestMessageDeliveryLeavesUnrelatedSessionUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := Bootstrap{CurrentUserID: AdminUserID, Rooms: []Room{
		{ID: "active", Members: []string{AdminUserID}},
		{ID: "history", Members: []string{AdminUserID}, Messages: []Message{{ID: "saved", SenderID: AdminUserID, Content: "History", CreatedAt: time.Now().UTC()}}},
	}}
	if err := SaveBootstrap(path, state); err != nil {
		t.Fatal(err)
	}
	svc, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	history := filepath.Join(filepath.Dir(path), "sessions", "history.jsonl")
	before, err := os.Stat(history)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeliverMessage(DeliverMessageRequest{RoomID: "active", SenderID: AdminUserID, MessageID: "result", Content: "Delivered"}); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(history)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("a message in another room replaced the committed history file")
	}
	loaded, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := loaded.ListMessages("active")
	if err != nil || len(messages) != 1 || messages[0].Content != "Delivered" {
		t.Fatalf("delivery did not survive restart: %+v, %v", messages, err)
	}
}

func TestUnchangedWriteRepairsExternalEditsAndDoesNotFollowSymlinks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	if err := writeFileIfChanged(path, []byte("correct"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeFileIfChanged(path, []byte("correct"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "correct" {
		t.Fatalf("same-size external edit was not repaired: %q, %v", data, err)
	}
	outside := filepath.Join(dir, "outside")
	if err := os.WriteFile(outside, []byte("correct"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := writeFileIfChanged(link, []byte("correct"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("unchanged write retained a symlink: %v, %v", info, err)
	}
}

func TestRepeatedLargeSessionSaveKeepsBlobAndRepairsCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "room.jsonl")
	message := Message{ID: "large", SenderID: AdminUserID, Content: strings.Repeat("X", 100*1024), CreatedAt: time.Now().UTC()}
	if err := saveMessagesJSONL(path, "room", []Message{message}); err != nil {
		t.Fatal(err)
	}
	blobs, err := filepath.Glob(filepath.Join(filepath.Dir(path), "blobs", "room", "*.json"))
	if err != nil || len(blobs) != 1 {
		t.Fatalf("expected a large-message blob: %v, %v", blobs, err)
	}
	before, err := os.Stat(blobs[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := saveMessagesJSONL(path, "room", []Message{message}); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(blobs[0])
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("unchanged content-addressed blob replaced: %v", err)
	}
	if err := os.WriteFile(blobs[0], []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := saveMessagesJSONL(path, "room", []Message{message}); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadMessagesJSONL(path, "room")
	if err != nil || len(loaded) != 1 || loaded[0].Content != message.Content {
		t.Fatalf("corrupt blob not repaired: %v", err)
	}
}

func TestConcurrentRoomDeliveryAndReloadSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := Bootstrap{CurrentUserID: AdminUserID, Rooms: []Room{
		{ID: "a", Members: []string{AdminUserID}}, {ID: "b", Members: []string{AdminUserID}},
	}}
	if err := SaveBootstrap(path, state); err != nil {
		t.Fatal(err)
	}
	svc, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, room := range []string{"a", "b"} {
		wg.Add(1)
		go func(room string) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				_, err := svc.DeliverMessage(DeliverMessageRequest{RoomID: room, SenderID: AdminUserID, MessageID: fmt.Sprint("message-", i), Content: fmt.Sprint(i)})
				if err != nil {
					t.Error(err)
					return
				}
				if err := svc.Reload(); err != nil {
					t.Error(err)
					return
				}
			}
		}(room)
	}
	wg.Wait()
	svc, err = NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, room := range []string{"a", "b"} {
		messages, err := svc.ListMessages(room)
		if err != nil || len(messages) != 20 {
			t.Fatalf("room %s lost concurrent updates: %d, %v", room, len(messages), err)
		}
		for i, message := range messages {
			if message.ID != fmt.Sprint("message-", i) || message.Content != fmt.Sprint(i) {
				t.Fatalf("room %s changed message order at %d: %+v", room, i, message)
			}
		}
	}
}

func TestOtherRoomCheckpointDoesNotLeaveDanglingAttachmentReferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := SaveBootstrap(path, Bootstrap{Rooms: []Room{
		{ID: "a", Members: []string{AdminUserID}}, {ID: "b", Members: []string{AdminUserID}},
	}}); err != nil {
		t.Fatal(err)
	}
	svc, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeliverMessage(DeliverMessageRequest{RoomID: "a", SenderID: AdminUserID, MessageID: "result", Content: "Saved result", Attachments: []MessageAttachmentUpload{{Name: "kept.txt", Data: []byte("saved bytes")}}}); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{"csgclaw": map[string]any{"delivery_kind": "activity", "turn_progress": map[string]any{"id": "turn", "revision": float64(1), "status": "running"}}}
	if _, err := svc.DeliverMessage(DeliverMessageRequest{RoomID: "a", SenderID: AdminUserID, MessageID: "result", Content: "Live projection", Metadata: metadata, Transient: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeliverMessage(DeliverMessageRequest{RoomID: "b", SenderID: AdminUserID, Content: "Another room checkpoint"}); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, room := range restarted.Bootstrap().Rooms {
		for _, message := range room.Messages {
			for _, attachment := range message.Attachments {
				if _, err := restarted.AttachmentFile(attachment.ID); err != nil {
					t.Fatalf("checkpoint in another room removed bytes still referenced on disk: %v", err)
				}
			}
		}
	}
}

func TestFailedDeliveryDoesNotPublishAndCanRetry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := SaveBootstrap(path, Bootstrap{Rooms: []Room{{ID: "room", Members: []string{AdminUserID}}}}); err != nil {
		t.Fatal(err)
	}
	bus := NewBus()
	svc, err := NewServiceFromPathWithBus(path, bus)
	if err != nil {
		t.Fatal(err)
	}
	events, cancel := bus.Subscribe()
	defer cancel()
	session := filepath.Join(filepath.Dir(path), "sessions", "room.jsonl")
	if err := os.Rename(session, session+".backup"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(session, 0o700); err != nil {
		t.Fatal(err)
	}
	req := DeliverMessageRequest{RoomID: "room", SenderID: AdminUserID, MessageID: "result", Content: "retry result"}
	if _, err := svc.DeliverMessage(req); err == nil {
		t.Fatal("persistence failure was swallowed")
	}
	select {
	case <-events:
		t.Fatal("failed delivery was published")
	default:
	}
	if err := os.Remove(session); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(session+".backup", session); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeliverMessage(req); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := restarted.ListMessages("room")
	if err != nil || len(messages) != 1 || messages[0].Content != req.Content {
		t.Fatalf("retry lost or duplicated a message: %+v, %v", messages, err)
	}
}

func TestDeliveryKeepsThreadContextAndAttachmentAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := SaveBootstrap(path, threadTestBootstrap()); err != nil {
		t.Fatal(err)
	}
	svc, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := svc.DeliverMessage(DeliverMessageRequest{RoomID: "room-1", SenderID: ManagerUserID, MessageID: "reply", ThreadRootID: "msg-4", Content: "Reply with file", Attachments: []MessageAttachmentUpload{{Name: "result.txt", MediaType: "text/plain", Data: []byte("attachment contents")}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeliverMessage(DeliverMessageRequest{RoomID: "room-1", SenderID: AdminUserID, MessageID: "msg-4", Content: "Updated root"}); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := restarted.GetThread("room-1", "msg-4")
	if err != nil || len(thread.Replies) != 1 || thread.Replies[0].ID != reply.ID || thread.Summary.Context.RootExcerpt != "Updated root" {
		t.Fatalf("thread update did not survive restart: %+v, %v", thread, err)
	}
	found := false
	for _, message := range thread.Context {
		if message.ID == "msg-4" && message.Content == "Updated root" {
			found = true
		}
	}
	if !found {
		t.Fatal("out-of-line thread context kept a stale root")
	}
	attachment, err := restarted.AttachmentFile(reply.Attachments[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(attachment.Path)
	if err != nil || string(data) != "attachment contents" {
		t.Fatalf("attachment bytes changed after restart: %q, %v", data, err)
	}
	if _, err := restarted.ClearRoomMessages("room-1"); err != nil {
		t.Fatal(err)
	}
	cleared, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	room, ok := cleared.Room("room-1")
	if !ok || len(room.Messages) != 0 || len(room.Threads) != 0 {
		t.Fatalf("cleared messages or threads reappeared after restart: %+v", room)
	}
	if _, err := cleared.AttachmentFile(reply.Attachments[0].ID); err == nil {
		t.Fatal("cleared attachment remained accessible")
	}
}

func TestStableBootstrapIndexPreservesRoomActivityOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	older := time.Now().UTC().Add(-time.Hour)
	state := Bootstrap{Rooms: []Room{
		{ID: "a", Members: []string{AdminUserID}, Messages: []Message{{ID: "old", SenderID: AdminUserID, Content: "Older", CreatedAt: older}}},
		{ID: "b", Members: []string{AdminUserID}, Messages: []Message{{ID: "recent", SenderID: AdminUserID, Content: "Newer", CreatedAt: older.Add(time.Minute)}}},
	}}
	if err := SaveBootstrap(path, state); err != nil {
		t.Fatal(err)
	}
	svc, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if rooms := svc.ListRooms(); rooms[0].ID != "b" {
		t.Fatal("saved index order replaced activity order")
	}
	if _, err := svc.DeliverMessage(DeliverMessageRequest{RoomID: "a", SenderID: AdminUserID, Content: "Now newest"}); err != nil {
		t.Fatal(err)
	}
	svc, err = NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if rooms := svc.ListRooms(); rooms[0].ID != "a" {
		t.Fatal("new activity order did not survive restart")
	}
}

func TestRepeatedBootstrapSaveKeepsCommittedFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := Bootstrap{CurrentUserID: AdminUserID, Rooms: []Room{{ID: "room", Members: []string{AdminUserID}}}}
	if err := SaveBootstrap(path, state); err != nil {
		t.Fatal(err)
	}
	files := []string{path, filepath.Join(filepath.Dir(path), "sessions", "room.jsonl")}
	before := make([]os.FileInfo, len(files))
	for i, name := range files {
		var err error
		before[i], err = os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := SaveBootstrap(path, state); err != nil {
		t.Fatal(err)
	}
	for i, name := range files {
		after, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(before[i], after) {
			t.Errorf("saving unchanged data replaced %s", name)
		}
	}
}
