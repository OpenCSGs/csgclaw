package im

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAgentArchiveHidesWithoutChangingMembershipOrManualArchive(t *testing.T) {
	s := NewServiceFromBootstrap(Bootstrap{CurrentUserID: AdminUserID, Users: []User{
		{ID: AdminUserID, Name: "admin", Role: "admin"}, {ID: "user-worker", Name: "worker", Role: "worker"}, {ID: ManagerUserID, Name: "manager", Role: "manager"},
	}, Rooms: []Room{
		{ID: "direct", IsDirect: true, Archived: true, Members: []string{AdminUserID, "user-worker"}},
		{ID: "group", Members: []string{AdminUserID, ManagerUserID, "user-worker"}, Messages: []Message{{ID: "old", SenderID: "user-worker", Content: "result"}}},
	}})
	archived := true
	s.SetAgentStateLookup(func(id string) string {
		if id == "user-worker" && archived {
			return "archived"
		}
		return ""
	})
	if got := s.ListRooms(); len(got) != 1 || got[0].ID != "group" || slices.Contains(got[0].Members, "user-worker") {
		t.Fatalf("visible rooms: %#v", got)
	}
	if len(s.rooms["group"].Members) != 3 {
		t.Fatal("archive changed stored memberships")
	}
	if _, err := s.CreateMessage(CreateMessageRequest{RoomID: "direct", SenderID: AdminUserID, Content: "run"}); err == nil {
		t.Fatal("archived direct conversation accepted input")
	}
	if _, err := s.CreateMessage(CreateMessageRequest{RoomID: "group", SenderID: AdminUserID, Content: "@worker run"}); err == nil {
		t.Fatal("archived mention accepted")
	}
	archived = false
	got := s.ListRooms()
	if len(got) != 2 {
		t.Fatalf("restore rooms = %d", len(got))
	}
	direct, _ := s.Room("direct")
	if !direct.Archived {
		t.Fatal("restore erased manual conversation archive")
	}
	group, _ := s.Room("group")
	if !slices.Contains(group.Members, "user-worker") {
		t.Fatal("restore lost membership")
	}
	if err := s.RetireAgentUser("user-worker"); err != nil {
		t.Fatal(err)
	}
	group, _ = s.Room("group")
	if len(group.Messages) != 2 || group.Messages[0].Content != "result" || group.Messages[1].Event.Key != AgentDeletedEventKey {
		t.Fatal("deletion erased collaboration history")
	}
	if _, exists := s.Room("direct"); exists {
		t.Fatal("direct conversation survived deletion")
	}
	user, exists := s.User("user-worker")
	if !exists || user.AgentState != "deleted" || user.Name != "worker" {
		t.Fatalf("identity tombstone = %#v", user)
	}
}

func TestAgentLifecycleNoticesPersistOnceWithoutDispatchingAgents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "im", "state.json")
	if err := SaveBootstrap(path, Bootstrap{CurrentUserID: AdminUserID, Users: []User{
		{ID: AdminUserID, Name: "admin", Role: "admin"},
		{ID: ManagerUserID, Name: "manager", Role: "manager"},
		{ID: "user-worker", Name: "Worker", Role: "worker"},
	}, Rooms: []Room{
		{ID: "group", Members: []string{AdminUserID, ManagerUserID, "user-worker"}},
		{ID: "archived-group", Archived: true, Members: []string{AdminUserID, "user-worker"}},
		{ID: "removed-group", Members: []string{AdminUserID, "user-worker"}},
		{ID: "direct", IsDirect: true, Members: []string{AdminUserID, "user-worker"}},
		{ID: "unrelated", Members: []string{AdminUserID, ManagerUserID}},
	}}); err != nil {
		t.Fatal(err)
	}
	bus := NewBus()
	s, err := NewServiceFromPathWithBus(path, bus)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteRoom("removed-group"); err != nil {
		t.Fatal(err)
	}
	s.SetAgentStateLookup(func(id string) string {
		if id == "user-worker" {
			return "archived"
		}
		return ""
	})
	events, unsubscribe := bus.Subscribe()
	defer unsubscribe()
	for _, transition := range []struct{ key, version string }{
		{AgentArchivedEventKey, "cycle-1"}, {AgentArchivedEventKey, "cycle-1"},
		{AgentRestoredEventKey, "cycle-1"}, {AgentRestoredEventKey, "cycle-1"},
		{AgentArchivedEventKey, "cycle-2"},
	} {
		if err := s.RecordAgentLifecycle("agent-worker", transition.key, transition.version); err != nil {
			t.Fatal(err)
		}
	}
	group, _ := s.Room("group")
	if len(group.Messages) != 3 || len(s.rooms["archived-group"].Messages) != 3 || len(s.rooms["direct"].Messages) != 0 || len(s.rooms["unrelated"].Messages) != 0 {
		t.Fatal("notices must appear once per transition, in related group rooms only")
	}
	if _, exists := s.Room("removed-group"); exists {
		t.Fatal("lifecycle notice recreated a deleted room")
	}
	for range 6 {
		select {
		case event := <-events:
			if event.Type != EventTypeMessageCreated || (event.RoomID != "group" && event.RoomID != "archived-group") || event.Message == nil || event.Message.Kind != MessageKindEvent || len(event.Message.Mentions) != 0 {
				t.Fatalf("notice event = %+v", event)
			}
			for _, participantID := range []string{"pt-manager", "pt-worker"} {
				if shouldNotifyParticipant(group, *event.Sender, *event.Message, participantID) {
					t.Fatal("lifecycle notice dispatched an agent")
				}
			}
		default:
			t.Fatal("missing live message event")
		}
	}
	select {
	case event := <-events:
		t.Fatalf("duplicate event: %+v", event)
	default:
	}
	if err := s.RetireAgentUser("agent-worker"); err != nil {
		t.Fatal(err)
	}
	if err := s.RetireAgentUser("agent-worker"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{AgentArchivedEventKey, AgentRestoredEventKey, AgentArchivedEventKey, AgentDeletedEventKey}
	for _, locale := range []string{"en", "zh"} {
		messages, err := reloaded.ListMessagesWithOptions("group", ListMessagesOptions{Locale: locale})
		if err != nil || len(messages) != len(wantKeys) {
			t.Fatalf("reloaded messages = %v, %v", messages, err)
		}
		for i, key := range wantKeys {
			if messages[i].Event == nil || messages[i].Event.Key != key || !strings.Contains(messages[i].Content, "Worker") {
				t.Fatalf("message %d = %+v", i, messages[i])
			}
		}
		if locale == "zh" && (messages[0].Content != "Worker 因智能体归档已被移出协作空间" || messages[1].Content != "Worker 已恢复并加入协作空间" || messages[3].Content != "Worker 因智能体删除已被移出协作空间") {
			t.Fatalf("Chinese notices = %+v", messages)
		}
		if locale == "en" && !strings.Contains(messages[1].Content, "restored and rejoined") {
			t.Fatalf("English restore notice = %s", messages[1].Content)
		}
	}
}

func TestAgentLifecycleNoticeSaveFailureDoesNotPublishOrChangeHistory(t *testing.T) {
	bus := NewBus()
	s := NewServiceFromBootstrapWithBus(Bootstrap{CurrentUserID: AdminUserID, Users: []User{
		{ID: AdminUserID, Name: "admin"}, {ID: "user-worker", Name: "Worker"},
	}, Rooms: []Room{{ID: "group", Members: []string{AdminUserID, "user-worker"}}}}, bus)
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block writes"), 0600); err != nil {
		t.Fatal(err)
	}
	s.statePath = filepath.Join(blocker, "state.json")
	events, unsubscribe := bus.Subscribe()
	defer unsubscribe()
	if err := s.RecordAgentLifecycle("agent-worker", AgentArchivedEventKey, "cycle-1"); err == nil {
		t.Fatal("notice persisted despite blocked storage")
	}
	group, _ := s.Room("group")
	if len(group.Messages) != 0 {
		t.Fatal("failed save changed visible history")
	}
	select {
	case event := <-events:
		t.Fatalf("failed save published event: %+v", event)
	default:
	}
	s.statePath = ""
	if err := s.RecordAgentLifecycle("agent-worker", AgentArchivedEventKey, "cycle-1"); err != nil {
		t.Fatal(err)
	}
	group, _ = s.Room("group")
	if len(group.Messages) != 1 {
		t.Fatal("retry did not record the notice")
	}
}
