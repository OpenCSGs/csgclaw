package conv

import (
	"testing"

	"csgclaw/internal/channel"
	"csgclaw/internal/channelbridge"
)

func TestConversationKeySeparatesTopLevelAndThread(t *testing.T) {
	binding := channel.Binding{ID: "binding-1", ParticipantID: "manager", AgentID: "u-manager"}
	top, err := ConversationKey(binding, channelbridge.BotEvent{RoomID: "room-1"})
	if err != nil {
		t.Fatalf("top ConversationKey() error = %v", err)
	}
	thread, err := ConversationKey(binding, channelbridge.BotEvent{RoomID: "room-1", ThreadRootID: "root-1"})
	if err != nil {
		t.Fatalf("thread ConversationKey() error = %v", err)
	}
	if top == thread {
		t.Fatalf("top and thread keys are equal: %q", top)
	}
}

func TestShouldDispatch(t *testing.T) {
	binding := channel.Binding{ParticipantID: "manager", AgentID: "u-manager"}
	event := channelbridge.BotEvent{MessageID: "m1", RoomID: "room-1", Text: "hello"}

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
