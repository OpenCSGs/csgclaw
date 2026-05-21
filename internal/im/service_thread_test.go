package im

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStartThreadSnapshotsContextAndSummarizesRoot(t *testing.T) {
	svc := NewServiceFromBootstrap(threadTestBootstrap())

	view, created, err := svc.StartThread(StartThreadRequest{
		RoomID:        "room-1",
		RootMessageID: "msg-6",
	})
	if err != nil {
		t.Fatalf("StartThread() error = %v", err)
	}
	if !created {
		t.Fatal("StartThread() created = false, want true for first start")
	}
	if view.Root.ID != "msg-6" {
		t.Fatalf("thread root = %q, want msg-6", view.Root.ID)
	}
	if len(view.Context) != 8 {
		t.Fatalf("context len = %d, want 5 before + root + 2 after", len(view.Context))
	}
	if view.Context[0].ID != "msg-1" || view.Context[5].ID != "msg-6" || view.Context[7].ID != "msg-8" {
		t.Fatalf("context ids = %s ... %s, want msg-1 through msg-8", view.Context[0].ID, view.Context[len(view.Context)-1].ID)
	}
	if view.Summary.RootID != "msg-6" || view.Summary.ReplyCount != 0 {
		t.Fatalf("summary = %+v, want root msg-6 with no replies", view.Summary)
	}
	if view.Summary.Context.MessageCount != 8 || view.Summary.Context.BeforeCount != 5 || view.Summary.Context.AfterCount != 2 {
		t.Fatalf("context summary = %+v, want counts 8/5/2", view.Summary.Context)
	}
	if !strings.Contains(view.Summary.Context.RootExcerpt, "message 6") {
		t.Fatalf("root excerpt = %q, want root text", view.Summary.Context.RootExcerpt)
	}

	again, created, err := svc.StartThread(StartThreadRequest{RoomID: "room-1", RootMessageID: "msg-6"})
	if err != nil {
		t.Fatalf("StartThread() second error = %v", err)
	}
	if created {
		t.Fatal("StartThread() second created = true, want idempotent false")
	}
	if again.Root.ID != view.Root.ID || len(again.Context) != len(view.Context) {
		t.Fatalf("second view = %+v, want same root/context", again)
	}

	threads, err := svc.ListThreads("room-1", ThreadListOptions{Include: ThreadListIncludeAll})
	if err != nil {
		t.Fatalf("ListThreads() error = %v", err)
	}
	if len(threads.Threads) != 1 || threads.Threads[0].Root.ID != "msg-6" {
		t.Fatalf("ListThreads() = %+v, want one msg-6 thread", threads)
	}

	room, ok := svc.Room("room-1")
	if !ok {
		t.Fatal("Room(room-1) ok = false, want true")
	}
	found := false
	for _, message := range room.Messages {
		if message.ID == "msg-6" {
			found = true
			if message.Thread == nil || message.Thread.RootID != "msg-6" {
				t.Fatalf("root message thread = %+v, want summary", message.Thread)
			}
		}
	}
	if !found {
		t.Fatal("presented room missing root msg-6")
	}
}

func TestStartThreadRootExcerptStripsMarkdownFence(t *testing.T) {
	bootstrap := threadTestBootstrap()
	bootstrap.Rooms[0].Messages[2].Content = "```text\nthread title should be plain\n```"
	svc := NewServiceFromBootstrap(bootstrap)

	view, _, err := svc.StartThread(StartThreadRequest{RoomID: "room-1", RootMessageID: "msg-3"})
	if err != nil {
		t.Fatalf("StartThread() error = %v", err)
	}
	if view.Summary.Context.RootExcerpt != "thread title should be plain" {
		t.Fatalf("root excerpt = %q, want plain fenced body", view.Summary.Context.RootExcerpt)
	}
}

func TestCreateThreadReplyHidesFromMainTimelineAndUpdatesSummary(t *testing.T) {
	svc := NewServiceFromBootstrap(threadTestBootstrap())
	if _, _, err := svc.StartThread(StartThreadRequest{RoomID: "room-1", RootMessageID: "msg-3"}); err != nil {
		t.Fatalf("StartThread() error = %v", err)
	}

	reply, err := svc.CreateMessage(CreateMessageRequest{
		RoomID:   "room-1",
		SenderID: "u-manager",
		Content:  "reply inside thread",
		RelatesTo: &MessageRelation{
			RelType: RelationTypeThread,
			EventID: "msg-3",
		},
	})
	if err != nil {
		t.Fatalf("CreateMessage(thread reply) error = %v", err)
	}
	if reply.RelatesTo == nil || reply.RelatesTo.RelType != RelationTypeThread || reply.RelatesTo.EventID != "msg-3" {
		t.Fatalf("reply.RelatesTo = %+v, want m.thread -> msg-3", reply.RelatesTo)
	}

	timeline, err := svc.ListMessages("room-1")
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	for _, message := range timeline {
		if message.ID == reply.ID {
			t.Fatalf("ListMessages() included thread reply %+v, want top-level only", message)
		}
	}

	all, err := svc.ListMessagesWithOptions("room-1", ListMessagesOptions{IncludeThreadReplies: true})
	if err != nil {
		t.Fatalf("ListMessagesWithOptions(include replies) error = %v", err)
	}
	if !containsMessageID(all, reply.ID) {
		t.Fatalf("ListMessagesWithOptions(include replies) = %+v, want reply %s", all, reply.ID)
	}

	view, err := svc.GetThread("room-1", "msg-3")
	if err != nil {
		t.Fatalf("GetThread() error = %v", err)
	}
	if len(view.Replies) != 1 || view.Replies[0].ID != reply.ID {
		t.Fatalf("thread replies = %+v, want reply %s", view.Replies, reply.ID)
	}
	if view.Summary.ReplyCount != 1 || view.Summary.LatestReply == nil || view.Summary.LatestReply.ID != reply.ID {
		t.Fatalf("thread summary = %+v, want latest reply %s", view.Summary, reply.ID)
	}
	if !view.Summary.CurrentUserParticipated {
		t.Fatal("CurrentUserParticipated = false, want true because u-admin owns the root")
	}
}

func TestStartThreadContextSnapshotRespectsPayloadCap(t *testing.T) {
	bootstrap := threadTestBootstrap()
	large := strings.Repeat("large-context ", 1800)
	for idx := range bootstrap.Rooms[0].Messages {
		bootstrap.Rooms[0].Messages[idx].Content = large
	}
	svc := NewServiceFromBootstrap(bootstrap)

	view, _, err := svc.StartThread(StartThreadRequest{RoomID: "room-1", RootMessageID: "msg-4"})
	if err != nil {
		t.Fatalf("StartThread() error = %v", err)
	}
	if len(view.Context) == 0 {
		t.Fatal("context is empty, want at least the root message")
	}
	if len(view.Context) >= 8 {
		t.Fatalf("context len = %d, want payload cap to trim surrounding messages", len(view.Context))
	}
	if !containsMessageID(view.Context, "msg-4") {
		t.Fatalf("context ids = %+v, want root msg-4 retained", view.Context)
	}
	if view.Summary.Context.MessageCount != len(view.Context) {
		t.Fatalf("summary message count = %d, want %d", view.Summary.Context.MessageCount, len(view.Context))
	}
}

func TestStartThreadRejectsMissingAndNestedRoots(t *testing.T) {
	svc := NewServiceFromBootstrap(threadTestBootstrap())
	if _, _, err := svc.StartThread(StartThreadRequest{RoomID: "room-1", RootMessageID: "missing"}); err == nil {
		t.Fatal("StartThread(missing) error = nil, want error")
	}
	if _, _, err := svc.StartThread(StartThreadRequest{RoomID: "room-1", RootMessageID: "msg-2"}); err != nil {
		t.Fatalf("StartThread(root) error = %v", err)
	}
	reply, err := svc.CreateMessage(CreateMessageRequest{
		RoomID:   "room-1",
		SenderID: "u-manager",
		Content:  "nested candidate",
		RelatesTo: &MessageRelation{
			RelType: RelationTypeThread,
			EventID: "msg-2",
		},
	})
	if err != nil {
		t.Fatalf("CreateMessage(thread reply) error = %v", err)
	}
	if _, _, err := svc.StartThread(StartThreadRequest{RoomID: "room-1", RootMessageID: reply.ID}); err == nil {
		t.Fatal("StartThread(nested reply) error = nil, want nested thread rejection")
	}
}

func TestThreadStatePersistsAcrossReload(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	if err := SaveBootstrap(statePath, threadTestBootstrap()); err != nil {
		t.Fatalf("SaveBootstrap() error = %v", err)
	}
	svc, err := NewServiceFromPath(statePath)
	if err != nil {
		t.Fatalf("NewServiceFromPath() error = %v", err)
	}
	if _, _, err := svc.StartThread(StartThreadRequest{RoomID: "room-1", RootMessageID: "msg-4"}); err != nil {
		t.Fatalf("StartThread() error = %v", err)
	}
	if _, err := svc.CreateMessage(CreateMessageRequest{
		RoomID:   "room-1",
		SenderID: "u-manager",
		Content:  "persisted reply",
		RelatesTo: &MessageRelation{
			RelType: RelationTypeThread,
			EventID: "msg-4",
		},
	}); err != nil {
		t.Fatalf("CreateMessage(thread reply) error = %v", err)
	}

	reloaded, err := NewServiceFromPath(statePath)
	if err != nil {
		t.Fatalf("NewServiceFromPath(reload) error = %v", err)
	}
	view, err := reloaded.GetThread("room-1", "msg-4")
	if err != nil {
		t.Fatalf("GetThread(reloaded) error = %v", err)
	}
	if len(view.Context) != 6 {
		t.Fatalf("reloaded context len = %d, want 3 before + root + 2 after", len(view.Context))
	}
	if len(view.Replies) != 1 || view.Replies[0].Content != "persisted reply" {
		t.Fatalf("reloaded replies = %+v, want persisted reply", view.Replies)
	}
	if view.Summary.ReplyCount != 1 {
		t.Fatalf("reloaded summary = %+v, want one reply", view.Summary)
	}
}

func TestDeleteUserRebuildsThreadStateFromSurvivingMessages(t *testing.T) {
	base := time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC)
	svc := NewServiceFromBootstrap(Bootstrap{
		CurrentUserID: "u-admin",
		Users: []User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-alice", Name: "Alice", Handle: "alice"},
			{ID: "u-bob", Name: "Bob", Handle: "bob"},
		},
		Rooms: []Room{{
			ID:      "room-1",
			Title:   "Room One",
			Members: []string{"u-admin", "u-alice", "u-bob"},
			Messages: []Message{
				{ID: "msg-before", SenderID: "u-admin", Content: "before", CreatedAt: base},
				{ID: "msg-delete-context", SenderID: "u-alice", Content: "delete from context", CreatedAt: base.Add(time.Minute)},
				{ID: "msg-root", SenderID: "u-admin", Content: "root survives", CreatedAt: base.Add(2 * time.Minute)},
				{ID: "msg-after", SenderID: "u-bob", Content: "after", CreatedAt: base.Add(3 * time.Minute)},
				{ID: "msg-delete-root", SenderID: "u-alice", Content: "delete root", CreatedAt: base.Add(4 * time.Minute)},
			},
		}},
	})
	if _, _, err := svc.StartThread(StartThreadRequest{RoomID: "room-1", RootMessageID: "msg-root"}); err != nil {
		t.Fatalf("StartThread(surviving root) error = %v", err)
	}
	if _, _, err := svc.StartThread(StartThreadRequest{RoomID: "room-1", RootMessageID: "msg-delete-root"}); err != nil {
		t.Fatalf("StartThread(deleted root) error = %v", err)
	}
	if _, err := svc.CreateMessage(CreateMessageRequest{
		RoomID:   "room-1",
		SenderID: "u-alice",
		Content:  "deleted reply",
		RelatesTo: &MessageRelation{
			RelType: RelationTypeThread,
			EventID: "msg-root",
		},
	}); err != nil {
		t.Fatalf("CreateMessage(thread reply) error = %v", err)
	}

	if err := svc.DeleteUser("u-alice"); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}

	view, err := svc.GetThread("room-1", "msg-root")
	if err != nil {
		t.Fatalf("GetThread(surviving root) error = %v", err)
	}
	if view.Summary.ReplyCount != 0 || len(view.Replies) != 0 {
		t.Fatalf("thread replies = %+v summary=%+v, want deleted user's reply removed", view.Replies, view.Summary)
	}
	for _, message := range view.Context {
		if message.SenderID == "u-alice" {
			t.Fatalf("thread context = %+v, want deleted user's messages pruned", view.Context)
		}
	}
	if view.Summary.Context.MessageCount != len(view.Context) {
		t.Fatalf("context summary = %+v, want message_count %d", view.Summary.Context, len(view.Context))
	}
	if _, err := svc.GetThread("room-1", "msg-delete-root"); err == nil {
		t.Fatal("GetThread(deleted root) error = nil, want thread state pruned")
	}
}

func threadTestBootstrap() Bootstrap {
	base := time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC)
	messages := make([]Message, 0, 8)
	for i := 1; i <= 8; i++ {
		messages = append(messages, Message{
			ID:        "msg-" + string(rune('0'+i)),
			SenderID:  "u-admin",
			Kind:      MessageKindMessage,
			Content:   "message " + string(rune('0'+i)),
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		})
	}
	return Bootstrap{
		CurrentUserID: "u-admin",
		Users: []User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-manager", Name: "manager", Handle: "manager"},
		},
		Rooms: []Room{
			{ID: "room-1", Title: "Room One", Members: []string{"u-admin", "u-manager"}, Messages: messages},
		},
	}
}

func containsMessageID(messages []Message, id string) bool {
	for _, message := range messages {
		if message.ID == id {
			return true
		}
	}
	return false
}
