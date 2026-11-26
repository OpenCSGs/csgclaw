package codex

import (
	"context"
	"encoding/json"
	"fmt"

	"csgclaw/internal/runtime/turnjournal"
)

type preparingMessage struct{ conversationKey, messageID string }

// Codex persists userMessage.clientId, but accepts duplicate turn/start calls
// with the same ID. Only follow a unique match in the requested thread.
func (m *appServerManager) followMessage(ctx context.Context, live *liveSession, waiter *appServerTurnWaiter, req PromptRequest) (PromptResponse, error) {
	readCtx, cancel := context.WithTimeout(ctx, nativeTurnLookupTimeout)
	defer cancel()
	raw, err := live.appClient.request(readCtx, "thread/read", map[string]any{"threadId": waiter.threadID, "includeTurns": true})
	if err != nil {
		return PromptResponse{}, fmt.Errorf("%w: read Codex history: %w", turnjournal.ErrUnconfirmed, err)
	}
	var history struct {
		Thread struct {
			ID     string
			Status struct{ Type string }
			Turns  []map[string]any
		}
	}
	if err := json.Unmarshal(raw, &history); err != nil || history.Thread.ID != waiter.threadID {
		return PromptResponse{}, fmt.Errorf("%w: invalid history for thread %q", turnjournal.ErrUnconfirmed, waiter.threadID)
	}
	confirmed, err := turnjournal.ConfirmedTurn(live.spec.RuntimeDir, req.ConversationKey, req.ClientUserMessageID)
	if err != nil {
		return PromptResponse{}, err
	}
	var match map[string]any
	for _, turn := range history.Thread.Turns {
		matches := confirmed != "" && appServerString(turn, "id") == confirmed
		items, _ := turn["items"].([]any)
		for _, item := range items {
			msg, _ := item.(map[string]any)
			if appServerString(msg, "type") == "userMessage" && appServerString(msg, "clientId") == req.ClientUserMessageID {
				matches = true
				break
			}
		}
		if matches {
			if match != nil || (confirmed != "" && appServerString(turn, "id") != confirmed) {
				return PromptResponse{}, fmt.Errorf("%w: conflicting native turns for %q", turnjournal.ErrUnconfirmed, req.ClientUserMessageID)
			}
			match = turn
		}
	}
	id := appServerString(match, "id")
	if id == "" {
		return PromptResponse{}, fmt.Errorf("%w: no matching Codex turn for %q", turnjournal.ErrUnconfirmed, req.ClientUserMessageID)
	}
	waiter.setTurnID(id)
	live.trackAppServerTurn(waiter.threadID, id)
	if req.OnAccepted != nil {
		req.OnAccepted()
	}
	switch status := appServerString(match, "status"); status {
	case "completed":
		rawItems, _ := match["items"].([]any)
		for _, v := range rawItems {
			item, _ := v.(map[string]any)
			switch appServerString(item, "tool") {
			case appServerPublishFileToolName, appServerUploadFileToolName, "csgclaw_generate_image", "csgclaw_generate_video":
				return PromptResponse{}, fmt.Errorf("%w: turn %q published files whose delivery cannot be reconstructed from Codex history", turnjournal.ErrUnconfirmed, id)
			}
		}
		items := quietTurnItems(match)
		completedAt, _ := match["completedAt"].(float64)
		if completedAt <= 0 || len(items) == 0 {
			return PromptResponse{}, fmt.Errorf("%w: no confirmed completed output for turn %q", turnjournal.ErrUnconfirmed, id)
		}
		return PromptResponse{StopReason: StopReasonEndTurn, history: items}, nil
	case "inProgress":
		if history.Thread.Status.Type != "active" || appServerString(history.Thread.Turns[len(history.Thread.Turns)-1], "id") != id {
			return PromptResponse{}, fmt.Errorf("%w: Codex does not report turn %q as active", turnjournal.ErrUnconfirmed, id)
		}
		return m.waitAppServerTurn(ctx, live, waiter)
	case "failed", "interrupted":
		return PromptResponse{}, fmt.Errorf("original Codex turn %q ended with status %s", id, status)
	default:
		return PromptResponse{}, fmt.Errorf("%w: original Codex turn %q has status %q", turnjournal.ErrUnconfirmed, id, status)
	}
}
