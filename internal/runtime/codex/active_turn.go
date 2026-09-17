package codex

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
)

// ActiveTurnContext validates a caller's already-bound native thread and turn.
// It never guesses the latest conversation or creates/restores a runtime.
func (r *Runtime) ActiveTurnContext(runtimeID, threadID, turnID string) (context.Context, error) {
	if strings.TrimSpace(threadID) == "" || strings.TrimSpace(turnID) == "" {
		return nil, fmt.Errorf("thread ID and turn ID are required")
	}
	manager, ok := r.currentSessionManager().(*appServerManager)
	if !ok {
		return nil, fmt.Errorf("runtime is not active")
	}
	manager.mu.RLock()
	live := manager.sessions[strings.TrimSpace(runtimeID)]
	manager.mu.RUnlock()
	if live == nil || !live.appServerPublishesFilesForThread(threadID) {
		return nil, fmt.Errorf("thread is not available for file delivery")
	}
	ctx, ok := live.appServerTurnContext(threadID, turnID)
	if !ok || ctx.Err() != nil {
		return nil, fmt.Errorf("thread and turn are not active")
	}
	return ctx, nil
}

// AgentTurnContext resolves the exact native thread and turn carried in Codex
// MCP metadata. The returned conversation is the persisted Engine binding.
func (r *Runtime) AgentTurnContext(agentID, threadID, turnID string) (context.Context, string, error) {
	_, live, err := r.activeAgentSession(agentID)
	if err != nil {
		return nil, "", err
	}
	ctx, err := r.ActiveTurnContext(live.spec.RuntimeID, threadID, turnID)
	if err != nil {
		return nil, "", err
	}
	live.mu.Lock()
	defer live.mu.Unlock()
	for conversation, thread := range live.conversationSessions {
		if thread == threadID {
			return ctx, conversation, nil
		}
	}
	return nil, "", fmt.Errorf("thread has no Engine conversation binding")
}

func (r *Runtime) activeAgentSession(agentID string) (*appServerManager, *liveSession, error) {
	manager, ok := r.currentSessionManager().(*appServerManager)
	if !ok {
		return nil, nil, fmt.Errorf("runtime is not active")
	}
	agentID = canonicalRuntimeAgentID(agentID)
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	for _, live := range manager.sessions {
		if live.spec.AgentID == agentID {
			return manager, live, nil
		}
	}
	return nil, nil, fmt.Errorf("Agent runtime is not active")
}

// CallPlatformFileTool uses the same file delivery/upload implementation as
// native dynamic tools after validating the Agent, thread and active turn.
// Its JSON result has the MCP tools/call result shape.
func (r *Runtime) CallPlatformFileTool(ctx context.Context, agentID, name, threadID, turnID string, args json.RawMessage) (json.RawMessage, error) {
	manager, live, err := r.activeAgentSession(agentID)
	if err != nil {
		return nil, err
	}
	turnCtx, _, err := r.AgentTurnContext(agentID, threadID, turnID)
	if err != nil {
		return nil, err
	}
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if turnCtx.Err() != nil {
		return nil, turnCtx.Err()
	}
	params, err := json.Marshal(appServerDynamicToolCallParams{
		ThreadID: threadID, TurnID: turnID, CallID: "mcp-" + rand.Text(), Tool: name, Arguments: args,
	})
	if err != nil {
		return nil, err
	}
	response, err := manager.handleAppServerDynamicToolCall(live.spec.RuntimeID, live, appServerServerRequest{Method: "item/tool/call", Params: params})
	if err != nil {
		return nil, err
	}
	result := response.(map[string]any)
	items := result["contentItems"].([]map[string]any)
	content := make([]map[string]any, 0, len(items))
	for _, item := range items {
		content = append(content, map[string]any{"type": "text", "text": item["text"]})
	}
	return json.Marshal(map[string]any{"content": content, "isError": result["success"] != true})
}
