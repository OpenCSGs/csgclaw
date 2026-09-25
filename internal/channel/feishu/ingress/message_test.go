package ingress

import (
	"testing"

	channeltypes "csgclaw/internal/channel"
	"csgclaw/internal/channel/feishu/transport"
)

func TestNormalizeMessageRequiresExactBotMentionInGroup(t *testing.T) {
	t.Parallel()
	binding := channeltypes.Binding{ID: "feishu:bot-1", Channel: "feishu", AgentID: "agent-1", ParticipantID: "bot-1"}
	event := transport.Event{
		Kind:    transport.EventMessage,
		EventID: "event-1",
		Message: &transport.Message{
			ID:       "message-1",
			ChatID:   "chat-1",
			ChatType: transport.ChatGroup,
			Text:     "@_user_1 hello @_user_2",
			Mentions: []transport.Mention{
				{Key: "@_user_1", OpenID: "other"},
				{Key: "@_user_2", OpenID: "human"},
			},
		},
	}
	if _, accepted, err := normalizeMessage(binding, event, transport.Identity{OpenID: "bot-open-id"}); err != nil || accepted {
		t.Fatalf("other mention normalize = accepted %v, error %v", accepted, err)
	}
	event.Message.Mentions[0].OpenID = "bot-open-id"
	got, accepted, err := normalizeMessage(binding, event, transport.Identity{OpenID: "bot-open-id"})
	if err != nil || !accepted {
		t.Fatalf("bot mention normalize = accepted %v, error %v", accepted, err)
	}
	if got.Text != "hello @_user_2" {
		t.Fatalf("normalized text = %q", got.Text)
	}
}

func TestNormalizeMessagePreservesThreadAndResources(t *testing.T) {
	t.Parallel()
	binding := channeltypes.Binding{ID: "feishu:bot-1", Channel: "feishu", AgentID: "agent-1"}
	event := transport.Event{
		Kind:    transport.EventMessage,
		EventID: "event-1",
		Message: &transport.Message{
			ID:       "message-1",
			ChatID:   "chat-1",
			ChatType: transport.ChatP2P,
			ThreadID: "thread-1",
			RootID:   "root-1",
			ParentID: "parent-1",
			Text:     "post text",
			Resources: []transport.Resource{{
				Kind: "image", ID: "image-key", Name: "image.png", Size: 42,
			}},
		},
	}
	got, accepted, err := normalizeMessage(binding, event, transport.Identity{OpenID: "bot"})
	if err != nil || !accepted {
		t.Fatalf("normalize = accepted %v, error %v", accepted, err)
	}
	if got.Source.ThreadID != "thread-1" || got.Source.RootID != "root-1" || got.Source.ParentID != "parent-1" {
		t.Fatalf("source thread = %#v", got.Source)
	}
	if len(got.Files) != 1 || got.Files[0].ID != "image-key" || got.Files[0].Kind != "image" {
		t.Fatalf("files = %#v", got.Files)
	}
}

func TestNormalizeInteractiveMessageUsesRawJSON(t *testing.T) {
	t.Parallel()
	binding := channeltypes.Binding{ID: "feishu:bot-1", Channel: "feishu", AgentID: "agent-1"}
	event := transport.Event{Kind: transport.EventMessage, EventID: "event-1", Message: &transport.Message{
		ID: "message-1", ChatID: "chat-1", ChatType: transport.ChatP2P,
		Text: "[interactive card]", ContentType: "interactive", RawContent: []byte(`{"title":"Choose"}`),
	}}
	got, accepted, err := normalizeMessage(binding, event, transport.Identity{})
	if err != nil || !accepted || got.Text != `{"title":"Choose"}` {
		t.Fatalf("normalize interactive = %#v, accepted %v, error %v", got, accepted, err)
	}
}

func TestNormalizeMessageUsesStructuredPostContent(t *testing.T) {
	t.Parallel()
	binding := channeltypes.Binding{ID: "feishu:bot-1", Channel: "feishu", AgentID: "agent-1"}
	event := transport.Event{Kind: transport.EventMessage, EventID: "event-1", Message: &transport.Message{
		ID: "message-1", ChatID: "chat-1", ChatType: transport.ChatP2P,
		ContentType: "post", Text: "磁盘告警\n你能看到这个图片么",
		Resources: []transport.Resource{{Kind: "image", ID: "img_same", Name: "disk.png"}},
	}}
	got, accepted, err := normalizeMessage(binding, event, transport.Identity{})
	if err != nil || !accepted {
		t.Fatalf("normalize = accepted %v, error %v", accepted, err)
	}
	if got.Text != "磁盘告警\n你能看到这个图片么" {
		t.Fatalf("text = %q", got.Text)
	}
	if len(got.Files) != 1 || got.Files[0].Name != "disk.png" {
		t.Fatalf("files = %#v", got.Files)
	}
}

func TestNormalizeMessageAcceptsStopCommand(t *testing.T) {
	binding := channeltypes.Binding{ID: "binding", Channel: "feishu", AgentID: "agent"}
	for _, text := range []string{"/stop", " /STOP ", "@_user_1 /stop"} {
		event := transport.Event{EventID: "event", Message: &transport.Message{ID: "message", ChatID: "chat", ChatType: transport.ChatP2P, Text: text, Mentions: []transport.Mention{{Key: "@_user_1", OpenID: "bot"}}}}
		if _, accepted, err := normalizeMessage(binding, event, transport.Identity{OpenID: "bot"}); err != nil || !accepted {
			t.Fatalf("text=%q accepted=%v err=%v", text, accepted, err)
		}
	}
	event := transport.Event{EventID: "event", Message: &transport.Message{ID: "message", ChatID: "chat", ChatType: transport.ChatP2P, Text: "Explain /stop"}}
	if _, accepted, err := normalizeMessage(binding, event, transport.Identity{OpenID: "bot"}); err != nil || !accepted {
		t.Fatalf("ordinary message accepted=%v err=%v", accepted, err)
	}
}
