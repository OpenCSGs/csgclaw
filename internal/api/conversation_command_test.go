package api

import (
	"reflect"
	"testing"

	"csgclaw/internal/im"
)

func TestNewConversationCommandReasonExtractsCanonicalBody(t *testing.T) {
	reason, matched, err := newConversationCommandReason(`<slash-command name="new" arg="conversation"></slash-command> reset before rebuild`)
	if err != nil {
		t.Fatalf("newConversationCommandReason() error = %v", err)
	}
	if !matched {
		t.Fatal("newConversationCommandReason() matched = false, want true")
	}
	if reason != "reset before rebuild" {
		t.Fatalf("newConversationCommandReason() reason = %q, want %q", reason, "reset before rebuild")
	}
}

func TestNewConversationCommandReasonIgnoresOtherSlashCommands(t *testing.T) {
	_, matched, err := newConversationCommandReason(`<slash-command name="use-skill" arg="skill-creator"></slash-command> reset before rebuild`)
	if err != nil {
		t.Fatalf("newConversationCommandReason() error = %v", err)
	}
	if matched {
		t.Fatal("newConversationCommandReason() matched = true, want false")
	}
}

func TestNewConversationTargetsDirectRoomTargetsAllAgentPeers(t *testing.T) {
	room := im.Room{
		ID:       "room-1",
		IsDirect: true,
		Members:  []string{"u-user", "u-agent-a", "u-agent-b"},
	}
	message := im.Message{SenderID: "u-user"}
	got := newConversationTargets(room, message, func(id string) bool {
		return id == "u-agent-a" || id == "u-agent-b"
	})
	want := []string{"u-agent-a", "u-agent-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("newConversationTargets() = %#v, want %#v", got, want)
	}
}

func TestNewConversationTargetsGroupRoomRequiresMentionedAgent(t *testing.T) {
	room := im.Room{
		ID:       "room-1",
		IsDirect: false,
		Members:  []string{"u-user", "u-agent-a", "u-agent-b"},
	}
	message := im.Message{
		SenderID: "u-user",
		Mentions: []im.Mention{
			{ID: "u-agent-b"},
		},
	}
	got := newConversationTargets(room, message, func(id string) bool {
		return id == "u-agent-a" || id == "u-agent-b"
	})
	want := []string{"u-agent-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("newConversationTargets() = %#v, want %#v", got, want)
	}
}

func TestNewConversationTargetsGroupRoomFanoutTargetsAllAgentPeers(t *testing.T) {
	room := im.Room{
		ID:              "room-1",
		NotifyAllAgents: true,
		Members:         []string{"u-user", "u-agent-a", "u-agent-b"},
	}
	message := im.Message{SenderID: "u-user"}
	got := newConversationTargets(room, message, func(id string) bool {
		return id == "u-agent-a" || id == "u-agent-b"
	})
	want := []string{"u-agent-a", "u-agent-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("newConversationTargets() = %#v, want %#v", got, want)
	}
}

func TestNewConversationTargetsGroupRoomFanoutIncludesExplicitlyMentionedMessages(t *testing.T) {
	room := im.Room{
		ID:              "room-1",
		NotifyAllAgents: true,
		Members:         []string{"u-user", "u-agent-a", "u-agent-b"},
	}
	message := im.Message{
		SenderID: "u-user",
		Content:  `<at user_id="u-agent-b">agent-b</at> please handle this`,
		Mentions: []im.Mention{{ID: "u-agent-b", Name: "agent-b"}},
	}
	got := newConversationTargets(room, message, func(id string) bool {
		return id == "u-agent-a" || id == "u-agent-b"
	})
	want := []string{"u-agent-a", "u-agent-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("newConversationTargets() = %#v, want notify-all targets %#v", got, want)
	}
}

func TestNewConversationTargetsGroupRoomFanoutDoesNotCascadeAgentReplies(t *testing.T) {
	room := im.Room{
		ID:              "room-1",
		NotifyAllAgents: true,
		Members:         []string{"u-user", "u-agent-a", "u-agent-b"},
	}
	isAgent := func(id string) bool {
		return id == "u-agent-a" || id == "u-agent-b"
	}

	message := im.Message{SenderID: "u-agent-a"}
	if got := newConversationTargets(room, message, isAgent); len(got) != 0 {
		t.Fatalf("newConversationTargets() = %#v, want no implicit targets for agent reply", got)
	}

	message.Mentions = []im.Mention{{ID: "u-agent-b"}}
	want := []string{"u-agent-b"}
	if got := newConversationTargets(room, message, isAgent); !reflect.DeepEqual(got, want) {
		t.Fatalf("newConversationTargets() = %#v, want explicit target %#v", got, want)
	}
}

func TestNewConversationTargetsGroupRoomSupportsAtMentionTag(t *testing.T) {
	room := im.Room{
		ID:       "room-1",
		IsDirect: false,
		Members:  []string{"u-user", "u-agent-a", "u-agent-b"},
	}
	message := im.Message{
		SenderID: "u-user",
		Content:  `<at user_id="u-agent-b">qa-worker</at>`,
	}
	got := newConversationTargets(room, message, func(id string) bool {
		return id == "u-agent-a" || id == "u-agent-b"
	})
	want := []string{"u-agent-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("newConversationTargets() = %#v, want %#v", got, want)
	}
}

func TestNewConversationTargetsGroupRoomSupportsAtMentionTagWithExtraMentions(t *testing.T) {
	room := im.Room{
		ID:       "room-1",
		IsDirect: false,
		Members:  []string{"u-user", "u-agent-a", "u-agent-b", "u-human-c"},
	}
	message := im.Message{
		SenderID: "u-user",
		Content:  `<at user_id="u-human-c">human-c</at> <at user_id="u-agent-b">qa-worker</at>`,
		Mentions: []im.Mention{
			{ID: "u-human-c"},
		},
	}
	got := newConversationTargets(room, message, func(id string) bool {
		return id == "u-agent-a" || id == "u-agent-b"
	})
	want := []string{"u-agent-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("newConversationTargets() = %#v, want %#v", got, want)
	}
}
