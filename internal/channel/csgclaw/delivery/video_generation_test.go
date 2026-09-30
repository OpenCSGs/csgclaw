package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	"csgclaw/internal/im"
)

func TestCompletedVideoReplacesStatusWithPlayableAttachment(t *testing.T) {
	ctx := context.Background()
	service, err := im.NewServiceFromPath(filepath.Join(t.TempDir(), "im", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "agent-video", Name: "video", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	room, err := service.CreateRoom(im.CreateRoomRequest{Title: "Videos", CreatorID: "user-admin", MemberIDs: []string{"user-video"}})
	if err != nil {
		t.Fatal(err)
	}
	files := agentengine.NewFileStore().Scope("agent-video")
	payload := []byte("small mp4 fixture")
	file, err := files.Create(ctx, agentengine.FileCreateRequest{Name: "generated-video.mp4", MIMEType: "video/mp4", SizeBytes: int64(len(payload))}, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewIMTranscriptStore(service, fixedParticipantResolver{item: apitypes.Participant{ID: "pt-video", ChannelUserRef: "user-video"}}, fixedOutputFileSource{files: files})
	if err != nil {
		t.Fatal(err)
	}
	turn := channel.TurnContext{AgentID: "agent-video", ParticipantID: "pt-video", RoomID: room.ID, TurnID: "turn-video", ConversationKey: "room-video"}
	task := contract.VideoGenerationTask{ID: "call-video", Prompt: "kitten", State: "generating"}
	if err := store.DeliverVideoGeneration(ctx, turn, task); err != nil {
		t.Fatal(err)
	}
	beforeDelivery := time.Now()
	task.EndedAt = beforeDelivery.Add(-time.Minute).Format(time.RFC3339Nano)
	task.State = "completed"
	task.File = &file
	if err := store.DeliverVideoGeneration(ctx, turn, task); err != nil {
		t.Fatal(err)
	}
	messages, err := service.ListMessages(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	var delivered []apitypes.Message
	for _, message := range messages {
		if len(message.Attachments) > 0 {
			delivered = append(delivered, message)
		}
	}
	if len(delivered) != 1 || len(delivered[0].Attachments) != 1 || delivered[0].Attachments[0].MediaType != "video/mp4" {
		t.Fatalf("messages = %#v", messages)
	}
	raw, _ := json.Marshal(delivered[0].Metadata["video_generation"])
	var completed contract.VideoGenerationTask
	if err := json.Unmarshal(raw, &completed); err != nil {
		t.Fatal(err)
	}
	end, err := time.Parse(time.RFC3339Nano, completed.EndedAt)
	if err != nil || end.Before(beforeDelivery) || completed.State != "completed" {
		t.Fatalf("completion timestamp precedes attachment delivery: %+v", completed)
	}

}
