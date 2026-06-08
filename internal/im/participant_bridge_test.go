package im

import (
	"testing"
	"time"
)

func TestChatTypeForRoomRespectsIsDirect(t *testing.T) {
	tests := []struct {
		name string
		room Room
		want string
	}{
		{
			name: "direct room stays direct",
			room: Room{
				ID:       "room-direct",
				IsDirect: true,
				Members:  []string{"u-admin", "u-bot"},
			},
			want: "direct",
		},
		{
			name: "two member group stays group",
			room: Room{
				ID:       "room-group",
				IsDirect: false,
				Members:  []string{"u-admin", "u-bot"},
			},
			want: "group",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := chatTypeForRoom(tc.room); got != tc.want {
				t.Fatalf("chatTypeForRoom() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestShouldNotifyParticipantPushesForTwoMemberGroupWithoutMention(t *testing.T) {
	room := Room{
		ID:       "room-group",
		IsDirect: false,
		Members:  []string{"u-admin", "u-bot"},
	}

	message := Message{
		ID:        "msg-1",
		SenderID:  "u-admin",
		Content:   "hello",
		CreatedAt: time.Now().UTC(),
	}

	if !shouldNotifyParticipant(room, message, "u-bot") {
		t.Fatal("shouldNotifyParticipant() = false, want true for room member without mention")
	}
}

func TestPublishMessageEventUsesGroupChatTypeForTwoMemberGroup(t *testing.T) {
	bridge := NewParticipantBridge("")
	events, cancel := bridge.Subscribe("u-bot")
	defer cancel()

	room := Room{
		ID:       "room-group",
		IsDirect: false,
		Members:  []string{"u-admin", "u-bot"},
	}
	sender := User{ID: "u-admin", Name: "Admin", Handle: "admin"}
	message := Message{
		ID:        "msg-1",
		SenderID:  "u-admin",
		Content:   "hello",
		CreatedAt: time.Now().UTC(),
	}

	bridge.PublishMessageEvent(room, sender, message)

	select {
	case evt := <-events:
		if evt.ChatType != "group" {
			t.Fatalf("PublishMessageEvent() chat_type = %q, want group", evt.ChatType)
		}
	case <-time.After(time.Second):
		t.Fatal("PublishMessageEvent() timed out waiting for event")
	}
}

func TestPublishMessageEventUsesSlashContentVerbatim(t *testing.T) {
	bridge := NewParticipantBridge("")
	events, cancel := bridge.Subscribe("u-bot")
	defer cancel()

	room := Room{
		ID:       "room-direct",
		IsDirect: true,
		Members:  []string{"u-admin", "u-bot"},
	}
	sender := User{ID: "u-admin", Name: "Admin", Handle: "admin"}
	message := Message{
		ID:        "msg-skill",
		SenderID:  "u-admin",
		Content:   "/skill-creator make a skill",
		CreatedAt: time.Now().UTC(),
	}

	bridge.PublishMessageEvent(room, sender, message)

	select {
	case evt := <-events:
		if evt.Text != "/skill-creator make a skill" {
			t.Fatalf("Text = %q, want original slash content", evt.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("PublishMessageEvent() timed out waiting for event")
	}
}

func TestPublishMessageEventIncludesThreadRootAndContext(t *testing.T) {
	bridge := NewParticipantBridge("")
	events, cancel := bridge.Subscribe("u-bot")
	defer cancel()

	root := Message{
		ID:        "msg-root",
		SenderID:  "u-ux",
		Content:   "root context",
		CreatedAt: time.Now().UTC(),
	}
	reply := Message{
		ID:        "msg-reply",
		SenderID:  "u-admin",
		Content:   "thread reply",
		CreatedAt: time.Now().UTC().Add(time.Second),
		Mentions:  []Mention{{ID: "u-bot", Name: "bot"}},
		RelatesTo: &MessageRelation{
			RelType: RelationTypeThread,
			EventID: root.ID,
		},
	}
	room := Room{
		ID:       "room-group",
		IsDirect: false,
		Members:  []string{"u-admin", "u-ux", "u-bot"},
		Messages: []Message{root, reply},
		Threads: []ThreadState{{
			RootMessageID: root.ID,
			Context:       []Message{root},
			Summary: ThreadContextSummary{
				RootExcerpt:  "root context",
				MessageCount: 1,
			},
		}},
	}
	sender := User{ID: "u-admin", Name: "Admin", Handle: "admin"}

	bridge.PublishMessageEvent(room, sender, reply)

	select {
	case evt := <-events:
		if evt.ThreadRootID != root.ID {
			t.Fatalf("ThreadRootID = %q, want %q", evt.ThreadRootID, root.ID)
		}
		if len(evt.Mentions) != 1 || evt.Mentions[0] != "u-bot" {
			t.Fatalf("Mentions = %+v, want [u-bot]", evt.Mentions)
		}
		if evt.Channel != "csgclaw" || evt.ChatID != room.ID {
			t.Fatalf("PicoClaw event address = channel %q chat_id %q, want csgclaw %q", evt.Channel, evt.ChatID, room.ID)
		}
		if evt.Context.Channel != "csgclaw" ||
			evt.Context.ChatID != room.ID ||
			evt.Context.ChatType != "group" ||
			evt.Context.TopicID != root.ID ||
			evt.Context.MessageID != reply.ID ||
			evt.Context.SenderID != sender.ID {
			t.Fatalf("PicoClaw context = %+v, want thread topic context", evt.Context)
		}
		if evt.ThreadContext == nil || evt.ThreadContext.RootMessageID != root.ID || len(evt.ThreadContext.Context) != 1 {
			t.Fatalf("ThreadContext = %+v, want root context", evt.ThreadContext)
		}
	case <-time.After(time.Second):
		t.Fatal("PublishMessageEvent() timed out waiting for event")
	}
}

func TestPublishMessageEventNormalizesPlainThreadMentionForPicoClaw(t *testing.T) {
	bridge := NewParticipantBridge("")
	events, cancel := bridge.Subscribe("u-qa")
	defer cancel()

	root := Message{
		ID:        "msg-root",
		SenderID:  "u-manager",
		Content:   "root context",
		CreatedAt: time.Now().UTC(),
	}
	reply := Message{
		ID:        "msg-reply",
		SenderID:  "u-admin",
		Content:   "@qa please check this",
		CreatedAt: time.Now().UTC().Add(time.Second),
		Mentions:  []Mention{{ID: "u-qa", Name: "qa"}},
		RelatesTo: &MessageRelation{
			RelType: RelationTypeThread,
			EventID: root.ID,
		},
	}
	room := Room{
		ID:       "room-group",
		IsDirect: false,
		Members:  []string{"u-admin", "u-manager", "u-qa"},
		Messages: []Message{root, reply},
		Threads: []ThreadState{{
			RootMessageID: root.ID,
			Context:       []Message{root},
		}},
	}
	sender := User{ID: "u-admin", Name: "Admin", Handle: "admin"}

	bridge.PublishMessageEvent(room, sender, reply)

	select {
	case evt := <-events:
		if evt.ThreadRootID != root.ID {
			t.Fatalf("ThreadRootID = %q, want %q", evt.ThreadRootID, root.ID)
		}
		if evt.Text != `<at user_id="u-qa">qa</at> please check this` {
			t.Fatalf("Text = %q, want PicoClaw mention tag", evt.Text)
		}
		if len(evt.Mentions) != 1 || evt.Mentions[0] != "u-qa" || !evt.Context.Mentioned {
			t.Fatalf("Mentions = %+v context = %+v, want u-qa mentioned", evt.Mentions, evt.Context)
		}
	case <-time.After(time.Second):
		t.Fatal("PublishMessageEvent() timed out waiting for event")
	}
}

func TestEnqueueMessageEventWithTextKeepsGroupMentionVisible(t *testing.T) {
	bridge := NewParticipantBridge("")
	events, cancel := bridge.Subscribe("u-qa")
	defer cancel()

	room := Room{
		ID:       "room-group",
		IsDirect: false,
		Members:  []string{"u-admin", "u-qa"},
	}
	sender := User{ID: "u-admin", Name: "Admin", Handle: "admin"}
	message := Message{
		ID:        "msg-new",
		SenderID:  "u-admin",
		Content:   `<slash-command name="new" arg="conversation"></slash-command> <at user_id="u-qa">qa</at>`,
		CreatedAt: time.Now().UTC(),
		Mentions:  []Mention{{ID: "u-qa", Name: "qa"}},
	}

	if !bridge.EnqueueMessageEventWithText(room, sender, message, "u-qa", "/clear") {
		t.Fatal("EnqueueMessageEventWithText() = false, want true for subscribed participant")
	}

	select {
	case evt := <-events:
		if evt.Text != `/clear <at user_id="u-qa">qa</at>` {
			t.Fatalf("Text = %q, want action text with mention tag", evt.Text)
		}
		if len(evt.Mentions) != 1 || evt.Mentions[0] != "u-qa" || !evt.Context.Mentioned {
			t.Fatalf("Mentions = %+v context = %+v, want u-qa mentioned", evt.Mentions, evt.Context)
		}
	case <-time.After(time.Second):
		t.Fatal("EnqueueMessageEventWithText() timed out waiting for event")
	}
}

func TestPublishMessageEventQueuesUntilParticipantSubscribes(t *testing.T) {
	bridge := NewParticipantBridge("")
	room := Room{
		ID:       "room-direct",
		IsDirect: true,
		Members:  []string{"u-admin", "u-bot"},
	}
	sender := User{ID: "u-admin", Name: "Admin", Handle: "admin"}
	message := Message{
		ID:        "msg-queued",
		SenderID:  "u-admin",
		Content:   "queued hello",
		CreatedAt: time.Now().UTC(),
	}

	missed := bridge.PublishMessageEvent(room, sender, message)
	if len(missed) != 1 || missed[0] != "u-bot" {
		t.Fatalf("PublishMessageEvent() missed = %v, want [u-bot]", missed)
	}

	events, cancel := bridge.Subscribe("u-bot")
	defer cancel()

	select {
	case evt := <-events:
		if evt.MessageID != "msg-queued" || evt.Text != "queued hello" {
			t.Fatalf("queued event = %+v, want msg-queued queued hello", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("Subscribe() timed out waiting for queued event")
	}
}

func TestParticipantBridgeAckPreventsDuplicateReplay(t *testing.T) {
	bridge := NewParticipantBridge("")
	events, cancel := bridge.Subscribe("u-bot")
	defer cancel()

	room := Room{
		ID:       "room-direct",
		IsDirect: true,
		Members:  []string{"u-admin", "u-bot"},
	}
	sender := User{ID: "u-admin", Name: "Admin", Handle: "admin"}
	message := Message{
		ID:        "msg-acked",
		SenderID:  "u-admin",
		Content:   "acked hello",
		CreatedAt: time.Now().UTC(),
	}

	bridge.PublishMessageEvent(room, sender, message)
	select {
	case evt := <-events:
		bridge.Ack("u-bot", evt.MessageID)
	case <-time.After(time.Second):
		t.Fatal("PublishMessageEvent() timed out waiting for event")
	}

	bridge.EnqueueMessageEvent(room, sender, message, "u-bot")
	select {
	case evt := <-events:
		t.Fatalf("duplicate event = %+v, want none after ack", evt)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestParticipantBridgeRequeueDeliversUnackedEventAgain(t *testing.T) {
	bridge := NewParticipantBridge("")
	events, cancel := bridge.Subscribe("u-bot")

	room := Room{
		ID:       "room-direct",
		IsDirect: true,
		Members:  []string{"u-admin", "u-bot"},
	}
	sender := User{ID: "u-admin", Name: "Admin", Handle: "admin"}
	message := Message{
		ID:        "msg-requeue",
		SenderID:  "u-admin",
		Content:   "retry hello",
		CreatedAt: time.Now().UTC(),
	}

	bridge.PublishMessageEvent(room, sender, message)
	var got ParticipantEvent
	select {
	case got = <-events:
	case <-time.After(time.Second):
		t.Fatal("PublishMessageEvent() timed out waiting for event")
	}
	bridge.Requeue("u-bot", got)
	cancel()

	events, cancel = bridge.Subscribe("u-bot")
	defer cancel()
	select {
	case evt := <-events:
		if evt.MessageID != "msg-requeue" || evt.Text != "retry hello" {
			t.Fatalf("requeued event = %+v, want msg-requeue retry hello", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("Subscribe() timed out waiting for requeued event")
	}
}
