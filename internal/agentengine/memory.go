package agentengine

import (
	"context"
	"log/slog"
	"strings"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/agentmemory"
	"csgclaw/internal/identity"
)

func (e *Engine) SetMemory(store *agentmemory.Store) {
	e.memory.Store(store)
}

func (c *conversations) prepareMemory(request *TurnRequest) agentmemory.Ticket {
	store := c.engine.memory.Load()
	source := request.MemorySource
	if store == nil || source == nil {
		return agentmemory.Ticket{}
	}
	request.RuntimeMemory = &contract.RuntimeMemory{Binding: store.Policy() + "-plain"}
	if source.UserID == "" {
		return agentmemory.Ticket{}
	}
	scope := agentmemory.Scope{AgentID: identity.CanonicalAgentID(c.agentID), UserID: source.UserID}
	ticket, binding, content, err := store.Snapshot(scope, source.Text)
	if err != nil {
		slog.Warn("agent memory could not be read", "agent_id", c.agentID)
		return agentmemory.Ticket{}
	}
	if binding != "" {
		request.RuntimeMemory = &contract.RuntimeMemory{Binding: store.Policy() + "-" + binding, Text: content}
	}
	return ticket
}

func (c *conversations) learnMemory(request TurnRequest, ticket agentmemory.Ticket, result TurnResult) {
	store := c.engine.memory.Load()
	if store == nil || request.MemorySource == nil || result.Status != TurnSucceeded || !result.Dispatched {
		return
	}
	source := request.MemorySource
	message := agentmemory.Message{Source: agentmemory.Source{RoomID: source.RoomID, MessageID: source.MessageID}, Text: source.Text}
	if err := store.Enqueue(ticket, source.RoomID+"\x00"+source.MessageID, message); err != nil {
		slog.Warn("agent memory could not enqueue a completed turn", "agent_id", c.agentID)
	}
}

// WithMemoryExecution protects a memory mutation from racing with Agent
// deletion/archival while allowing an already-running chat to finish.
func (e *Engine) WithMemoryExecution(ctx context.Context, agentID string, fn func() error) error {
	if e.lifecycle == nil {
		return agentmemory.ErrUnavailable
	}
	release, err := e.lifecycle.Execution(ctx, identity.CanonicalAgentID(agentID))
	if err != nil {
		return agentmemory.ErrUnavailable
	}
	defer release()
	current, err := e.Agents().Get(ctx, agentID, AgentGetOptions{})
	if err != nil || current.ArchivedAt != nil {
		return agentmemory.ErrUnavailable
	}
	return fn()
}

func (c *conversations) forgetMemory(ctx context.Context, source *contract.MemorySource) (TurnResult, bool) {
	store := c.engine.memory.Load()
	if store == nil || source == nil || source.UserID == "" {
		return TurnResult{}, false
	}
	text := strings.TrimSpace(source.Text)
	if text != "忘掉这条" && !strings.EqualFold(text, "forget this") {
		return TurnResult{}, false
	}
	zh := strings.HasPrefix(strings.ToLower(source.Locale), "zh") || text == "忘掉这条"
	result := TurnResult{Status: TurnSucceeded}
	if source.ReplyTo == "" {
		result.Output = "Choose Forget this on the original message to select what to forget."
		if zh {
			result.Output = "请在原消息的操作区选择「忘掉这条」。"
		}
		return result, true
	}
	err := c.engine.WithMemoryExecution(ctx, c.agentID, func() error {
		scope := agentmemory.Scope{AgentID: identity.CanonicalAgentID(c.agentID), UserID: source.UserID}
		_, err := store.Forget(scope, agentmemory.Source{RoomID: source.RoomID, MessageID: source.ReplyTo})
		return err
	})
	if err != nil {
		return failedResult(ErrorRuntimeFailed, "Memory could not be updated"), true
	}
	result.Output = "Forgotten."
	if zh {
		result.Output = "已忘掉。"
	}
	return result, true
}
