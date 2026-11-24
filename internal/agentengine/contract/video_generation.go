package contract

import (
	"context"
	"csgclaw/internal/modelprovider"
	"fmt"
)

const OutputItemVideoGeneration OutputItemKind = "video_generation"

const OutputItemVideoFeedback OutputItemKind = "video_generation_feedback"

// VideoGenerationFeedback reports a terminal upstream failure after the tool acknowledged the job.
type VideoGenerationFeedback struct {
	Retry   VideoGenerationRetry
	Code    string
	Message string
}

// VideoGenerationRetry carries trusted attempt state across a live follow-up.
type VideoGenerationRetry struct {
	AttemptID string
	CardID    string
	Prompt    string
	Model     modelprovider.VideoGenerationConfig
	Size      string
	Seconds   int
	Indices   []int
	Reference *modelprovider.ReferenceImage `json:"-"`
}

type VideoGenerationTask struct {
	ModelStartedAt   string                               `json:"model_started_at,omitempty"`
	ModelCompletedAt string                               `json:"model_completed_at,omitempty"`
	StartedAt        string                               `json:"started_at,omitempty"`
	EndedAt          string                               `json:"ended_at,omitempty"`
	ErrorDetails     *modelprovider.VideoGenerationError  `json:"error_details,omitempty"`
	ID               string                               `json:"id"`
	RequestID        string                               `json:"request_id,omitempty"`
	Prompt           string                               `json:"prompt"`
	Model            *modelprovider.VideoGenerationConfig `json:"model,omitempty"`
	Options          modelprovider.VideoGenerationOptions `json:"options,omitempty"`
	State            string                               `json:"state"`
	Error            string                               `json:"error,omitempty"`
	UpstreamID       string                               `json:"upstream_id,omitempty"`
	UpstreamStatus   string                               `json:"upstream_status,omitempty"`
	LastPolledAt     string                               `json:"last_polled_at,omitempty"`
	File             *OutputFile                          `json:"file,omitempty"`
}

// VideoRetryError identifies a definitively rejected attempt that a tool call can replace.
type VideoRetryError struct {
	AttemptID string
	Err       error
}

func (e *VideoRetryError) Error() string { return e.Err.Error() }
func (e *VideoRetryError) Unwrap() error { return e.Err }

type videoGenerationKey struct{}
type VideoGenerationHandler func(context.Context, string, string, modelprovider.VideoGenerationOptions, ...int) error

func WithVideoGenerationHandler(ctx context.Context, handler VideoGenerationHandler) context.Context {
	return context.WithValue(ctx, videoGenerationKey{}, handler)
}
func GenerateVideo(ctx context.Context, id, prompt string, options modelprovider.VideoGenerationOptions, references ...int) error {
	handler, ok := ctx.Value(videoGenerationKey{}).(VideoGenerationHandler)
	if !ok || handler == nil {
		return fmt.Errorf("video generation is unavailable in this conversation")
	}
	return handler(ctx, id, prompt, options, references...)
}

type videoPreflightKey struct{}

func WithVideoGenerationPreflight(ctx context.Context, preflight func(modelprovider.VideoGenerationOptions) (modelprovider.VideoGenerationOptions, error)) context.Context {
	return context.WithValue(ctx, videoPreflightKey{}, preflight)
}

func PrepareVideoGeneration(ctx context.Context, options modelprovider.VideoGenerationOptions) (modelprovider.VideoGenerationOptions, error) {
	if preflight, ok := ctx.Value(videoPreflightKey{}).(func(modelprovider.VideoGenerationOptions) (modelprovider.VideoGenerationOptions, error)); ok {
		return preflight(options)
	}
	return options, nil
}

type videoRetryPreflightKey struct{}

type VideoRetryPreflight func(string, modelprovider.VideoGenerationOptions, ...int) error

func WithVideoRetryPreflight(ctx context.Context, preflight VideoRetryPreflight) context.Context {
	return context.WithValue(ctx, videoRetryPreflightKey{}, preflight)
}

func CheckVideoRetry(ctx context.Context, prompt string, options modelprovider.VideoGenerationOptions, references ...int) error {
	if check, ok := ctx.Value(videoRetryPreflightKey{}).(VideoRetryPreflight); ok && options.RetryOf != "" {
		return check(prompt, options, references...)
	}
	return nil
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
