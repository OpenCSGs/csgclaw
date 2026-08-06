package agentengine

import (
	"context"
	"fmt"
	"strings"
)

type conversationRuntimeResolver interface {
	conversationRuntime(agentID string) (conversationRuntimeAdapter, *TurnError)
}

type conversationRuntimeAdapter interface {
	Run(ctx context.Context, request TurnRequest, sink EventSink) TurnResult
}

// Engine is the Phase 1 in-process Conversation implementation.
// Agent CRUD and lifecycle remain owned by the existing Agent Service.
type Engine struct {
	runtimes conversationRuntimeResolver
}

func (e *Engine) Conversations(agentID string) ConversationInterface {
	return &conversations{engine: e, agentID: strings.TrimSpace(agentID)}
}

type conversations struct {
	engine  *Engine
	agentID string
}

func (c *conversations) Run(ctx context.Context, request TurnRequest, sink EventSink) TurnResult {
	if ctx == nil {
		ctx = context.Background()
	}
	request.ConversationKey = ConversationKey(strings.TrimSpace(string(request.ConversationKey)))
	request.ID = TurnID(strings.TrimSpace(string(request.ID)))
	if c == nil || c.engine == nil || c.engine.runtimes == nil || c.agentID == "" || request.ID == "" || request.ConversationKey == "" || len(request.Input) == 0 {
		return failedResult(ErrorInvalidRequest, "agent ID, turn ID, conversation key, and input are required")
	}
	if err := validateInput(request.Input); err != nil {
		return TurnResult{Status: TurnFailed, Error: err}
	}

	runtimeAdapter, err := c.engine.runtimes.conversationRuntime(c.agentID)
	if err != nil {
		return TurnResult{Status: TurnFailed, Error: err}
	}
	return runtimeAdapter.Run(ctx, request, sink)
}

func validateInput(input []InputPart) *TurnError {
	for _, part := range input {
		switch part.Kind {
		case InputPartText:
			if part.File != nil {
				return &TurnError{Code: ErrorInvalidRequest, Message: "text input must not include a file"}
			}
			if strings.TrimSpace(part.Text) == "" {
				return &TurnError{Code: ErrorInvalidRequest, Message: "text input must not be empty"}
			}
		case InputPartFile:
			if part.File == nil || strings.TrimSpace(part.Text) != "" {
				return &TurnError{Code: ErrorInvalidRequest, Message: "file input must include only a file"}
			}
		default:
			return &TurnError{Code: ErrorInvalidRequest, Message: fmt.Sprintf("unsupported input kind %q", part.Kind)}
		}
	}
	return nil
}

func failedResult(code ErrorCode, message string) TurnResult {
	return TurnResult{Status: TurnFailed, Error: &TurnError{Code: code, Message: message}}
}
