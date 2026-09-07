package api

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/im"
)

func TestNativeConversationResetThroughMessageAPI(t *testing.T) {
	for _, tc := range []struct{ kind, command string }{
		{agent.RuntimeKindOpenClawSandbox, "/new"},
		{agent.RuntimeKindPicoClawSandbox, "/clear"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			target := completeWorkerAgent("agent-native", "native")
			target.RuntimeKind = tc.kind
			controller := mustNewSeededServiceWithOptions(t, []agent.Agent{target}, agent.WithRuntime(fakeCompatRuntime{kind: tc.kind}))
			bus := im.NewBus()
			imService := im.NewServiceFromBootstrapWithBus(im.Bootstrap{CurrentUserID: "u-admin", Users: []im.User{{ID: "u-admin", Name: "admin"}, {ID: "u-native", Name: "native", Role: "worker"}}, Rooms: []im.Room{{ID: "room-native", IsDirect: true, Members: []string{"u-admin", "u-native"}}}}, bus)
			messages, unsubscribeMessages := bus.Subscribe()
			defer unsubscribeMessages()
			bridge := im.NewParticipantBridge("")
			events, unsubscribe := bridge.Subscribe("u-native")
			defer unsubscribe()
			h := NewHandler(AgentServices{Records: controller, Workspace: controller.Workspace(), Models: controller.Models(), Runtime: controller}, agentengine.New(controller), imService, bus, bridge, nil, nil)
			w := httptest.NewRecorder()
			h.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"room_id":"room-native","sender_id":"u-admin","content":"<slash-command name=\"new\" arg=\"conversation\"></slash-command> reset"}`)))
			if w.Code != http.StatusCreated {
				t.Fatalf("HTTP=%d %s", w.Code, w.Body)
			}
			select {
			case message := <-messages:
				h.PublishParticipantEvent(message)
			case <-time.After(time.Second):
				t.Fatal("message API did not publish its event")
			}
			select {
			case event := <-events:
				if event.Text != tc.command {
					t.Fatalf("native gateway received %q, want %q", event.Text, tc.command)
				}
			case <-time.After(time.Second):
				t.Fatal("native gateway did not receive reset command")
			}
		})
	}
}

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
