package contract

import (
	"context"
	"csgclaw/internal/modelprovider"
	"fmt"
)

const OutputItemVideoGeneration OutputItemKind = "video_generation"

type VideoGenerationTask struct {
	ErrorDetails   *modelprovider.VideoGenerationError  `json:"error_details,omitempty"`
	ID             string                               `json:"id"`
	Prompt         string                               `json:"prompt"`
	Model          *modelprovider.VideoGenerationConfig `json:"model,omitempty"`
	Options        modelprovider.VideoGenerationOptions `json:"options,omitempty"`
	State          string                               `json:"state"`
	Error          string                               `json:"error,omitempty"`
	UpstreamID     string                               `json:"upstream_id,omitempty"`
	UpstreamStatus string                               `json:"upstream_status,omitempty"`
	LastPolledAt   string                               `json:"last_polled_at,omitempty"`
	File           *OutputFile                          `json:"file,omitempty"`
}

type videoGenerationKey struct{}
type VideoGenerationHandler func(context.Context, string, string, modelprovider.VideoGenerationOptions) error

func WithVideoGenerationHandler(ctx context.Context, handler VideoGenerationHandler) context.Context {
	return context.WithValue(ctx, videoGenerationKey{}, handler)
}
func GenerateVideo(ctx context.Context, id, prompt string, options modelprovider.VideoGenerationOptions) error {
	handler, ok := ctx.Value(videoGenerationKey{}).(VideoGenerationHandler)
	if !ok || handler == nil {
		return fmt.Errorf("video generation is unavailable in this conversation")
	}
	return handler(ctx, id, prompt, options)
}
func CloneVideoGenerationTask(task VideoGenerationTask) VideoGenerationTask {
	task.Model = modelprovider.CloneVideoGeneration(task.Model)
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

type VideoGenerationSink struct{ EventSink }

func (VideoGenerationSink) SupportsVideoGeneration() bool { return true }

type MediaGenerationSink struct{ EventSink }

func (MediaGenerationSink) SupportsImageGeneration() bool { return true }
func (MediaGenerationSink) SupportsVideoGeneration() bool { return true }
