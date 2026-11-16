package im

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agentmemory"
	"csgclaw/internal/apitypes"
)

func TestMemoryHistoryIncludesPrivateTaskConversationOnly(t *testing.T) {
	svc := NewService()
	for _, room := range []CreateRoomRequest{
		{Title: "Private task", CreatorID: AdminUserID, MemberIDs: []string{ManagerUserID}, IsDirect: true, TaskSession: true, Type: apitypes.RoomTypeFree},
		{Title: "Group task", CreatorID: AdminUserID, MemberIDs: []string{ManagerUserID}, TaskSession: true, Type: apitypes.RoomTypeOnDemand},
	} {
		created, err := svc.CreateRoom(room)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.CreateMessage(CreateMessageRequest{RoomID: created.ID, SenderID: AdminUserID, Content: room.Title, MemoryUserID: AdminUserID}); err != nil {
			t.Fatal(err)
		}
	}
	history, err := svc.MemoryHistory(context.Background(), agentmemory.Scope{AgentID: "agent-manager", UserID: AdminUserID}, time.Time{}, time.Now(), agentmemory.HistoryCursor{})
	if err != nil || len(history) != 1 || history[0].Text != "Private task" {
		t.Fatalf("history = %+v, error = %v", history, err)
	}
}

func TestMemoryHistoryPagesCoverAllPrivateMessagesWithStableTies(t *testing.T) {
	svc := NewService()
	stamp := time.Unix(100, 0)
	var rooms []string
	for index := range 2 {
		room, err := svc.CreateRoom(CreateRoomRequest{Title: fmt.Sprintf("History %d", index), CreatorID: AdminUserID, MemberIDs: []string{ManagerUserID}, IsDirect: true, TaskSession: true})
		if err != nil {
			t.Fatal(err)
		}
		rooms = append(rooms, room.ID)
	}
	want := make(map[agentmemory.Source]bool)
	for index, roomID := range rooms {
		for i := range 125 {
			id := fmt.Sprintf("history-%03d", i)
			svc.rooms[roomID].Messages = append(svc.rooms[roomID].Messages, Message{ID: id, SenderID: AdminUserID,
				CreatedAt: stamp, Content: fmt.Sprintf("private-%d-%d", index, i)})
			want[agentmemory.Source{RoomID: roomID, MessageID: id}] = true
		}
		for _, message := range []Message{
			{ID: "future", SenderID: AdminUserID, CreatedAt: stamp.Add(time.Hour), Content: "future"},
			{ID: "old", SenderID: AdminUserID, CreatedAt: stamp.Add(-time.Hour), Content: "old"},
			{ID: "assistant", SenderID: ManagerUserID, CreatedAt: stamp, Content: "not user text"},
			{ID: "control", SenderID: AdminUserID, CreatedAt: stamp, Content: "忘掉这条"},
		} {
			svc.rooms[roomID].Messages = append(svc.rooms[roomID].Messages, message)
		}
	}
	scope := agentmemory.Scope{AgentID: "agent-manager", UserID: AdminUserID}
	var cursor agentmemory.HistoryCursor
	var count, pages int
	for {
		page, err := svc.MemoryHistory(t.Context(), scope, stamp.Add(-time.Second), stamp, cursor)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		pages++
		if pages > 10 || len(page) > agentmemory.HistoryPageMessages {
			t.Fatal("page did not make progress")
		}
		for _, item := range page {
			if !want[item.Source] || item.Cursor().Compare(cursor) <= 0 {
				t.Fatalf("duplicated, missing or unordered source: %+v", item.Source)
			}
			delete(want, item.Source)
			cursor = item.Cursor()
			count++
		}
	}
	if count != 250 || len(want) != 0 || pages < 2 {
		t.Fatalf("history incomplete: messages=%d pages=%d missing=%d", count, pages, len(want))
	}
	cursor = agentmemory.HistoryCursor{At: stamp, RoomID: rooms[0], MessageID: "history-000", Offset: 8}
	page, err := svc.MemoryHistory(t.Context(), scope, stamp.Add(-time.Second), stamp, cursor)
	if err != nil || len(page) == 0 || page[0].MessageID != "history-000" || page[0].RoomID != rooms[0] {
		t.Fatalf("partial message not resumed: %+v, %v", page, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := svc.MemoryHistory(ctx, scope, time.Time{}, stamp, agentmemory.HistoryCursor{}); err != context.Canceled {
		t.Fatalf("cancellation ignored: %v", err)
	}
}

func TestMemoryHistoryExcludesIneligibleIngressAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	svc, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	room, err := svc.CreateRoom(CreateRoomRequest{Title: "Private", CreatorID: AdminUserID, MemberIDs: []string{ManagerUserID}, IsDirect: true, TaskSession: true})
	if err != nil {
		t.Fatal(err)
	}
	local, err := svc.CreateMessage(CreateMessageRequest{RoomID: room.ID, SenderID: AdminUserID, Content: "Local preference", MemoryUserID: AdminUserID})
	if err != nil {
		t.Fatal(err)
	}
	var remote CreateMessageRequest
	raw := fmt.Sprintf(`{"room_id":%q,"sender_id":%q,"content":"Remote preference","memory_user_id":%q,"memory_excluded":false}`, room.ID, AdminUserID, AdminUserID)
	if err := json.Unmarshal([]byte(raw), &remote); err != nil {
		t.Fatal(err)
	}
	if remote.MemoryUserID != "" {
		t.Fatal("client forged trusted memory identity")
	}
	denied, err := svc.CreateMessage(remote)
	if err != nil {
		t.Fatal(err)
	}
	if local.MemoryExcluded || !denied.MemoryExcluded {
		t.Fatalf("wrong provenance: local=%+v remote=%+v", local, denied)
	}
	delivered, err := svc.DeliverMessage(DeliverMessageRequest{RoomID: room.ID, SenderID: AdminUserID, Content: strings.Repeat("Untrusted delivery. ", 6000)})
	if err != nil || !delivered.MemoryExcluded {
		t.Fatalf("untrusted delivery was not excluded: %+v %v", delivered, err)
	}
	reloaded, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	scope := agentmemory.Scope{AgentID: "agent-manager", UserID: AdminUserID}
	history, err := reloaded.MemoryHistory(t.Context(), scope, time.Time{}, time.Now(), agentmemory.HistoryCursor{})
	if err != nil || len(history) != 1 || history[0].MessageID != local.ID {
		t.Fatalf("history mixed callers after restart: %+v %v", history, err)
	}
	if reloaded.MemorySourceExists(scope, agentmemory.Source{RoomID: room.ID, MessageID: denied.ID}) {
		t.Fatal("ineligible source can be used for memory")
	}
}
