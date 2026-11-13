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
	err := conversation.generateVideo(context.Background(), turn, sink, contract.VideoGenerationTask{ID: "video-call", Prompt: "kitten"}, nil)
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
	var recoveredFileID string
	go func() {
		done <- conversation.RecoverVideoGeneration(context.Background(), TurnRequest{
			ID: "recovery-turn", ConversationKey: "room-video", VideoGeneration: &contract.VideoGenerationTask{ID: "video-call", Prompt: "kitten"},
		}, contract.VideoGenerationSink{EventSink: EventSinkFunc(func(_ context.Context, event TurnEvent) error {
			task := event.Output.Payload.(contract.VideoGenerationTask)
			if task.File != nil {
				recoveredFileID = task.File.ID
			}
			return nil
		})})
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
	if recoveredFileID == "" {
		t.Fatal("recovery did not deliver a file")
	}
	if _, err := conversation.Files().Get(context.Background(), recoveredFileID); ErrorCodeOf(err) != ErrorFileNotFound {
		t.Fatalf("recovery file remains registered: %v", err)
	}
}

func TestVideoTimingPreservesStartOnRecoveryAndEndsOnFailure(t *testing.T) {
	start := time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339Nano)
	engine := &Engine{files: NewFileStore(), generateVideo: func(context.Context, *modelprovider.VideoGenerationConfig, string, modelprovider.VideoGenerationOptions) (modelprovider.GeneratedVideo, error) {
		return modelprovider.GeneratedVideo{}, errors.New("provider failed")
	}}
	conversation := &conversations{engine: engine, agentID: "video-agent"}
	var tasks []contract.VideoGenerationTask
	sink := contract.VideoGenerationSink{EventSink: EventSinkFunc(func(_ context.Context, event TurnEvent) error {
		tasks = append(tasks, event.Output.Payload.(contract.VideoGenerationTask))
		return nil
	})}
	result := conversation.RecoverVideoGeneration(context.Background(), TurnRequest{ID: "recovery", ConversationKey: "room", VideoGeneration: &contract.VideoGenerationTask{ID: "job", Prompt: "test", StartedAt: start}}, sink)
	if result.Status != TurnFailed || len(tasks) != 2 {
		t.Fatalf("result=%+v tasks=%+v", result, tasks)
	}
	if tasks[0].StartedAt != start || tasks[0].EndedAt != "" || tasks[1].StartedAt != start {
		t.Fatalf("timing was reset: %+v", tasks)
	}
	end, err := time.Parse(time.RFC3339Nano, tasks[1].EndedAt)
	if err != nil || time.Since(end) > time.Second {
		t.Fatalf("missing terminal timestamp: %+v", tasks[1])
	}
}

func TestVideoHandlerKeepsReferenceAfterTurnInputCleanup(t *testing.T) {
	ctx := context.Background()
	data := []byte("\x89PNG\r\n\x1a\nfixture")
	file, err := contract.NewOutputFile(ctx, contract.OutputFileMetadata{
		Name: "first-frame.png", MediaType: "image/png", SizeBytes: int64(len(data)),
	}, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Cleanup()

	inputReleased := make(chan struct{})
	completed := make(chan contract.VideoGenerationTask, 1)
	engine := &Engine{files: NewFileStore(), generateVideo: func(_ context.Context, _ *modelprovider.VideoGenerationConfig, _ string, options modelprovider.VideoGenerationOptions) (modelprovider.GeneratedVideo, error) {
		<-inputReleased
		if options.Reference == nil || options.Reference.Name != "first-frame.png" || !bytes.Equal(options.Reference.Data, data) {
			return modelprovider.GeneratedVideo{}, errors.New("first frame was lost after input cleanup")
		}
		return modelprovider.GeneratedVideo{Content: io.NopCloser(bytes.NewReader([]byte("video"))), SizeBytes: 5, MediaType: "video/mp4"}, nil
	}}
	conversation := &conversations{engine: engine, agentID: "agent-video"}
	turn := &activeTurn{agentID: "agent-video", request: TurnRequest{ID: "turn-video", ConversationKey: "room-video"}}
	defer engine.files.DeleteTurn("agent-video", "room-video", "turn-video")
	sink := contract.MediaGenerationSink{EventSink: EventSinkFunc(func(_ context.Context, event TurnEvent) error {
		task := event.Output.Payload.(contract.VideoGenerationTask)
		if task.State == "completed" || task.State == "failed" {
			completed <- task
		}
		return nil
	})}
	handler := conversation.videoHandler(turn, sink, nil, []InputPart{{Kind: InputPartFile, File: &InputFile{ID: file.ID, Resolved: file}}})
	if err := handler(ctx, "call-video", "animate", modelprovider.VideoGenerationOptions{}, 2); err == nil {
		t.Fatal("accepted an out-of-range reference")
	}
	if err := handler(ctx, "call-video", "animate", modelprovider.VideoGenerationOptions{}, 1); err != nil {
		t.Fatal(err)
	}
	file.Cleanup()
	close(inputReleased)

	select {
	case task := <-completed:
		if task.State != "completed" || task.File == nil {
			t.Fatalf("video task = %+v", task)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("video task did not finish")
	}
}
