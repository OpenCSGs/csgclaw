package contract

import (
	"context"
	"csgclaw/internal/modelprovider"
	"fmt"
)

const OutputItemImageGeneration OutputItemKind = "image_generation"

// ImageGenerationTask is the replayable image request and its delivery state.
// Model is captured when the task starts, so retry never reruns the chat turn.
type ImageGenerationTask struct {
	ErrorDetails *modelprovider.ImageGenerationError  `json:"error_details,omitempty"`
	ID           string                               `json:"id"`
	Prompt       string                               `json:"prompt"`
	Model        *modelprovider.ImageGenerationConfig `json:"model,omitempty"`
	State        string                               `json:"state"`
	Error        string                               `json:"error,omitempty"`
	File         *OutputFile                          `json:"file,omitempty"`
}

type imageGenerationKey struct{}
type ImageGenerationHandler func(context.Context, string, string) error

func WithImageGenerationHandler(ctx context.Context, handler ImageGenerationHandler) context.Context {
	return context.WithValue(ctx, imageGenerationKey{}, handler)
}

func GenerateImage(ctx context.Context, callID, prompt string) error {
	handler, ok := ctx.Value(imageGenerationKey{}).(ImageGenerationHandler)
	if !ok || handler == nil {
		return fmt.Errorf("image generation is unavailable in this conversation")
	}
	return handler(ctx, callID, prompt)
}

func CloneImageGenerationTask(task ImageGenerationTask) ImageGenerationTask {
	task.Model = modelprovider.CloneImageGeneration(task.Model)
	if task.ErrorDetails != nil {
		details := *task.ErrorDetails
		task.ErrorDetails = &details
	}
	if task.File != nil {
		file := task.File.Metadata()
		task.File = &file
	}
	return task
}

// ImageGenerationSink opts in only after the channel implements immediate,
// acknowledged image delivery. Ignoring unknown events is not delivery.
type ImageGenerationSink struct{ EventSink }

func (ImageGenerationSink) SupportsImageGeneration() bool { return true }
