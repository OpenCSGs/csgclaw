package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/delivery"
	"csgclaw/internal/im"
	"csgclaw/internal/modelprovider"
)

// RecoverVideoGenerations resumes persisted asynchronous jobs without
// submitting a second provider request. A task that crashed before receiving
// an upstream ID is failed explicitly because automatically replaying its POST
// could create a duplicate billable job.
func (h *Handler) RecoverVideoGenerations(ctx context.Context) error {
	if h.im == nil || h.participant == nil || h.agentEngine == nil {
		return nil
	}
	store, err := delivery.NewIMTranscriptStore(h.im, h.participant, h.agentEngine)
	if err != nil {
		return err
	}
	renderer := delivery.NewTranscriptRenderer(store)
	seen := map[string]struct{}{}
	for _, room := range h.im.ListRoomsWithOptions(im.ListMessagesOptions{IncludeThreadReplies: true}) {
		for _, message := range room.Messages {
			var task contract.VideoGenerationTask
			var turn channel.TurnContext
			body, bodyErr := json.Marshal(message.Metadata["video_generation"])
			route, routeErr := json.Marshal(message.Metadata["video_generation_context"])
			if bodyErr != nil || routeErr != nil || json.Unmarshal(body, &task) != nil || json.Unmarshal(route, &turn) != nil {
				continue
			}
			if !recoverableVideoState(task.State) || task.ID == "" || turn.AgentID == "" || turn.RoomID != room.ID {
				continue
			}
			key := turn.AgentID + "\x00" + string(turn.ConversationKey) + "\x00" + task.ID
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			digest := sha256.Sum256([]byte(key))
			recoveryTurnID := agentengine.TurnID("video-recovery-" + hex.EncodeToString(digest[:16]))
			if strings.TrimSpace(task.UpstreamID) == "" {
				task.State = "failed"
				task.Error = "video_generation_interrupted"
				task.ErrorDetails = &modelprovider.VideoGenerationError{Code: task.Error, Message: "Video submission was interrupted before an upstream task ID was saved. Retry manually to avoid creating a duplicate charged job."}
				if err := store.DeliverVideoGeneration(ctx, turn, task); err != nil {
					slog.Error("mark uncertain video generation failed", "video_task_id", task.ID, "error", err)
				}
				continue
			}
			task.File = nil
			go func(turn channel.TurnContext, recoveryTurnID agentengine.TurnID, task contract.VideoGenerationTask) {
				recoveryCtx, cancel := context.WithTimeout(ctx, 16*time.Minute)
				defer cancel()
				conversation := h.agentEngine.Conversations(turn.AgentID)
				recovery, ok := conversation.(interface {
					RecoverVideoGeneration(context.Context, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult
				})
				if !ok {
					slog.Error("recover video generation", "video_task_id", task.ID, "upstream_id", task.UpstreamID, "error", "video recovery is unavailable")
					return
				}
				result := recovery.RecoverVideoGeneration(recoveryCtx, agentengine.TurnRequest{
					ID: recoveryTurnID, ConversationKey: turn.ConversationKey,
					Input:           []agentengine.InputPart{{Kind: agentengine.InputPartText, Text: task.Prompt}},
					VideoGeneration: &task,
				}, contract.VideoGenerationSink{EventSink: agentengine.EventSinkFunc(func(eventCtx context.Context, event agentengine.TurnEvent) error {
					return renderer.Emit(eventCtx, turn, event)
				})})
				if result.Status != agentengine.TurnSucceeded {
					slog.Error("recover video generation", "video_task_id", task.ID, "upstream_id", task.UpstreamID, "error", fmt.Sprint(result.Error))
				}
			}(turn, recoveryTurnID, task)
		}
	}
	return nil
}

func recoverableVideoState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "generating", "delivering", "queued", "in_progress", "downloading":
		return true
	default:
		return false
	}
}
