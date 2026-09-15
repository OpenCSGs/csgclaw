package agentengine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelprovider"
)

type imageGenerator func(context.Context, *modelprovider.ImageGenerationConfig, string) (modelprovider.GeneratedImage, error)

func (c *conversations) imageHandler(turn *activeTurn, sink EventSink, ref *modelprovider.ImageGenerationConfig) contract.ImageGenerationHandler {
	var mu sync.Mutex
	tasks := map[string]error{}
	return func(ctx context.Context, id, prompt string) error {
		mu.Lock()
		defer mu.Unlock()
		if err, ok := tasks[id]; ok {
			return err
		}
		task := contract.ImageGenerationTask{ID: id, Prompt: strings.TrimSpace(prompt), Model: modelprovider.CloneImageGeneration(ref)}
		err := c.generateImage(ctx, turn, sink, task)
		tasks[id] = err
		return err
	}
}

func (c *conversations) generateImage(ctx context.Context, turn *activeTurn, sink EventSink, task contract.ImageGenerationTask) error {
	capability, ok := sink.(interface{ SupportsImageGeneration() bool })
	if !ok || !capability.SupportsImageGeneration() {
		return fmt.Errorf("image generation is available in CSGClaw web conversations only")
	}
	if task.ID == "" || strings.TrimSpace(task.Prompt) == "" || len(task.Prompt) > 32000 {
		return fmt.Errorf("invalid image prompt")
	}
	emit := func() error {
		copy := task
		return c.engine.recordAndEmit(ctx, turn, sink, TurnEvent{Kind: TurnEventOutputItem, Output: &OutputItem{Kind: contract.OutputItemImageGeneration, Payload: copy}})
	}
	task.Error = ""
	task.ErrorDetails = nil
	task.State = "generating"
	if task.File != nil {
		task.State = "delivering"
	}
	if err := emit(); err != nil {
		return err
	}
	if task.File == nil {
		var result modelprovider.GeneratedImage
		var err error
		if c.engine.generateImage == nil {
			err = fmt.Errorf("image_model_unavailable")
		} else {
			imageCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
			result, err = c.engine.generateImage(imageCtx, task.Model, task.Prompt)
			cancel()
		}
		if err != nil {
			task.State = "failed"
			task.Error = "image_generation_failed"
			if err.Error() == "image_model_not_configured" || err.Error() == "image_model_unavailable" {
				task.Error = err.Error()
			}
			var providerError *modelprovider.ImageGenerationError
			if errors.As(err, &providerError) {
				details := *providerError
				task.ErrorDetails = &details
				switch details.Code {
				case "moderation_blocked", "content_policy_violation":
					task.Error = "image_generation_blocked"
					if details.Stage == "output" {
						task.Error = "image_generation_output_blocked"
					}
				case "upstream_timeout":
					task.Error = "image_generation_timeout"
				}
			}
			if ctx.Err() != nil {
				task.Error = "image_generation_canceled"
			}
			// Persist the terminal state even if Stop canceled the provider request.
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			ctx = cleanupCtx
			if emitErr := emit(); emitErr != nil {
				return emitErr
			}
			if task.ErrorDetails != nil {
				return fmt.Errorf("%s: %w", task.Error, task.ErrorDetails)
			}
			return fmt.Errorf("%s", task.Error)
		}
		name := "generated-image.png"
		if result.MediaType == "image/jpeg" {
			name = "generated-image.jpg"
		}
		file, err := contract.NewOutputFile(ctx, contract.OutputFileMetadata{Name: name, MediaType: result.MediaType, SizeBytes: int64(len(result.Data))}, bytes.NewReader(result.Data))
		if err != nil {
			return err
		}
		files, fileErr := c.engine.files.RegisterTurnFiles(c.agentID, turn.request.ConversationKey, turn.request.ID, []*OutputFile{file})
		if fileErr != nil {
			return fileErr
		}
		task.File = &files[0]
	}
	task.State = "completed"
	if err := emit(); err != nil {
		task.State = "delivery_failed"
		task.Error = "image_delivery_failed"
		_ = emit()
		return fmt.Errorf("image_delivery_failed: image already generated; retry delivery only")
	}
	return nil
}
