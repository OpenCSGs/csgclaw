package im

import (
	"path/filepath"
	"testing"

	"csgclaw/internal/taskconfig"
)

func TestTaskTitlePersistsAndRespectsManualRename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	bus := NewBus()
	svc, err := NewServiceFromPathWithBus(path, bus)
	if err != nil {
		t.Fatal(err)
	}
	agent, _, err := svc.EnsureAgentUser(EnsureAgentUserRequest{
		ID: "user-worker", Name: "Worker", Role: "worker", SkipDirectRoom: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name       string
		autoTitle  bool
		manualEdit bool
		want       string
	}{
		{name: "generated", autoTitle: true, want: "排查登录超时"},
		{name: "manual rename to same title", autoTitle: true, manualEdit: true, want: "临时标题"},
		{name: "legacy or opted out", want: "临时标题"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			room, err := svc.CreateRoom(CreateRoomRequest{
				Title: "临时标题", CreatorID: AdminUserID, MemberIDs: []string{agent.ID},
				IsDirect: true, TaskSession: true, AutoTitle: tt.autoTitle,
			})
			if err != nil {
				t.Fatal(err)
			}
			message, err := svc.CreateMessage(CreateMessageRequest{
				RoomID: room.ID, SenderID: AdminUserID, Content: "为什么登录请求总是超时？",
			})
			if err != nil {
				t.Fatal(err)
			}
			question, _, claimed := svc.ClaimTaskTitle(room.ID, message.ID)
			if claimed != tt.autoTitle {
				t.Fatalf("ClaimTaskTitle() = %q", question)
			}
			if question != "" && question != message.Content {
				t.Fatalf("question = %q, want %q", question, message.Content)
			}
			if _, _, claimed := svc.ClaimTaskTitle(room.ID, message.ID); claimed {
				t.Fatal("duplicate claim succeeded")
			}
			if current, _ := svc.Room(room.ID); current.Title != room.Title {
				t.Fatalf("title changed during generation: %q", current.Title)
			}
			if tt.manualEdit {
				if _, err := svc.UpdateRoom(room.ID, UpdateRoomRequest{Title: &room.Title}); err != nil {
					t.Fatal(err)
				}
			}
			events, unsubscribe := bus.Subscribe()
			defer unsubscribe()
			if err := svc.ApplyTaskTitle(room.ID, message.ID, "排查登录超时"); err != nil {
				t.Fatal(err)
			}
			select {
			case event := <-events:
				if !tt.autoTitle || tt.manualEdit || event.Type != EventTypeRoomUpdated || event.Room.Title != tt.want {
					t.Fatalf("unexpected title event: %+v", event)
				}
			default:
				if tt.autoTitle && !tt.manualEdit {
					t.Fatal("missing room.updated event")
				}
			}
			reloaded, err := NewServiceFromPath(path)
			if err != nil {
				t.Fatal(err)
			}
			current, ok := reloaded.Room(room.ID)
			if !ok || current.Title != tt.want || messageIndexByID(current.Messages, message.ID) < 0 {
				t.Fatalf("persisted room title = %q, found = %v, messages = %d", current.Title, ok, len(current.Messages))
			}
		})
	}
}

func TestTaskTitleClaimRequiresOptInAndCanBeRetried(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	svc, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	worker, _, err := svc.EnsureAgentUser(EnsureAgentUserRequest{ID: "user-worker", Name: "Worker", Role: "worker", SkipDirectRoom: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct {
		id        string
		autoTitle bool
	}{
		{id: "manual"},
		{id: "automatic", autoTitle: true},
	} {
		room, err := svc.CreateRoom(CreateRoomRequest{
			Title: "First question", CreatorID: AdminUserID, MemberIDs: []string{worker.ID},
			IsDirect: true, TaskSession: true, AutoTitle: request.autoTitle,
			ClientRequestID: request.id,
			Configuration:   &taskconfig.Config{},
		})
		if err != nil {
			t.Fatal(err)
		}
		message, err := svc.CreateMessage(CreateMessageRequest{
			RoomID: room.ID, SenderID: AdminUserID, ClientMessageID: "first", Content: room.Title,
		})
		if err != nil {
			t.Fatal(err)
		}
		question, _, claimed := svc.ClaimTaskTitle(room.ID, message.ID)
		if claimed != request.autoTitle {
			t.Fatalf("claim = %q with autoTitle = %v", question, request.autoTitle)
		}
		if !request.autoTitle {
			continue
		}
		svc.ReleaseTaskTitle(room.ID, message.ID)
		if _, _, claimed := svc.ClaimTaskTitle(room.ID, message.ID); !claimed {
			t.Fatal("released claim blocked a retry")
		}
		reloaded, err := NewServiceFromPath(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, claimed := reloaded.ClaimTaskTitle(room.ID, message.ID); !claimed {
			t.Fatal("restarting with an active claim blocked a retry")
		}
	}
}
