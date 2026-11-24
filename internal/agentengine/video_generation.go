package agentengine

import (
	"context"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelprovider"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

type videoGenerator func(context.Context, *modelprovider.VideoGenerationConfig, string, modelprovider.VideoGenerationOptions) (modelprovider.GeneratedVideo, error)

var videoDeliveryRetryInterval = time.Second

const videoSubmissionStatusWindow = 30 * time.Second

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
	if err := c.generateVideo(turnCtx, turn, sink, *request.VideoGeneration, nil); err != nil {
		return failedResult(ErrorRuntimeFailed, err.Error())
	}
	return TurnResult{Status: TurnSucceeded, Dispatched: true}
}

func (c *conversations) prepareVideoTool(ctx context.Context, sink EventSink, ref *modelprovider.VideoGenerationConfig) (context.Context, string) {
	support, supported := sink.(interface{ SupportsVideoGeneration() bool })
	if ref == nil || !supported || !support.SupportsVideoGeneration() {
		return contract.WithVideoGenerationPreflight(ctx, func(options modelprovider.VideoGenerationOptions) (modelprovider.VideoGenerationOptions, error) {
			return options, fmt.Errorf("video generation is unavailable in this conversation")
		}), ""
	}

	capabilities, err := c.engine.videoCapabilities(ctx, ref, false)
	ctx = contract.WithVideoGenerationPreflight(ctx, func(options modelprovider.VideoGenerationOptions) (modelprovider.VideoGenerationOptions, error) {
		if err != nil {
			return options, err
		}
		current, err := c.engine.videoCapabilities(ctx, ref, true)
		if err != nil {
			return options, err
		}
		return modelprovider.ResolveVideoGenerationOptions(options, current)
	})
	if err != nil {
		return ctx, "The configured video model is unavailable for this turn."
	}
	defaults, _ := modelprovider.ResolveVideoGenerationOptions(modelprovider.VideoGenerationOptions{}, capabilities)
	return ctx, fmt.Sprintf(
		"Current video tool model: %q. Supported size values: %q. Supported seconds values: %v. An empty list means the provider did not advertise values for that parameter. Leave unspecified parameters omitted. Defaults when advertised: size=%q, seconds=%d (zero means unspecified). If an explicit value is unsupported, follow the video tool recovery instructions for interactive confirmation or selection.",
		ref.ModelID, capabilities.Sizes, capabilities.Seconds, defaults.Size, defaults.Seconds,
	)
}

func (c *conversations) videoHandler(turn *activeTurn, sink EventSink, ref *modelprovider.VideoGenerationConfig, input []InputPart, inherited *contract.VideoGenerationRetry) (contract.VideoGenerationHandler, contract.VideoRetryPreflight) {
	var mu sync.Mutex
	type submission struct {
		done chan struct{}
		err  error
	}
	tasks := map[string]*submission{}
	rejected := map[string]contract.VideoGenerationRetry{}
	if inherited != nil && ref != nil && inherited.Model == *ref {
		rejected[inherited.AttemptID] = *inherited
	}
	wait := func(ctx context.Context, result *submission) error {
		select {
		case <-result.done:
			return result.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	validateRetry := func(prompt string, options modelprovider.VideoGenerationOptions, indices []int) (contract.VideoGenerationRetry, error) {
		previous, ok := rejected[options.RetryOf]
		if !ok || previous.Prompt != strings.TrimSpace(prompt) || !slices.Equal(previous.Indices, indices) ||
			(previous.Size == options.Size && previous.Seconds == options.Seconds) {
			return contract.VideoGenerationRetry{}, fmt.Errorf("retry_of must identify a rejected attempt for the same prompt and references with revised parameters")
		}
		return previous, nil
	}
	checkRetry := func(prompt string, options modelprovider.VideoGenerationOptions, indices ...int) error {
		mu.Lock()
		defer mu.Unlock()
		_, err := validateRetry(prompt, options, indices)
		return err
	}
	return func(ctx context.Context, id, prompt string, options modelprovider.VideoGenerationOptions, indices ...int) error {
		capability, ok := sink.(interface{ SupportsVideoGeneration() bool })
		if !ok || !capability.SupportsVideoGeneration() {
			return fmt.Errorf("video generation is available in CSGClaw web conversations only")
		}
		prompt = strings.TrimSpace(prompt)
		if id == "" || prompt == "" || len(prompt) > 32000 {
			return fmt.Errorf("invalid video prompt")
		}
		mu.Lock()
		if result := tasks[id]; result != nil {
			mu.Unlock()
			return wait(ctx, result)
		}
		attempt := contract.VideoGenerationRetry{
			AttemptID: id, CardID: id, Prompt: prompt, Size: options.Size,
			Seconds: options.Seconds, Indices: slices.Clone(indices),
		}
		if ref != nil {
			attempt.Model = *ref
		}
		if options.RetryOf != "" {
			previous, err := validateRetry(prompt, options, indices)
			if err != nil {
				mu.Unlock()
				return err
			}
			attempt.CardID = previous.CardID
			attempt.Reference = previous.Reference
			delete(rejected, options.RetryOf)
		} else {
			references, _, err := loadReferenceImages(ctx, input, indices, 1)
			if err != nil {
				mu.Unlock()
				return err
			}
			if len(references) > 0 {
				attempt.Reference = &references[0]
			}
		}
		result := &submission{done: make(chan struct{})}
		tasks[id] = result
		mu.Unlock()

		task := contract.VideoGenerationTask{StartedAt: time.Now().UTC().Format(time.RFC3339Nano), ID: attempt.CardID, Prompt: prompt, Model: modelprovider.CloneVideoGeneration(ref), Options: options}
		if attempt.CardID != id {
			// Each upstream attempt needs a new idempotency key even when its
			// progress replaces the previous attempt on the same card.
			task.RequestID = id
		}
		polledFailure := false
		var statusWindow *time.Timer
		var once sync.Once
		finish := func(err error) {
			once.Do(func() {
				var upstream *modelprovider.VideoGenerationError
				if errors.As(err, &upstream) && (polledFailure || (statusWindow == nil && (upstream.HTTPStatus == http.StatusBadRequest || upstream.HTTPStatus == http.StatusUnprocessableEntity))) {
					mu.Lock()
					rejected[id] = attempt
					mu.Unlock()
					err = &contract.VideoRetryError{AttemptID: id, Err: err}
				}
				result.err = err
				close(result.done)
			})
		}
		task.Options.Progress = func(progress modelprovider.VideoGenerationProgress) {
			if progress.ID != "" && statusWindow == nil {
				// A queued job can still reject input. Give it a bounded window
				// to report that rejection while the Agent can act on it.
				statusWindow = time.AfterFunc(videoSubmissionStatusWindow, func() { finish(nil) })
			}
			if progress.Status == "failed" {
				polledFailure = true
			}
			if progress.StatusChecked {
				finish(nil)
			}
		}
		// Keep polling and delivery independent of the turn once the tool has
		// received the first upstream status or a submission failure.
		backgroundCtx := context.WithoutCancel(ctx)
		go func() {
			err := c.generateVideo(backgroundCtx, turn, sink, task, attempt.Reference)
			if statusWindow != nil {
				statusWindow.Stop()
			}
			finish(err)
			var upstream *modelprovider.VideoGenerationError
			if err != nil && result.err == nil && polledFailure && errors.As(err, &upstream) {
				feedback := contract.VideoGenerationFeedback{Retry: attempt, Code: upstream.Code, Message: upstream.Message}
				feedbackCtx, cancel := context.WithTimeout(backgroundCtx, 10*time.Second)
				defer cancel()
				event := TurnEvent{Kind: TurnEventOutputItem, Output: &OutputItem{Kind: contract.OutputItemVideoFeedback, Payload: feedback}}
				if emitErr := emitTurnEvent(feedbackCtx, sink, event); emitErr != nil {
					slog.Warn("deliver video generation feedback", "video_task_id", task.ID, "error", emitErr)
				}
			}
			if err != nil {
				slog.Error("background video generation failed", "agent_id", c.agentID, "turn_id", turn.request.ID, "video_task_id", task.ID, "error", err)
			}
		}()
		return wait(ctx, result)
	}, checkRetry
}

func (c *conversations) generateVideo(ctx context.Context, turn *activeTurn, sink EventSink, task contract.VideoGenerationTask, reference *modelprovider.ReferenceImage) error {
	capability, ok := sink.(interface{ SupportsVideoGeneration() bool })
	if !ok || !capability.SupportsVideoGeneration() {
		return fmt.Errorf("video generation is available in CSGClaw web conversations only")
	}
	if task.ID == "" || strings.TrimSpace(task.Prompt) == "" || len(task.Prompt) > 32000 {
		return fmt.Errorf("invalid video prompt")
	}
	if task.StartedAt == "" {
		task.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	onProgress := task.Options.Progress
	task.Options.Progress = nil
	task.EndedAt = ""
	emit := func() error {
		switch task.State {
		case "completed", "failed", "delivery_failed":
			task.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
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
		options.Reference = reference
		options.RequestID = task.RequestID
		if options.RequestID == "" {
			options.RequestID = task.ID
		}
		options.ResumeID = task.UpstreamID
		options.Progress = func(progress modelprovider.VideoGenerationProgress) {
			if progress.Status == "submitting" && task.ModelStartedAt == "" {
				task.ModelStartedAt = progress.UpdatedAt.UTC().Format(time.RFC3339Nano)
			}
			if progress.Status == "completed" && task.ModelCompletedAt == "" {
				task.ModelCompletedAt = progress.UpdatedAt.UTC().Format(time.RFC3339Nano)
			}
			if onProgress != nil {
				onProgress(progress)
			}
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
