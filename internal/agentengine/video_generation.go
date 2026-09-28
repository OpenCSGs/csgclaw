package agentengine

import (
	"context"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelprovider"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type videoGenerator func(context.Context, *modelprovider.VideoGenerationConfig, string, modelprovider.VideoGenerationOptions) (modelprovider.GeneratedVideo, error)

var videoDeliveryRetryInterval = time.Second

// RecoverVideoGeneration resumes a persisted asynchronous video job without
// entering normal conversation admission. The caller owns the delivery
// context, so recovered progress can keep pointing at the original chat turn.
func (c *conversations) RecoverVideoGeneration(ctx context.Context, request TurnRequest, sink EventSink) TurnResult {
	if c == nil || c.engine == nil || c.agentID == "" || request.ID == "" || request.ConversationKey == "" || request.VideoGeneration == nil {
		return failedResult(ErrorInvalidRequest, "agent ID, turn ID, conversation key, and video generation task are required")
	}
	if err := ctx.Err(); err != nil {
		return resultFromContext(ctx, err)
	}
	request = cloneTurnRequest(request)
	if request.VideoGeneration.Model == nil && c.engine.agents != nil {
		if selected, err := c.engine.agents.Get(ctx, c.agentID, AgentGetOptions{}); err == nil {
			request.VideoGeneration.Model = selected.Spec.Model.VideoGeneration
		}
	}
	turnCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	turn := &activeTurn{ctx: turnCtx, cancel: cancel, agentID: c.agentID, request: request}
	defer c.engine.files.DeleteTurn(c.agentID, request.ConversationKey, request.ID)
	if err := c.generateVideo(turnCtx, turn, sink, *request.VideoGeneration); err != nil {
		return failedResult(ErrorRuntimeFailed, err.Error())
	}
	return TurnResult{Status: TurnSucceeded, Dispatched: true}
}

func (c *conversations) videoHandler(turn *activeTurn, sink EventSink, ref *modelprovider.VideoGenerationConfig) contract.VideoGenerationHandler {
	var mu sync.Mutex
	tasks := map[string]struct{}{}
	return func(ctx context.Context, id, prompt string, options modelprovider.VideoGenerationOptions) error {
		capability, ok := sink.(interface{ SupportsVideoGeneration() bool })
		if !ok || !capability.SupportsVideoGeneration() {
			return fmt.Errorf("video generation is available in CSGClaw web conversations only")
		}
		prompt = strings.TrimSpace(prompt)
		if id == "" || prompt == "" || len(prompt) > 32000 {
			return fmt.Errorf("invalid video prompt")
		}
		mu.Lock()
		if _, ok := tasks[id]; ok {
			mu.Unlock()
			return nil
		}
		tasks[id] = struct{}{}
		mu.Unlock()

		task := contract.VideoGenerationTask{ID: id, Prompt: prompt, Model: modelprovider.CloneVideoGeneration(ref), Options: options}
		// Video providers commonly take longer than Codex's dynamic-tool request
		// window. Detach the generation from the turn so the tool can acknowledge
		// submission immediately and the terminal event can arrive later.
		backgroundCtx := context.WithoutCancel(ctx)
		go func() {
			if err := c.generateVideo(backgroundCtx, turn, sink, task); err != nil {
				slog.Error("background video generation failed", "agent_id", c.agentID, "turn_id", turn.request.ID, "video_task_id", task.ID, "error", err)
			}
		}()
		return nil
	}
}

func (c *conversations) generateVideo(ctx context.Context, turn *activeTurn, sink EventSink, task contract.VideoGenerationTask) error {
	capability, ok := sink.(interface{ SupportsVideoGeneration() bool })
	if !ok || !capability.SupportsVideoGeneration() {
		return fmt.Errorf("video generation is available in CSGClaw web conversations only")
	}
	if task.ID == "" || strings.TrimSpace(task.Prompt) == "" || len(task.Prompt) > 32000 {
		return fmt.Errorf("invalid video prompt")
	}
	emit := func() error {
		copy := task
		return c.engine.recordAndEmit(ctx, turn, sink, TurnEvent{Kind: TurnEventOutputItem, Output: &OutputItem{Kind: contract.OutputItemVideoGeneration, Payload: copy}})
	}
	failDelivery := func(stage string, cause error) error {
		task.State = "delivery_failed"
		task.Error = "video_delivery_failed"
		task.ErrorDetails = &modelprovider.VideoGenerationError{Code: "video_delivery_failed", Message: cause.Error(), Stage: stage}
		// A failed file must not be attached again while reporting the failure;
		// otherwise the status update repeats the same delivery error and the UI
		// remains stuck at its previous state.
		task.File = nil
		failureCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		previousCtx := ctx
		ctx = failureCtx
		emitErr := emit()
		ctx = previousCtx
		if emitErr != nil {
			slog.Error("deliver video delivery failure status", "agent_id", c.agentID, "turn_id", turn.request.ID, "video_task_id", task.ID, "stage", stage, "cause", cause, "error", emitErr)
			return fmt.Errorf("video_delivery_failed at %s: %v; report failure: %w", stage, cause, emitErr)
		}
		return fmt.Errorf("video_delivery_failed at %s: %w", stage, cause)
	}
	task.Error, task.ErrorDetails, task.State = "", nil, "generating"
	if task.File != nil {
		task.State = "delivering"
	}
	if err := emit(); err != nil {
		return err
	}
	if task.File == nil {
		if c.engine.generateVideo == nil {
			task.State = "failed"
			task.Error = "video_model_unavailable"
			_ = emit()
			return errors.New(task.Error)
		}
		videoCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		options := task.Options
		options.RequestID = task.ID
		options.ResumeID = task.UpstreamID
		options.Progress = func(progress modelprovider.VideoGenerationProgress) {
			previousStatus := task.UpstreamStatus
			previousPoll, _ := time.Parse(time.RFC3339, task.LastPolledAt)
			if previousStatus == progress.Status && !previousPoll.IsZero() && progress.UpdatedAt.Sub(previousPoll) < 30*time.Second {
				return
			}
			task.UpstreamID = progress.ID
			task.UpstreamStatus = progress.Status
			task.LastPolledAt = progress.UpdatedAt.Format(time.RFC3339)
			slog.Info("video generation progress", "agent_id", c.agentID, "turn_id", turn.request.ID, "video_task_id", task.ID, "upstream_id", progress.ID, "status", progress.Status)
			if err := emit(); err != nil {
				slog.Warn("deliver video generation progress", "agent_id", c.agentID, "turn_id", turn.request.ID, "video_task_id", task.ID, "error", err)
			}
		}
		result, err := c.engine.generateVideo(videoCtx, task.Model, task.Prompt, options)
		if err != nil {
			cancel()
			task.State, task.Error = "failed", "video_generation_failed"
			if err.Error() == "video_model_not_configured" || err.Error() == "video_model_unavailable" {
				task.Error = err.Error()
			}
			var providerErr *modelprovider.VideoGenerationError
			if errors.As(err, &providerErr) {
				details := *providerErr
				task.ErrorDetails = &details
				if details.Code == "content_policy_violation" {
					task.Error = "video_generation_blocked"
				}
			}
			if ctx.Err() != nil {
				task.Error = "video_generation_canceled"
			}
			cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cleanupCancel()
			ctx = cleanupCtx
			if emitErr := emit(); emitErr != nil {
				slog.Error("deliver terminal video generation failure", "agent_id", c.agentID, "turn_id", turn.request.ID, "video_task_id", task.ID, "error", emitErr)
			}
			return fmt.Errorf("%s: %w", task.Error, err)
		}
		name := "generated-video.mp4"
		if result.MediaType == "video/webm" {
			name = "generated-video.webm"
		}
		if result.Content == nil || result.SizeBytes <= 0 {
			cancel()
			return failDelivery("snapshot", fmt.Errorf("video provider returned empty content"))
		}
		file, err := contract.NewOutputFile(videoCtx, contract.OutputFileMetadata{Name: name, MediaType: result.MediaType, SizeBytes: result.SizeBytes}, result.Content)
		closeErr := result.Content.Close()
		cancel()
		if err != nil {
			return failDelivery("snapshot", err)
		}
		if closeErr != nil {
			return failDelivery("snapshot", closeErr)
		}
		files, fileErr := c.engine.files.RegisterTurnFiles(c.agentID, turn.request.ConversationKey, turn.request.ID, []*OutputFile{file})
		if fileErr != nil {
			return failDelivery("register", fileErr)
		}
		if len(files) != 1 {
			return failDelivery("register", fmt.Errorf("generated video file was not registered"))
		}
		task.File = &files[0]
	}
	task.State = "completed"
	var deliveryErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if deliveryErr = emit(); deliveryErr == nil {
			return nil
		}
		slog.Warn("deliver completed video", "agent_id", c.agentID, "turn_id", turn.request.ID, "video_task_id", task.ID, "attempt", attempt, "error", deliveryErr)
		if attempt < 3 {
			timer := time.NewTimer(videoDeliveryRetryInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return failDelivery("attachment", errors.Join(deliveryErr, ctx.Err()))
			case <-timer.C:
			}
		}
	}
	return failDelivery("attachment", deliveryErr)
}
