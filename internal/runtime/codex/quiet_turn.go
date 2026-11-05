package codex

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine/contract"
)

const quietTurnInterval = 2 * time.Minute

// Check native state without delaying a live notification or cancellation.
func readQuietTurn(ctx context.Context, client *appServerClient, threadID, turnID string) <-chan map[string]any {
	done := make(chan map[string]any, 1)
	go func() {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		done <- readStoredTurn(ctx, client, threadID, turnID)
	}()
	return done
}

func readStoredTurn(ctx context.Context, client *appServerClient, threadID, turnID string) map[string]any {
	raw, err := client.probe(ctx, "thread/read", map[string]any{"threadId": threadID, "includeTurns": false})
	var state struct {
		Thread struct {
			ID     string
			Status struct{ Type string }
		}
	}
	if err != nil || json.Unmarshal(raw, &state) != nil || state.Thread.ID != threadID {
		return nil
	}
	threadStatus := state.Thread.Status.Type
	if threadStatus != "idle" && threadStatus != "systemError" {
		return nil
	}
	raw, err = client.probe(ctx, "thread/turns/list", map[string]any{
		"threadId": threadID, "sortDirection": "desc", "limit": 1, "itemsView": "full",
	})
	var page struct{ Data []map[string]any }
	if err != nil || json.Unmarshal(raw, &page) != nil || len(page.Data) != 1 {
		return nil
	}
	turn := page.Data[0]
	// Older rollouts can infer a status from the end of the file. Require a
	// recorded completion time before treating the historical status as final.
	completedAt, _ := turn["completedAt"].(float64)
	if appServerString(turn, "id") != turnID || completedAt <= 0 {
		return nil
	}
	status := appServerString(turn, "status")
	if threadStatus == "systemError" && status != "failed" {
		return nil
	}
	switch status {
	case "completed", "failed", "interrupted":
		return turn
	}
	return nil
}

type quietTurnHistory []contract.TurnEvent

// A recovered failure can reach the adapter through the event subscription or
// PromptTurn's return. Carry its items on both paths without delaying the error.
type quietTurnFailure struct {
	error
	history quietTurnHistory
}

func (e *quietTurnFailure) Unwrap() error { return e.error }

func (h quietTurnHistory) failure(err error) error {
	if h == nil {
		return err
	}
	return &quietTurnFailure{error: err, history: h}
}

func quietTurnItems(turn map[string]any) quietTurnHistory {
	if appServerString(turn, "itemsView") != "full" {
		return nil
	}
	var history quietTurnHistory
	items, _ := turn["items"].([]any)
	for _, entry := range items {
		item, _ := entry.(map[string]any)
		id := appServerString(item, "id")
		if id == "" {
			continue
		}
		kind := appServerString(item, "type")
		if kind == "agentMessage" {
			text, _ := item["text"].(string)
			text = historicalDisplayText(text)
			if text != "" {
				phase := appServerString(item, "phase")
				history = append(history, contract.TurnEvent{Kind: contract.TurnEventTextDelta, ItemID: id, Phase: phase, Text: text})
			}
			continue
		}
		// Web searches have no native status field. A stored search is
		// finished on a successful turn. Other tools report their own status.
		status := appServerToolStatus(item, "")
		finishedSearch := kind == "webSearch" && status == "" && appServerString(turn, "status") == "completed"
		if !finishedSearch && activity.TerminalToolStatus(status) == "" {
			continue
		}
		if kind == "commandExecution" {
			output := appServerString(item, "aggregatedOutput")
			if output == "" {
				output = appServerString(item, "output")
			}
			item = cloneAppServerParams(item)
			item["aggregatedOutput"] = historicalDisplayText(output)
			delete(item, "output")
		}
		end, ok := nativeToolEvent(item, true)
		if !ok {
			continue
		}
		start, _ := nativeToolEvent(item, false)
		start.ToolStatus = "started"
		history = append(history,
			contract.TurnEvent{Kind: contract.TurnEventToolCallStart, Tool: toolActivity(start)},
			contract.TurnEvent{Kind: contract.TurnEventToolCallUpdate, Tool: toolActivity(end)})
	}
	if len(history) == 0 {
		return nil
	}
	return history
}

func historicalDisplayText(text string) string {
	if strings.Contains(text, structuredOutputPrefix) {
		text, _, _ = decodeStructuredCommandOutput(text)
	}
	return text
}
