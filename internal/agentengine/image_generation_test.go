package agentengine

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelprovider"
)

func TestImageDeliveryRetryDoesNotGenerateAgain(t *testing.T) {
	calls := 0
	engine := &Engine{files: NewFileStore(), generateImage: func(context.Context, *modelprovider.ImageGenerationConfig, string) (modelprovider.GeneratedImage, error) {
		calls++
		return modelprovider.GeneratedImage{Data: []byte("generated image"), MediaType: "image/png"}, nil
	}}
	conversation := &conversations{engine: engine, agentID: "agent-1"}
	turn := &activeTurn{agentID: "agent-1", request: TurnRequest{ID: "turn-1", ConversationKey: "room-1"}}
	var failed contract.ImageGenerationTask
	sink := EventSinkFunc(func(_ context.Context, event TurnEvent) error {
		task := event.Output.Payload.(contract.ImageGenerationTask)
		if task.State == "completed" {
			return errors.New("delivery unavailable")
		}
		if task.State == "delivery_failed" {
			failed = task
		}
		return nil
	})
	err := conversation.generateImage(context.Background(), turn, contract.ImageGenerationSink{EventSink: sink}, contract.ImageGenerationTask{ID: "call-1", Prompt: "blue sky", Model: &modelprovider.ImageGenerationConfig{ProviderID: "codex", ModelID: "gpt-image-2"}})
	if err == nil || failed.File == nil || failed.Error != "image_delivery_failed" {
		t.Fatalf("failed task not retained: %+v, %v", failed, err)
	}
	defer engine.files.Scope("agent-1").Delete(context.Background(), failed.File.ID)
	if err := conversation.generateImage(context.Background(), turn, contract.ImageGenerationSink{EventSink: EventSinkFunc(func(context.Context, TurnEvent) error { return nil })}, failed); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("delivery retry generated %d times", calls)
	}
	if _, err := engine.files.Scope("agent-1").Get(context.Background(), failed.File.ID); err != nil {
		t.Fatalf("generated image disappeared: %v", err)
	}
}

func TestImageGenerationRequiresChannelDeliverySupport(t *testing.T) {
	calls := 0
	engine := &Engine{generateImage: func(context.Context, *modelprovider.ImageGenerationConfig, string) (modelprovider.GeneratedImage, error) {
		calls++
		return modelprovider.GeneratedImage{}, nil
	}}
	conversation := &conversations{engine: engine, agentID: "agent-1"}
	err := conversation.generateImage(context.Background(), &activeTurn{}, EventSinkFunc(func(context.Context, TurnEvent) error { return nil }), contract.ImageGenerationTask{ID: "call-1", Prompt: "blue sky"})
	if err == nil || calls != 0 {
		t.Fatal("generated an image without a channel capable of delivering it")
	}
}

func TestSuccessfulImageRetryParticipatesInCompletedTurnEviction(t *testing.T) {
	ctx := context.Background()
	fail := true
	calls := 0
	engine := &Engine{files: NewFileStore(), runtimes: directConversationResolver{}, generateImage: func(context.Context, *modelprovider.ImageGenerationConfig, string) (modelprovider.GeneratedImage, error) {
		calls++
		if fail {
			return modelprovider.GeneratedImage{}, errors.New("provider unavailable")
		}
		return modelprovider.GeneratedImage{Data: []byte("image snapshot"), MediaType: "image/png"}, nil
	}}
	t.Cleanup(func() { engine.files.DeleteAgent("agent-image") })
	conversation := engine.Conversations("agent-image")
	request := TurnRequest{ID: "image-retry", ConversationKey: "room-image", Input: []InputPart{{Kind: InputPartText, Text: "blue sky"}}, ImageGeneration: &contract.ImageGenerationTask{ID: "image-call", Prompt: "blue sky"}}
	var fileID string
	sink := contract.ImageGenerationSink{EventSink: EventSinkFunc(func(_ context.Context, event TurnEvent) error {
		if task, ok := event.Output.Payload.(contract.ImageGenerationTask); ok && task.State == "completed" {
			fileID = task.File.ID
		}
		return nil
	})}
	result := conversation.Run(ctx, request, sink)
	if result.Status != TurnFailed || result.Dispatched || len(engine.completed) != 0 {
		t.Fatalf("provider failure was cached: %+v", result)
	}
	fail = false
	result = conversation.Run(ctx, request, sink)
	if result.Status != TurnSucceeded || !result.Dispatched || len(engine.completed) != 1 {
		t.Fatalf("successful retry not retained for cleanup: %+v", result)
	}
	snapshot, release, err := engine.files.Resolve("agent-image", fileID)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if replay := conversation.Run(ctx, request, sink); replay.Status != TurnSucceeded || calls != 2 {
		t.Fatalf("successful retry was regenerated: %+v, calls=%d", replay, calls)
	}
	identity := conversationIdentity{agentID: "agent-image", key: "room-image"}
	for i := 0; i < maxCompletedTurns; i++ {
		engine.complete(identity, &activeTurn{request: TurnRequest{ID: TurnID(fmt.Sprintf("later-%d", i))}, done: make(chan struct{})}, TurnResult{Status: TurnSucceeded, Dispatched: true})
	}
	if _, err := conversation.Files().Get(ctx, fileID); err == nil {
		t.Fatal("evicted image remains registered")
	}
	if reader, err := snapshot.Open(ctx); err == nil {
		reader.Close()
		t.Fatal("evicted image snapshot remains readable on disk")
	}
}
