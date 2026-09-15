package delivery

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	"csgclaw/internal/im"
)

func TestGeneratedImageSurvivesFailedChatAndRestart(t *testing.T) {
	ctx := context.Background()
	state := filepath.Join(t.TempDir(), "im", "state.json")
	service, err := im.NewServiceFromPath(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "agent-worker", Name: "worker", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	room, err := service.CreateRoom(im.CreateRoomRequest{Title: "Images", CreatorID: "user-admin", MemberIDs: []string{"user-worker"}})
	if err != nil {
		t.Fatal(err)
	}
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	files := agentengine.NewFileStore().Scope("agent-worker")
	file, err := files.Create(ctx, agentengine.FileCreateRequest{Name: "image.png", MIMEType: "image/png", SizeBytes: int64(imageData.Len())}, bytes.NewReader(imageData.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer files.Delete(ctx, file.ID)
	store, err := NewIMTranscriptStore(service, fixedParticipantResolver{item: apitypes.Participant{ID: "pt-worker", ChannelUserRef: "user-worker"}}, fixedOutputFileSource{files: files})
	if err != nil {
		t.Fatal(err)
	}
	renderer := NewTranscriptRenderer(store)
	turn := channel.TurnContext{AgentID: "agent-worker", ParticipantID: "pt-worker", RoomID: room.ID, TurnID: "turn-1", ConversationKey: "room-1", Locale: "en"}
	task := contract.ImageGenerationTask{ID: "image-call", Prompt: "blue sky", State: "completed", File: &file}
	event := agentengine.TurnEvent{Kind: agentengine.TurnEventOutputItem, Sequence: 1, Output: &agentengine.OutputItem{Kind: contract.OutputItemImageGeneration, Payload: task}}
	if err := renderer.Emit(ctx, turn, event); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Complete(ctx, turn, agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: "trailing text failed"}}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := im.NewServiceFromPath(state)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := reloaded.ListMessages(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, m := range messages {
		for _, attachment := range m.Attachments {
			found++
			stored, err := reloaded.AttachmentFile(attachment.ID)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(stored.Path)
			if err != nil || !bytes.Equal(data, imageData.Bytes()) {
				t.Fatal("generated image bytes did not survive")
			}
		}
	}
	if found != 1 {
		t.Fatalf("got %d images after trailing failure and restart", found)
	}
}
