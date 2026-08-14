package delivery

import (
	"context"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	"csgclaw/internal/channelbridge/runtimebridge"
	"csgclaw/internal/im"
)

type fixedParticipantResolver struct {
	item apitypes.Participant
}

func (r fixedParticipantResolver) Get(_, id string) (apitypes.Participant, bool) {
	return r.item, id == r.item.ID
}

func TestIMTranscriptStoreWritesFinalAndUpdatesToolActivity(t *testing.T) {
	imService := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "user-admin",
		Users: []im.User{
			{ID: "user-admin", Name: "admin"},
			{ID: "user-worker", Name: "worker"},
		},
		Rooms: []im.Room{{
			ID: "room-1", IsDirect: true, Members: []string{"user-admin", "user-worker"},
		}},
	})
	store, err := NewIMTranscriptStore(imService, fixedParticipantResolver{item: apitypes.Participant{
		ID: "pt-worker", ChannelUserRef: "user-worker",
	}})
	if err != nil {
		t.Fatalf("NewIMTranscriptStore() error = %v", err)
	}
	turn := channel.TurnContext{
		ParticipantID: "pt-worker", RoomID: "room-1", SourceMessageID: "message-1",
		ConversationKey: "conversation-1", TurnID: "turn-1",
	}
	for _, status := range []string{"running", "completed"} {
		if err := store.DeliverActivity(context.Background(), turn, agentengine.TurnEvent{
			Kind: agentengine.TurnEventToolCallUpdate,
			Tool: &agentengine.ToolActivity{ID: "tool-1", Kind: "exec_command", Status: status},
		}); err != nil {
			t.Fatalf("DeliverActivity(%s) error = %v", status, err)
		}
	}
	if err := store.DeliverMessage(context.Background(), turn, "finished"); err != nil {
		t.Fatalf("DeliverMessage() error = %v", err)
	}
	if err := store.DeliverMessage(context.Background(), turn, "finished again"); err != nil {
		t.Fatalf("DeliverMessage(replay) error = %v", err)
	}

	room, ok := imService.Room("room-1")
	if !ok || len(room.Messages) != 2 {
		t.Fatalf("room = %+v, want replaced tool activity and one final response", room)
	}
	if room.Messages[0].SenderID != "user-worker" || !strings.Contains(room.Messages[0].Content, "completed") {
		t.Fatalf("tool message = %+v", room.Messages[0])
	}
	if room.Messages[1].SenderID != "user-worker" || room.Messages[1].Content != "finished again" || room.Messages[1].ID != "turn-1-final" {
		t.Fatalf("final message = %+v", room.Messages[1])
	}
	for _, message := range room.Messages {
		assertChannelMetadata(t, message.Metadata)
	}
}

func TestIMTranscriptStorePreservesActivityAndRuntimeErrorMetadata(t *testing.T) {
	imService := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "user-admin",
		Users: []im.User{
			{ID: "user-admin", Name: "admin"},
			{ID: "user-worker", Name: "worker"},
		},
		Rooms: []im.Room{{
			ID: "room-1", IsDirect: true, Members: []string{"user-admin", "user-worker"},
		}},
	})
	store, err := NewIMTranscriptStore(imService, fixedParticipantResolver{item: apitypes.Participant{
		ID: "pt-worker", ChannelUserRef: "user-worker",
	}})
	if err != nil {
		t.Fatalf("NewIMTranscriptStore() error = %v", err)
	}
	turn := channel.TurnContext{
		ParticipantID: "pt-worker", RoomID: "room-1", SourceMessageID: "message-1",
		ConversationKey: "conversation-1", TurnID: "turn-1",
	}
	activityPayload := map[string]any{"type": runtimebridge.AgentActivityType}
	if err := store.DeliverRenderedActivity(context.Background(), turn, ActivityDelivery{
		MessageID: "question-1",
		Text:      "question",
		Metadata: map[string]any{
			runtimebridge.CSGClawMetadataKey: map[string]any{
				runtimebridge.AgentActivityMetaKey: activityPayload,
			},
		},
	}); err != nil {
		t.Fatalf("DeliverRenderedActivity() error = %v", err)
	}

	failureTurn := turn
	failureTurn.TurnID = "turn-2"
	if err := store.DeliverFailure(context.Background(), failureTurn, "unexpected status 429"); err != nil {
		t.Fatalf("DeliverFailure() error = %v", err)
	}

	room, ok := imService.Room("room-1")
	if !ok || len(room.Messages) != 2 {
		t.Fatalf("room = %+v, want question and failure", room)
	}
	assertChannelMetadata(t, room.Messages[0].Metadata)
	questionMetadata, _ := room.Messages[0].Metadata[runtimebridge.CSGClawMetadataKey].(map[string]any)
	if questionMetadata[runtimebridge.AgentActivityMetaKey] == nil {
		t.Fatalf("question metadata = %#v, want preserved agent activity", room.Messages[0].Metadata)
	}
	assertChannelMetadata(t, room.Messages[1].Metadata)
	failureMetadata, _ := room.Messages[1].Metadata[runtimebridge.CSGClawMetadataKey].(map[string]any)
	if failureMetadata[runtimebridge.RuntimeErrorMetaKey] != true || failureMetadata["error_code"] != "rate_limit_exceeded" {
		t.Fatalf("failure metadata = %#v, want runtime error fields", failureMetadata)
	}
}

func assertChannelMetadata(t *testing.T, metadata map[string]any) map[string]any {
	t.Helper()
	namespace, ok := metadata[channelMetadataKey].(map[string]any)
	if !ok {
		t.Fatalf("metadata = %#v, want channel namespace", metadata)
	}
	if len(namespace) != 2 || namespace["type"] != string(channel.ChannelCSGClaw) || namespace["version"] != channelMetadataVersion {
		t.Fatalf("channel metadata = %#v, want only CSGClaw and version %d", namespace, channelMetadataVersion)
	}
	return namespace
}
