package conv

import (
	"testing"

	"csgclaw/internal/channel"
)

func TestConversationKeySeparatesTopLevelAndThread(t *testing.T) {
	binding := channel.Binding{ID: "binding-1", ParticipantID: "manager", AgentID: "u-manager"}
	top, err := ConversationKey(binding, channel.Event{RoomID: "room-1"})
	if err != nil {
		t.Fatalf("top ConversationKey() error = %v", err)
	}
	thread, err := ConversationKey(binding, channel.Event{RoomID: "room-1", ThreadRootID: "root-1"})
	if err != nil {
		t.Fatalf("thread ConversationKey() error = %v", err)
	}
	if top == thread {
		t.Fatalf("top and thread keys are equal: %q", top)
	}
}

func TestConversationKeyIsolatesTasksWithoutCreatingThreads(t *testing.T) {
	binding := channel.Binding{ID: "binding-worker", ParticipantID: "dev", AgentID: "agent-dev"}
	keys := map[string]bool{}
	for _, task := range []string{"", "task-1", "task-2"} {
		event := channel.Event{RoomID: "room-1", TaskID: task}
		key, err := ConversationKey(binding, event)
		if err != nil || keys[string(key)] {
			t.Fatalf("session collision: %s %v", key, err)
		}
		keys[string(key)] = true
		again, _ := ConversationKey(binding, event)
		if again != key {
			t.Fatal("same task must reuse its session")
		}
		if event.ThreadRootID != "" {
			t.Fatal("task sessions must not create UI threads")
		}
	}
}

func TestShouldDispatch(t *testing.T) {
	binding := channel.Binding{ParticipantID: "manager", AgentID: "u-manager"}
	event := channel.Event{MessageID: "m1", RoomID: "room-1", Text: "hello"}

	if !ShouldDispatch(binding, event, "u-admin", RoomScope{Direct: true}) {
		t.Fatal("direct room should dispatch")
	}
	if ShouldDispatch(binding, event, "manager", RoomScope{Direct: true}) {
		t.Fatal("self-sent participant message should not dispatch")
	}
	if ShouldDispatch(binding, event, "u-manager", RoomScope{Direct: true}) {
		t.Fatal("self-sent agent message should not dispatch")
	}
	if ShouldDispatch(binding, event, "u-admin", RoomScope{}) {
		t.Fatal("group message without mention should not dispatch")
	}
	if !ShouldDispatch(binding, event, "u-admin", RoomScope{NotifyAll: true}) {
		t.Fatal("notify-all should dispatch")
	}
	mentioned := event
	mentioned.Mentioned = true
	if !ShouldDispatch(binding, mentioned, "u-admin", RoomScope{}) {
		t.Fatal("mentioned event should dispatch")
	}
	named := event
	named.Mentions = []string{"manager"}
	if !ShouldDispatch(binding, named, "u-admin", RoomScope{}) {
		t.Fatal("explicit mention should dispatch")
	}
}

func TestRoomManagerKeepsOneConversationAndRelatedTaskContext(t *testing.T) {
	binding := channel.Binding{ID: "manager"}
	base, err := ConversationKey(binding, channel.Event{RoomID: "r", RoomManager: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", ""} {
		key, err := ConversationKey(binding, channel.Event{RoomID: "r", TaskID: id, ThreadRootID: "thread", RoomManager: true})
		if err != nil || key != base {
			t.Fatal("manager room session fragmented", key, err)
		}
	}
	other, _ := ConversationKey(binding, channel.Event{RoomID: "other", RoomManager: true})
	if other == base {
		t.Fatal("rooms shared manager context")
	}
}
