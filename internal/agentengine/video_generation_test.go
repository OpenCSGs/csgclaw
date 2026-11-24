package agentengine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelcap"
	"csgclaw/internal/modelprovider"
)

func TestVideoToolGuidanceAndPreflightUseSelectedModelCapabilities(t *testing.T) {
	ref := &modelprovider.VideoGenerationConfig{ProviderID: "opencsg", ModelID: "video-model"}
	var refreshCalls []bool
	conversation := &conversations{engine: &Engine{
		videoCapabilities: func(_ context.Context, selected *modelprovider.VideoGenerationConfig, refresh bool) (modelcap.VideoGeneration, error) {
			refreshCalls = append(refreshCalls, refresh)
			if *selected != *ref {
				t.Fatalf("selected model = %#v, want %#v", selected, ref)
			}
			return modelcap.VideoGeneration{Sizes: []string{"480P", "720P"}, Seconds: []int{4, 5, 6}}, nil
		},
	}}
	ctx, guidance := conversation.prepareVideoTool(context.Background(), contract.MediaGenerationSink{}, ref)
	for _, expected := range []string{"video-model", `["480P" "720P"]`, "[4 5 6]"} {
		if !strings.Contains(guidance, expected) {
			t.Fatalf("guidance %q missing %q", guidance, expected)
		}
	}
	if !slices.Equal(refreshCalls, []bool{false}) {
		t.Fatalf("guidance accessed more than cached metadata: %v", refreshCalls)
	}
	options, err := contract.PrepareVideoGeneration(ctx, modelprovider.VideoGenerationOptions{})
	if err != nil || options.Size != "480P" || options.Seconds != 4 {
		t.Fatalf("defaults = %#v, err = %v", options, err)
	}
	options, err = contract.PrepareVideoGeneration(ctx, modelprovider.VideoGenerationOptions{Size: "720p", Seconds: 5})
	if err != nil || options.Size != "720P" || options.Seconds != 5 {
		t.Fatalf("explicit options = %#v, err = %v", options, err)
	}
	if _, err := contract.PrepareVideoGeneration(ctx, modelprovider.VideoGenerationOptions{Size: "1080P"}); err == nil {
		t.Fatal("unsupported size accepted")
	}
	if !slices.Equal(refreshCalls, []bool{false, true, true, true}) {
		t.Fatalf("capability lookups = %v", refreshCalls)
	}
}

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
		options.Progress(modelprovider.VideoGenerationProgress{ID: "job", Status: "queued", StatusChecked: true, UpdatedAt: time.Now()})
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
	handler, _ := conversation.videoHandler(turn, sink, nil, []InputPart{{Kind: InputPartFile, File: &InputFile{ID: file.ID, Resolved: file}}}, nil)
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

func TestVideoHandlerReturnsFirstPollRejectionToAgent(t *testing.T) {
	rejection := &modelprovider.VideoGenerationError{Code: "invalid_parameter", Message: "duration 3 is unsupported"}
	var requestIDs []string
	var cards []contract.VideoGenerationTask
	conversation := &conversations{engine: &Engine{
		generateVideo: func(_ context.Context, _ *modelprovider.VideoGenerationConfig, _ string, options modelprovider.VideoGenerationOptions) (modelprovider.GeneratedVideo, error) {
			requestIDs = append(requestIDs, options.RequestID)
			options.Progress(modelprovider.VideoGenerationProgress{ID: "job", Status: "queued", UpdatedAt: time.Now()})
			options.Progress(modelprovider.VideoGenerationProgress{ID: "job", Status: "failed", UpdatedAt: time.Now()})
			return modelprovider.GeneratedVideo{}, rejection
		},
	}}
	turn := &activeTurn{request: TurnRequest{ID: "turn-video"}}
	sink := contract.VideoGenerationSink{EventSink: EventSinkFunc(func(_ context.Context, event TurnEvent) error {
		cards = append(cards, event.Output.Payload.(contract.VideoGenerationTask))
		return nil
	})}
	handler, checkRetry := conversation.videoHandler(turn, sink, nil, nil, nil)
	for range 2 {
		err := handler(context.Background(), "call-video", "animate", modelprovider.VideoGenerationOptions{Seconds: 3})
		if !errors.Is(err, rejection) {
			t.Fatalf("tool error = %v, want upstream rejection", err)
		}
	}
	if err := checkRetry("animate", modelprovider.VideoGenerationOptions{Seconds: 4, RetryOf: "missing"}); err == nil {
		t.Fatal("preflight accepted an unknown retry")
	}
	if err := checkRetry("animate", modelprovider.VideoGenerationOptions{Seconds: 4, RetryOf: "call-video"}); err != nil {
		t.Fatalf("valid retry preflight: %v", err)
	}
	if err := handler(context.Background(), "call-video-revised", "animate", modelprovider.VideoGenerationOptions{Seconds: 4, RetryOf: "call-video"}); !errors.Is(err, rejection) {
		t.Fatalf("retry error = %v, want upstream rejection", err)
	}
	if err := handler(context.Background(), "call-video-other", "animate", modelprovider.VideoGenerationOptions{Seconds: 5}); !errors.Is(err, rejection) {
		t.Fatalf("independent request error = %v, want upstream rejection", err)
	}
	if !slices.Equal(requestIDs, []string{"call-video", "call-video-revised", "call-video-other"}) {
		t.Fatalf("upstream idempotency keys = %v", requestIDs)
	}
	var cardIDs []string
	for _, card := range cards {
		if card.State == "failed" {
			cardIDs = append(cardIDs, card.ID)
		}
	}
	if !slices.Equal(cardIDs, []string{"call-video", "call-video", "call-video-other"}) {
		t.Fatalf("cards = %v, want the revised attempt to update the original card", cardIDs)
	}
}

func TestVideoHandlerEmitsFeedbackAfterAcknowledgment(t *testing.T) {
	release := make(chan struct{})
	feedback := make(chan contract.VideoGenerationFeedback, 1)
	conversation := &conversations{engine: &Engine{
		generateVideo: func(_ context.Context, _ *modelprovider.VideoGenerationConfig, _ string, options modelprovider.VideoGenerationOptions) (modelprovider.GeneratedVideo, error) {
			options.Progress(modelprovider.VideoGenerationProgress{ID: "job", Status: "in_progress", StatusChecked: true, UpdatedAt: time.Now()})
			<-release
			options.Progress(modelprovider.VideoGenerationProgress{ID: "job", Status: "failed", UpdatedAt: time.Now()})
			return modelprovider.GeneratedVideo{}, &modelprovider.VideoGenerationError{Code: "invalid_parameter", Message: "duration rejected"}
		},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	turn := &activeTurn{request: TurnRequest{ID: "turn-video"}}
	sink := contract.VideoGenerationSink{EventSink: EventSinkFunc(func(_ context.Context, event TurnEvent) error {
		if event.Output.Kind == contract.OutputItemVideoFeedback {
			feedback <- event.Output.Payload.(contract.VideoGenerationFeedback)
		}
		return nil
	})}
	handler, _ := conversation.videoHandler(turn, sink, nil, nil, nil)
	err := handler(ctx, "call-video", "animate", modelprovider.VideoGenerationOptions{Seconds: 3})
	close(release)
	if err != nil {
		t.Fatalf("submission acknowledgment: %v", err)
	}
	select {
	case got := <-feedback:
		if got.Retry.AttemptID != "call-video" || got.Retry.Seconds != 3 || got.Message != "duration rejected" {
			t.Fatalf("structured video feedback = %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("no upstream failure feedback after acknowledgment")
	}
}

func TestVideoContinuationReusesOriginalCardAndReference(t *testing.T) {
	ref := &modelprovider.VideoGenerationConfig{ProviderID: "fixture", ModelID: "video"}
	image := &modelprovider.ReferenceImage{Name: "original.png", Data: []byte("original image")}
	inherited := &contract.VideoGenerationRetry{
		AttemptID: "failed-attempt", CardID: "original-card", Prompt: "animate",
		Model: *ref, Seconds: 3, Indices: []int{2}, Reference: image,
	}
	var emitted contract.VideoGenerationTask
	submitted := false
	conversation := &conversations{engine: &Engine{
		generateVideo: func(_ context.Context, _ *modelprovider.VideoGenerationConfig, _ string, options modelprovider.VideoGenerationOptions) (modelprovider.GeneratedVideo, error) {
			submitted = options.Reference == image && options.RequestID == "new-attempt" && options.Seconds == 4
			return modelprovider.GeneratedVideo{}, errors.New("fixture stops after observing submission")
		},
	}}
	sink := contract.VideoGenerationSink{EventSink: EventSinkFunc(func(_ context.Context, event TurnEvent) error {
		emitted = event.Output.Payload.(contract.VideoGenerationTask)
		return nil
	})}
	handler, preflight := conversation.videoHandler(&activeTurn{request: TurnRequest{ID: "new-turn"}}, sink, ref, nil, inherited)
	options := modelprovider.VideoGenerationOptions{Seconds: 4, RetryOf: "failed-attempt"}
	if err := preflight("animate", options, 1); err == nil {
		t.Fatal("retry accepted different reference indices")
	}
	if err := preflight("animate", options, 2); err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if err := handler(context.Background(), "new-attempt", "animate", options, 2); err == nil {
		t.Fatal("fixture failure was lost")
	}
	if !submitted || emitted.ID != "original-card" || emitted.RequestID != "new-attempt" {
		t.Fatalf("retry preserved submission=%t, card=%+v", submitted, emitted)
	}
}
