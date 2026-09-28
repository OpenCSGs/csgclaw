package agentengine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelprovider"
)

func TestVideoDeliveryFailureReplacesDownloadingStatus(t *testing.T) {
	previous := videoDeliveryRetryInterval
	videoDeliveryRetryInterval = time.Millisecond
	t.Cleanup(func() { videoDeliveryRetryInterval = previous })
	engine := &Engine{files: NewFileStore(), generateVideo: func(context.Context, *modelprovider.VideoGenerationConfig, string, modelprovider.VideoGenerationOptions) (modelprovider.GeneratedVideo, error) {
		return modelprovider.GeneratedVideo{Content: io.NopCloser(bytes.NewReader([]byte("generated video"))), SizeBytes: int64(len("generated video")), MediaType: "video/mp4"}, nil
	}}
	conversation := &conversations{engine: engine, agentID: "agent-video"}
	turn := &activeTurn{agentID: "agent-video", request: TurnRequest{ID: "turn-video", ConversationKey: "room-video"}}
	var failed contract.VideoGenerationTask
	completedAttempts := 0
	sink := contract.MediaGenerationSink{EventSink: EventSinkFunc(func(_ context.Context, event TurnEvent) error {
		task := event.Output.Payload.(contract.VideoGenerationTask)
		switch task.State {
		case "completed":
			completedAttempts++
			return errors.New("attachment store unavailable")
		case "delivery_failed":
			failed = task
		}
		return nil
	})}
	err := conversation.generateVideo(context.Background(), turn, sink, contract.VideoGenerationTask{ID: "video-call", Prompt: "kitten"})
	if err == nil || completedAttempts != 3 {
		t.Fatalf("error/attempts = %v/%d", err, completedAttempts)
	}
	if failed.State != "delivery_failed" || failed.Error != "video_delivery_failed" || failed.File != nil || failed.ErrorDetails == nil || failed.ErrorDetails.Stage != "attachment" {
		t.Fatalf("failed task = %#v", failed)
	}
}

func TestRecoverVideoGenerationDoesNotOccupyConversationAdmission(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	engine := &Engine{files: NewFileStore(), generateVideo: func(context.Context, *modelprovider.VideoGenerationConfig, string, modelprovider.VideoGenerationOptions) (modelprovider.GeneratedVideo, error) {
		close(started)
		<-release
		return modelprovider.GeneratedVideo{Content: io.NopCloser(bytes.NewReader([]byte("video"))), SizeBytes: 5, MediaType: "video/mp4"}, nil
	}}
	conversation := engine.Conversations("agent-video").(*conversations)
	done := make(chan TurnResult, 1)
	go func() {
		done <- conversation.RecoverVideoGeneration(context.Background(), TurnRequest{
			ID: "recovery-turn", ConversationKey: "room-video", VideoGeneration: &contract.VideoGenerationTask{ID: "video-call", Prompt: "kitten"},
		}, contract.VideoGenerationSink{EventSink: EventSinkFunc(func(context.Context, TurnEvent) error { return nil })})
	}()
	<-started
	engine.mu.Lock()
	activeTurns := len(engine.active)
	engine.mu.Unlock()
	close(release)
	result := <-done
	if activeTurns != 0 {
		t.Fatalf("recovery occupied %d active conversation turns", activeTurns)
	}
	if result.Status != TurnSucceeded {
		t.Fatalf("recovery result = %+v", result)
	}
}
