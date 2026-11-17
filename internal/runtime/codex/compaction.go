package codex

import (
	"context"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelcap"
	agentruntime "csgclaw/internal/runtime"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var errCompactionIdentityChanged = errors.New("saved conversation identity changed")

func (a *ConversationAdapter) Compact(ctx context.Context, req contract.CompactRequest) (contract.CompactResult, error) {
	backend, ok := a.runtime.(interface {
		CompactConversation(context.Context, string, contract.CompactRequest) (contract.CompactResult, error)
	})
	if !ok {
		return contract.CompactResult{Status: "unsupported"}, nil
	}
	return backend.CompactConversation(ctx, a.runtimeID, req)
}
func (c *configuredConversation) Compact(ctx context.Context, req contract.CompactRequest) (contract.CompactResult, error) {
	state := c.runtime.conversationState(c.runtimeID)
	desired, scoped, err := c.runtime.conversationRunSettings(agentruntime.Handle{RuntimeID: c.runtimeID}, req.RoomConfiguration)
	if err != nil {
		return contract.CompactResult{}, err
	}
	release, err := c.runtime.prepareConversation(ctx, agentruntime.Handle{RuntimeID: c.runtimeID}, state, desired, scoped != nil || req.RoomConfiguration == nil)
	if err != nil {
		return contract.CompactResult{}, err
	}
	defer release()
	if scoped != nil {
		ctx = context.WithValue(ctx, roomThreadSettingsKey{}, *scoped)
	}
	return c.RuntimeConversation.(contract.ConversationCompactor).Compact(ctx, req)
}
func (r *Runtime) CompactConversation(ctx context.Context, runtimeID string, req contract.CompactRequest) (contract.CompactResult, error) {
	manager, ok := r.currentSessionManager().(*appServerManager)
	if !ok {
		return contract.CompactResult{Status: "unsupported"}, nil
	}
	live, err := manager.ensureLiveSession(ctx, SessionHandle{RuntimeID: runtimeID})
	if err != nil {
		return contract.CompactResult{}, err
	}
	live.mu.Lock()
	key := ""
	scopedSession := false
	for binding, session := range live.conversationSessions {
		if session != req.Usage.SessionID {
			continue
		}
		logical := contract.LogicalRuntimeSessionKey(binding)
		if logical == string(req.ConversationKey) {
			key = binding
			break
		}
		if strings.HasPrefix(logical, string(req.ConversationKey)+":task:") || strings.HasPrefix(logical, string(req.ConversationKey)+":thread:") {
			scopedSession = true
		}
	}
	live.mu.Unlock()
	if key == "" {
		if scopedSession {
			return contract.CompactResult{Status: "unsupported", Reason: modelcap.CompactionScopedSessionUnsupported}, nil
		}
		return contract.CompactResult{Status: "unavailable", Reason: modelcap.CompactionBindingMissing}, nil
	}
	thread := req.Usage.SessionID
	if err := manager.resumeCompactionThread(ctx, live, key, thread); err != nil {
		if ctx.Err() != nil {
			return contract.CompactResult{}, ctx.Err()
		}
		reason := modelcap.CompactionRestoreFailed
		if errors.Is(err, errCompactionIdentityChanged) {
			reason = modelcap.CompactionIdentityChanged
		}
		return contract.CompactResult{Status: "unavailable", Reason: reason}, nil
	}
	raw, readErr := live.appClient.request(ctx, "thread/read", map[string]any{"threadId": thread, "includeTurns": true})
	if readErr != nil {
		return contract.CompactResult{Status: "unavailable", Reason: modelcap.CompactionHistoryUnreadable}, nil
	}
	var history struct {
		Thread struct {
			Turns []json.RawMessage `json:"turns"`
		} `json:"thread"`
	}
	if json.Unmarshal(raw, &history) != nil {
		return contract.CompactResult{Status: "unavailable", Reason: modelcap.CompactionHistoryInvalid}, nil
	}
	if len(history.Thread.Turns) == 0 {
		return contract.CompactResult{Status: "no_history"}, nil
	}
	live.mu.Lock()
	selected := turnModelFromProfile(live.spec.Profile)
	if m, ok := live.threadModels[thread]; ok {
		selected = m
	}
	live.mu.Unlock()
	waiter, err := live.registerAppServerTurnWaiterWithModel(thread, selected, false)
	if err != nil {
		return contract.CompactResult{Status: "busy"}, nil
	}
	defer live.removeAppServerTurnWaiter(thread, waiter)
	live.setAppServerTurnContext(thread, waiter, ctx)
	live.mu.Lock()
	if live.compactingThreads == nil {
		live.compactingThreads = map[string]bool{}
	}
	live.compactingThreads[thread] = true
	live.mu.Unlock()
	defer func() {
		live.mu.Lock()
		delete(live.compactingThreads, thread)
		usage := live.contextUsage[thread]
		usage.Compacting = false
		live.contextUsage[thread] = usage
		live.mu.Unlock()
		manager.publishContextUsage(runtimeID, live, thread, nil, "snapshot")
	}()
	if err := ctx.Err(); err != nil {
		return contract.CompactResult{}, err
	}
	if req.OnStarted != nil {
		if err := req.OnStarted(); err != nil {
			return contract.CompactResult{}, err
		}
	}
	manager.publishContextUsage(runtimeID, live, thread, nil, "item/started")
	if _, err = live.appClient.request(ctx, "thread/compact/start", map[string]any{"threadId": thread}); err != nil {
		return contract.CompactResult{Status: "uncertain"}, nil
	}
	if _, err = manager.waitAppServerTurn(ctx, live, waiter); err != nil {
		if errors.Is(err, errContextCompaction) {
			return contract.CompactResult{Status: "failed"}, nil
		}
		return contract.CompactResult{Status: "uncertain"}, nil
	}
	live.mu.Lock()
	usage := live.contextUsage[thread]
	live.mu.Unlock()
	usage.Compacting = false
	return contract.CompactResult{Status: "completed", Usage: modelcap.CloneUsage(&usage)}, nil
}

// Manual compaction may resume a saved thread, but must never create a
// replacement or rewrite the binding when history is unavailable.
func (m *appServerManager) resumeCompactionThread(ctx context.Context, live *liveSession, key, thread string) error {
	live.conversationResumeMu.Lock()
	defer live.conversationResumeMu.Unlock()
	live.conversationPersistMu.Lock()
	defer live.conversationPersistMu.Unlock()
	live.mu.Lock()
	current, loaded := live.conversationSessions[key], live.loadedConversations[key]
	live.mu.Unlock()
	if current != thread {
		return fmt.Errorf("%w: binding changed", errCompactionIdentityChanged)
	}
	if loaded {
		return nil
	}
	resumed, err := m.resumeThread(ctx, live, thread)
	if err != nil {
		return err
	}
	if resumed != thread {
		return fmt.Errorf("%w: resume returned a different conversation", errCompactionIdentityChanged)
	}
	live.mu.Lock()
	defer live.mu.Unlock()
	if live.loadedConversations == nil {
		live.loadedConversations = make(map[string]bool)
	}
	live.loadedConversations[key] = true
	return nil
}
