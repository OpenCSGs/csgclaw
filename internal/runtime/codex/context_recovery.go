package codex

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"csgclaw/internal/activity"
)

var errContextCompaction = errors.New("Conversation compaction failed. Your history is preserved. Retry or choose a model with a larger context window.")

type contextWindowError struct{}

func (*contextWindowError) Error() string {
	return "The conversation exceeds this model's context window. Split large inputs or check the model capacity."
}
func isContextWindowError(err error) bool {
	var target *contextWindowError
	return errors.As(err, &target)
}
func contextErrorInfo(value any) bool {
	switch v := value.(type) {
	case string:
		if strings.EqualFold(v, "contextWindowExceeded") || v == "context_length_exceeded" {
			return true
		}
		var decoded map[string]any
		return json.Unmarshal([]byte(v), &decoded) == nil && contextErrorInfo(decoded)
	case map[string]any:
		for _, key := range []string{"code", "codexErrorInfo", "error", "message"} {
			if contextErrorInfo(v[key]) {
				return true
			}
		}
		for key := range v {
			if strings.EqualFold(key, "contextWindowExceeded") {
				return true
			}
		}
	}
	return false
}

// Recovery is only safe before assistant output or tool execution. Continue from
// the existing history with empty input, never append the user's request again.
func (m *appServerManager) recoverContext(ctx context.Context, live *liveSession, waiter **appServerTurnWaiter) (PromptResponse, error) {
	if err := ctx.Err(); err != nil {
		return PromptResponse{}, err
	}
	threadID := (*waiter).threadID
	(*waiter).mu.RLock()
	visible := (*waiter).visibleActivity
	(*waiter).mu.RUnlock()
	if visible {
		return PromptResponse{}, &contextWindowError{}
	}
	newWaiter := func() error {
		live.removeAppServerTurnWaiter(threadID, *waiter)
		next, err := live.registerAppServerTurnWaiter(threadID)
		if err != nil {
			return err
		}
		*waiter = next
		live.setAppServerTurnContext(threadID, next, ctx)
		return nil
	}
	if err := newWaiter(); err != nil {
		return PromptResponse{}, err
	}
	live.mu.Lock()
	if live.compactingThreads == nil {
		live.compactingThreads = make(map[string]bool)
	}
	live.compactingThreads[threadID] = true
	live.mu.Unlock()
	defer func() {
		live.mu.Lock()
		delete(live.compactingThreads, threadID)
		pending := live.contextUsage[threadID].Compacting
		live.mu.Unlock()
		if pending {
			m.publishContextUsage(live.spec.RuntimeID, live, threadID, nil, "item/completed")
		}
	}()
	m.publishContextUsage(live.spec.RuntimeID, live, threadID, nil, "item/started")
	if _, err := live.appClient.request(ctx, "thread/compact/start", map[string]any{"threadId": threadID}); err != nil {
		if ctx.Err() != nil {
			return PromptResponse{}, ctx.Err()
		}
		return PromptResponse{}, errContextCompaction
	}
	if _, err := m.waitAppServerTurn(ctx, live, *waiter); err != nil {
		if ctx.Err() != nil {
			return PromptResponse{}, ctx.Err()
		}
		return PromptResponse{}, errContextCompaction
	}
	live.mu.Lock()
	delete(live.compactingThreads, threadID)
	live.mu.Unlock()
	if err := newWaiter(); err != nil {
		return PromptResponse{}, err
	}
	raw, err := live.appClient.request(ctx, "turn/start", appServerTurnStartParamsWithInput(live.spec, threadID, []map[string]any{}, ""))
	if err != nil {
		return PromptResponse{}, err
	}
	(*waiter).setTurnID(appServerTurnIDFromResult(raw))
	return m.waitAppServerTurn(ctx, live, *waiter)
}

func contextVisibleEvent(kind activity.RuntimeEventKind) bool {
	return kind == activity.RuntimeEventTextDelta || kind == activity.RuntimeEventToolCallStart || kind == activity.RuntimeEventToolCallUpdate || kind == activity.RuntimeEventFileOutput || kind == activity.RuntimeEventStructuredOutput
}
