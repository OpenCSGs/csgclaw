package im

import (
	"csgclaw/internal/apitypes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func unreadFixture(t *testing.T) (*Service, Room, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"dev", "qa"} {
		if _, _, err := s.EnsureAgentUser(EnsureAgentUserRequest{ID: id, Name: id, Role: "worker"}); err != nil {
			t.Fatal(err)
		}
	}
	room, err := s.CreateRoom(CreateRoomRequest{Title: "Unread", CreatorID: "admin", MemberIDs: []string{"dev", "qa"}, Type: apitypes.RoomTypeAutonomous})
	if err != nil {
		t.Fatal(err)
	}
	return s, room, path
}
func unreadSend(t *testing.T, s *Service, room, actor, text, thread string) Message {
	t.Helper()
	var relation *MessageRelation
	if thread != "" {
		relation = &MessageRelation{RelType: RelationTypeThread, EventID: thread}
	}
	m, err := s.CreateMessage(CreateMessageRequest{RoomID: room, SenderID: actor, Content: text, RelatesTo: relation})
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func unreadAdmit(t *testing.T, s *Service, room, source, actor string, want bool) apitypes.UnreadNotification {
	t.Helper()
	n, ok, err := s.AdmitAutonomousNotification(room, source, actor)
	if err != nil || ok != want {
		t.Fatalf("admission=%+v,%v,%v want %v", n, ok, err, want)
	}
	return n
}
func unreadRead(t *testing.T, s *Service, room, actor, thread string, limit int, peek bool) apitypes.UnreadPage {
	t.Helper()
	p, err := s.PrepareUnread(room, actor, thread, limit, peek)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func unreadAck(t *testing.T, s *Service, p apitypes.UnreadPage) {
	t.Helper()
	if err := s.AcknowledgeUnread(apitypes.UnreadAck{RoomID: p.RoomID, ParticipantID: p.ParticipantID, ThreadRootID: p.ThreadRootID, Token: p.Token}); err != nil {
		t.Fatal(err)
	}
}
func TestUnreadNotificationDurableAndNoTurnCeiling(t *testing.T) {
	s, room, path := unreadFixture(t)
	first := unreadSend(t, s, room.ID, "admin", "Build", "")
	unreadAdmit(t, s, room.ID, first.ID, "outsider", false)
	unreadAdmit(t, s, room.ID, first.ID, "dev", true)
	var err error
	s, err = NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	unreadAdmit(t, s, room.ID, first.ID, "pt-dev", false)
	unreadAdmit(t, s, room.ID, first.ID, "qa", true)
	// Silence leaves unread intact, but never repeats the same offered batch.
	p := unreadRead(t, s, room.ID, "dev", "", 50, true)
	if len(p.Messages) != 1 || p.Token != "" {
		t.Fatalf("peek=%+v", p)
	}
	unreadAdmit(t, s, room.ID, first.ID, "dev", false)
	for i := 0; i < 40; i++ {
		m := unreadSend(t, s, room.ID, "dev", fmt.Sprint(i), "")
		unreadAdmit(t, s, room.ID, m.ID, "qa", true)
	}
	if err := s.PauseAutonomousRoom(room.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	late := unreadSend(t, s, room.ID, "dev", "Late result", "")
	s, err = NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	unreadAdmit(t, s, room.ID, late.ID, "qa", false)
	next := unreadSend(t, s, room.ID, "admin", "Continue", "")
	unreadAdmit(t, s, room.ID, next.ID, "qa", true)
}
func TestUnreadPagesOnlyAdvanceOnMatchingAck(t *testing.T) {
	s, room, path := unreadFixture(t)
	first := unreadSend(t, s, room.ID, "admin", "One", "")
	unreadSend(t, s, room.ID, "admin", "Two", "")
	unreadAdmit(t, s, room.ID, first.ID, "dev", true)
	page := unreadRead(t, s, room.ID, "dev", "", 1, false)
	if len(page.Messages) != 1 || page.Messages[0].ID != first.ID || page.Token == "" || !page.HasMore || page.Messages[0].Sender.Role != "admin" {
		t.Fatalf("page=%+v", page)
	}
	unreadSend(t, s, room.ID, "qa", "Three", "")
	var err error
	s, err = NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	retry := unreadRead(t, s, room.ID, "pt-dev", "", 100, false)
	if retry.Token != page.Token || len(retry.Messages) != 1 || retry.Through != page.Through {
		t.Fatalf("retry moved pending page: %+v", retry)
	}
	for _, actor := range []string{"qa", "outsider"} {
		if err := s.AcknowledgeUnread(apitypes.UnreadAck{RoomID: room.ID, ParticipantID: actor, Token: page.Token}); err == nil {
			t.Fatal("accepted another member's token")
		}
	}
	if err := s.AcknowledgeUnread(apitypes.UnreadAck{RoomID: room.ID, ParticipantID: "dev", Token: "invented"}); err == nil {
		t.Fatal("accepted unknown token")
	}
	unreadAck(t, s, page)
	unreadAck(t, s, page)
	next := unreadRead(t, s, room.ID, "dev", "", 50, false)
	if len(next.Messages) != 2 || next.Messages[0].Content != "Two" || next.Messages[1].Content != "Three" {
		t.Fatalf("new page=%+v", next)
	}
	unreadAck(t, s, next)
	// A pending event whose body was already consumed during work needs no new turn.
	unreadAdmit(t, s, room.ID, first.ID, "dev", false)
	empty := unreadRead(t, s, room.ID, "dev", "", 50, false)
	if len(empty.Messages) != 0 || empty.Token != "" {
		t.Fatalf("empty=%+v", empty)
	}
}

func TestPendingAutonomousNotificationPreservesUnreadAndFallbackAdmission(t *testing.T) {
	s, room, _ := unreadFixture(t)
	root := unreadSend(t, s, room.ID, "admin", "Root", "")
	first := unreadSend(t, s, room.ID, "admin", "Start in the thread", root.ID)
	initial := unreadAdmit(t, s, room.ID, first.ID, "dev", true)
	unreadAck(t, s, unreadRead(t, s, room.ID, "dev", root.ID, 50, false))

	unreadSend(t, s, room.ID, "admin", "Unrelated main chat", "")
	unreadSend(t, s, room.ID, "dev", "Own message", root.ID)
	next := unreadSend(t, s, room.ID, "qa", "New work", root.ID)
	unreadSend(t, s, room.ID, "admin", "New constraint", root.ID)
	notice, ok := s.PendingAutonomousNotification(room.ID, "pt-dev", root.ID, initial.Through)
	if !ok || notice.Count != 2 || notice.Through <= initial.Through || notice.ThreadRootID != root.ID {
		t.Fatalf("wrong pending notice: %+v, %v", notice, ok)
	}
	if _, ok := s.PendingAutonomousNotification(room.ID, "dev", root.ID, notice.Through); ok {
		t.Fatal("same batch should not be offered twice within a turn")
	}
	if _, ok := s.PendingAutonomousNotification(room.ID, "outsider", root.ID, 0); ok {
		t.Fatal("offered unread to a non-member")
	}
	// If the model did not read the batch, its queued source can still wake it.
	if fallback := unreadAdmit(t, s, room.ID, next.ID, "dev", true); fallback.Through != notice.Through || fallback.Count != 2 {
		t.Fatalf("fallback lost unread messages: %+v", fallback)
	}

	next = unreadSend(t, s, room.ID, "admin", "Another update", root.ID)
	if _, ok := s.PendingAutonomousNotification(room.ID, "dev", root.ID, notice.Through); !ok {
		t.Fatal("new update was not available")
	}
	unreadAck(t, s, unreadRead(t, s, room.ID, "dev", root.ID, 50, false))
	unreadAdmit(t, s, room.ID, next.ID, "dev", false)
}
func TestUnreadThreadScopeAndPrivateActivity(t *testing.T) {
	s, room, _ := unreadFixture(t)
	root := unreadSend(t, s, room.ID, "admin", "Root", "")
	reply := unreadSend(t, s, room.ID, "admin", "Thread question", root.ID)
	if _, err := s.DeliverMessage(DeliverMessageRequest{RoomID: room.ID, SenderID: "qa", ThreadRootID: root.ID, Content: "Private result", Metadata: map[string]any{"csgclaw": map[string]any{"delivery_kind": "activity"}}}); err != nil {
		t.Fatal(err)
	}
	unreadSend(t, s, room.ID, "dev", "Own message", root.ID)
	unreadAdmit(t, s, room.ID, reply.ID, "dev", true)
	thread := unreadRead(t, s, room.ID, "dev", root.ID, 50, false)
	if len(thread.Messages) != 1 || thread.Messages[0].ID != reply.ID || thread.Root == nil || thread.Root.ID != root.ID {
		t.Fatalf("thread=%+v", thread)
	}
	if err := s.AcknowledgeUnread(apitypes.UnreadAck{RoomID: room.ID, ParticipantID: "dev", Token: thread.Token}); err == nil {
		t.Fatal("thread token acknowledged main chat")
	}
	unreadAck(t, s, thread)
	main := unreadRead(t, s, room.ID, "dev", "", 50, false)
	if len(main.Messages) != 1 || main.Messages[0].ID != root.ID {
		t.Fatalf("main cursor affected by thread: %+v", main)
	}
	if _, err := s.PrepareUnread(room.ID, "dev", "missing", 50, false); err == nil {
		t.Fatal("unknown thread allowed")
	}
	view, err := s.GetThread(room.ID, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Summary.ReplyCount != 2 {
		t.Fatalf("private activity counted as reply: %+v", view.Summary)
	}
	notice, err := s.DeliverMessage(DeliverMessageRequest{
		RoomID: room.ID, SenderID: "qa", ThreadRootID: root.ID, Content: "Claimed task-17",
		Metadata: map[string]any{"csgclaw": map[string]any{"delivery_kind": "autonomous_task_update"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.GetThread(room.ID, root.ID)
	if err != nil || view.Summary.ReplyCount != 2 || view.Summary.LatestReply == nil || view.Summary.LatestReply.ID == notice.ID {
		t.Fatalf("coordination notice leaked into thread summary: %+v, %v", view.Summary, err)
	}
	thread = unreadRead(t, s, room.ID, "dev", root.ID, 50, false)
	if len(thread.Messages) != 1 || thread.Messages[0].ID != notice.ID {
		t.Fatalf("coordination notice missing from agent unread messages: %+v", thread)
	}
}
func TestUnreadExpiredPageCanBeReadAgain(t *testing.T) {
	s, room, _ := unreadFixture(t)
	unreadSend(t, s, room.ID, "admin", "One", "")
	page := unreadRead(t, s, room.ID, "dev", "", 50, false)
	s.rooms[room.ID].AutonomousState.Cursors[autonomousCursorKey("dev", "")].Page.ExpiresAt = time.Now().Add(-time.Second)
	if err := s.AcknowledgeUnread(apitypes.UnreadAck{RoomID: room.ID, ParticipantID: "dev", Token: page.Token}); err == nil {
		t.Fatal("expired page accepted")
	}
	next := unreadRead(t, s, room.ID, "dev", "", 50, false)
	if next.Token == page.Token || len(next.Messages) != 1 {
		t.Fatalf("expired read lost data: %+v", next)
	}
	unreadAck(t, s, next)
}
func TestAutonomousBridgeOffersNotificationWithoutBody(t *testing.T) {
	room := Room{ID: "r", Type: apitypes.RoomTypeAutonomous}
	message := Message{ID: "m", Content: "secret body", Attachments: []MessageAttachment{{ID: "file"}}}
	event := messageEventForParticipant(room, User{ID: "admin", Role: "admin"}, message, "pt-dev")
	if !event.NotificationOnly || event.Text != "" || event.MemoryText != "" || len(event.Attachments) > 0 || event.ThreadContext != nil {
		t.Fatalf("body pushed: %+v", event)
	}
	message.Content = `<slash-command name="new" arg="conversation"></slash-command>`
	event = messageEventForParticipant(room, User{ID: "admin", Role: "admin"}, message, "pt-dev")
	if event.NotificationOnly || event.Text == "" {
		t.Fatalf("reset swallowed: %+v", event)
	}
	room.Type = apitypes.RoomTypeFree
	event = messageEventForParticipant(room, User{ID: "admin", Role: "admin"}, message, "pt-dev")
	if event.NotificationOnly || event.Text == "" {
		t.Fatal("legacy room changed")
	}
}

func TestUnreadPersistenceFailureDoesNotConsumeOrSuppressRetry(t *testing.T) {
	s, room, path := unreadFixture(t)
	first := unreadSend(t, s, room.ID, "admin", "One", "")
	session := filepath.Join(filepath.Dir(path), "sessions", room.ID+".jsonl")
	block := func() func() {
		t.Helper()
		if err := os.Rename(session, session+".backup"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(session, 0700); err != nil {
			t.Fatal(err)
		}
		return func() {
			t.Helper()
			if err := os.Remove(session); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(session+".backup", session); err != nil {
				t.Fatal(err)
			}
		}
	}
	unblock := block()
	if _, _, err := s.AdmitAutonomousNotification(room.ID, first.ID, "dev"); err == nil {
		t.Fatal("failed admission persisted")
	}
	unblock()
	unreadAdmit(t, s, room.ID, first.ID, "dev", true)
	unblock = block()
	if _, err := s.PrepareUnread(room.ID, "dev", "", 50, false); err == nil {
		t.Fatal("failed page persisted")
	}
	unblock()
	page := unreadRead(t, s, room.ID, "dev", "", 50, false)
	unblock = block()
	if err := s.AcknowledgeUnread(apitypes.UnreadAck{RoomID: room.ID, ParticipantID: "dev", Token: page.Token}); err == nil {
		t.Fatal("failed ack accepted")
	}
	unblock()
	retry := unreadRead(t, s, room.ID, "dev", "", 50, false)
	if retry.Token != page.Token || len(retry.Messages) != 1 {
		t.Fatal("failed ack consumed messages")
	}
	unreadAck(t, s, retry)
	if _, err := s.ClearRoomMessages(room.ID); err != nil {
		t.Fatal(err)
	}
	next := unreadSend(t, s, room.ID, "admin", "After clear", "")
	unreadAdmit(t, s, room.ID, next.ID, "dev", true)
	if page := unreadRead(t, s, room.ID, "dev", "", 50, false); len(page.Messages) != 1 || page.Messages[0].ID != next.ID {
		t.Fatalf("clear left stale cursor: %+v", page)
	}
}
func TestUnreadLegacyReceiptsAndPublicProjection(t *testing.T) {
	s, room, _ := unreadFixture(t)
	first := unreadSend(t, s, room.ID, "admin", "One", "")
	s.rooms[room.ID].AutonomousState = &apitypes.AutonomousState{SourceID: first.ID, Receipts: []string{first.ID + ":pt-dev"}}
	unreadAdmit(t, s, room.ID, first.ID, "dev", false)
	legacy, err := s.DeliverMessage(DeliverMessageRequest{RoomID: room.ID, SenderID: "qa", Content: "Public result", Metadata: map[string]any{"csgclaw": map[string]any{"delivery_kind": "final", "turn_progress": map[string]any{"items": []any{"private tool history"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	unreadAdmit(t, s, room.ID, legacy.ID, "dev", true)
	page := unreadRead(t, s, room.ID, "dev", "", 50, false)
	for _, message := range page.Messages {
		if metadata, ok := message.Metadata["csgclaw"].(map[string]any); ok && metadata["turn_progress"] != nil {
			t.Fatal("public read exposed private trace")
		}
	}
	stored, _ := s.Room(room.ID)
	last := stored.Messages[len(stored.Messages)-1]
	if last.Metadata["csgclaw"].(map[string]any)["turn_progress"] == nil {
		t.Fatal("public projection mutated stored progress")
	}
}

func TestAutonomousReplyDedupUsesActorScopeAndTurnStart(t *testing.T) {
	s, room, _ := unreadFixture(t)
	unreadSend(t, s, room.ID, "dev", "Done", "")
	human := unreadSend(t, s, room.ID, "admin", "Check again", "")
	startedAt := time.Now()
	if s.HasAutonomousReply(room.ID, "dev", "", "", "Done", startedAt) {
		t.Fatal("old answer suppressed new work")
	}
	unreadSend(t, s, room.ID, "qa", "Done", "")
	unreadSend(t, s, room.ID, "dev", "Done", human.ID)
	if s.HasAutonomousReply(room.ID, "dev", "", "", "Done", startedAt) {
		t.Fatal("different actor or scope suppressed answer")
	}
	if !s.HasAutonomousReply(room.ID, "dev", human.ID, "", "Done", startedAt) {
		t.Fatal("matching thread reply not found")
	}
	final := unreadSend(t, s, room.ID, "dev", "Done", "")
	if !s.HasAutonomousReply(room.ID, "dev", "", "", "Done", startedAt) {
		t.Fatal("duplicate not found")
	}
	if s.HasAutonomousReply(room.ID, "dev", "", final.ID, "Done", startedAt) {
		t.Fatal("retry deduplicated its own delivery")
	}
}
