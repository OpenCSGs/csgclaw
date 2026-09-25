// Package interaction maps trusted, normalized Feishu interactions to Agent
// Engine conversation controls. Feishu SDK and card wire types stay outside
// this package.
package interaction

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"csgclaw/internal/agentengine"
)

// Operation is an allowlisted card control operation.
type Operation string

const (
	OperationResolve Operation = "resolve"
	OperationCancel  Operation = "cancel"
	OperationReset   Operation = "reset"
)

// CardAction is a Feishu card action after transport normalization.
type CardAction struct {
	Operation Operation
}

// Input combines a normalized action with trusted routing identity resolved by
// the binding and ingress layers. Callers must not populate these fields from
// the card action value.
type Input struct {
	InteractionID   string
	ResponderID     string
	OptionID        string
	FormValue       map[string]any
	AgentID         string
	ConversationKey string
	TurnID          string
	Action          CardAction
}

var ErrInvalidInput = errors.New("invalid feishu card action input")

// Handler dispatches card controls through the single Agent Engine path.
type Handler struct {
	engine agentengine.Interface
}

func NewHandler(engine agentengine.Interface) (*Handler, error) {
	if engine == nil {
		return nil, fmt.Errorf("%w: agent engine is required", ErrInvalidInput)
	}
	return &Handler{engine: engine}, nil
}

// Handle validates the trusted envelope and invokes exactly one Engine
// conversation control operation.
func (h *Handler) Handle(ctx context.Context, input Input) error {
	if h == nil || h.engine == nil {
		return fmt.Errorf("%w: agent engine is required", ErrInvalidInput)
	}
	agentID := strings.TrimSpace(input.AgentID)
	if agentID == "" {
		return fmt.Errorf("%w: agent_id is required", ErrInvalidInput)
	}
	conversationKey := strings.TrimSpace(input.ConversationKey)
	if conversationKey == "" {
		return fmt.Errorf("%w: conversation_key is required", ErrInvalidInput)
	}

	conversation := h.engine.Conversations(agentID)
	if conversation == nil {
		return fmt.Errorf("%w: agent engine returned no conversation interface", ErrInvalidInput)
	}

	switch Operation(strings.TrimSpace(string(input.Action.Operation))) {
	case OperationReset:
		return conversation.Reset(ctx, agentengine.ConversationKey(conversationKey))
	default:
		return fmt.Errorf("%w: unsupported operation %q", ErrInvalidInput, input.Action.Operation)
	}
}

// CancelRequest identifies the delivered control and the task resolved from it.
type CancelRequest struct {
	BindingID, AgentID, ConversationKey, TurnID string
	MessageID, ChatID, ThreadID, RequesterID    string
}
