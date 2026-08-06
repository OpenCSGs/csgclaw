package agentengine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"csgclaw/internal/activity"
	"csgclaw/internal/agent"
	"csgclaw/internal/runtime"
)

var errInteractiveTurnUnsupported = errors.New("interactive approval and user-input requests are not supported for anonymous streamed sessions")

type agentServiceAdapter struct {
	service *agent.Service
}

// New wires the transitional Agent Service-backed Runtime resolver into the
// runtime-neutral Engine core.
func New(service *agent.Service) *Engine {
	return &Engine{runtimes: agentServiceAdapter{service: service}}
}

func (a agentServiceAdapter) conversationRuntime(agentID string) (conversationRuntimeAdapter, *TurnError) {
	if a.service == nil {
		return nil, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is required"}
	}
	selected, ok := a.service.Agent(agentID)
	if !ok || !strings.EqualFold(strings.TrimSpace(selected.Status), string(runtime.StateRunning)) || strings.TrimSpace(selected.RuntimeID) == "" {
		return nil, &TurnError{Code: ErrorAgentUnavailable, Message: fmt.Sprintf("agent %q is unavailable", agentID)}
	}
	if !strings.EqualFold(strings.TrimSpace(selected.RuntimeKind), agent.RuntimeKindCodex) {
		return nil, &TurnError{Code: ErrorRuntimeAdapterUnavailable, Message: fmt.Sprintf("runtime adapter %q is unavailable", selected.RuntimeKind)}
	}

	runtimeImpl, err := a.service.Runtime(selected.RuntimeKind)
	if err != nil {
		return nil, &TurnError{Code: ErrorRuntimeAdapterUnavailable, Message: err.Error()}
	}
	codex, ok := runtimeImpl.(codexConversationRuntime)
	if !ok {
		return nil, &TurnError{Code: ErrorRuntimeAdapterUnavailable, Message: "Codex runtime does not support direct conversations"}
	}
	return codexRuntimeAdapter{runtimeID: selected.RuntimeID, runtime: codex}, nil
}

type codexConversationRuntime interface {
	EnsureSession(ctx context.Context, runtimeID, conversationKey string) (string, error)
	Prompt(ctx context.Context, runtimeID, sessionID, prompt string) error
	SubscribeSession(runtimeID, sessionID string) (<-chan activity.RuntimeEvent, func())
}

type codexRuntimeAdapter struct {
	runtimeID string
	runtime   codexConversationRuntime
}

func (a codexRuntimeAdapter) Run(ctx context.Context, request TurnRequest, sink EventSink) TurnResult {
	prompt, inputErr := a.prepareInput(request.Input)
	if inputErr != nil {
		return TurnResult{Status: TurnFailed, Error: inputErr}
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sessionID, err := a.runtime.EnsureSession(runCtx, a.runtimeID, string(request.ConversationKey))
	if err != nil {
		return resultFromContext(runCtx, fmt.Errorf("ensure Codex conversation: %w", err))
	}
	events, unsubscribe := a.runtime.SubscribeSession(a.runtimeID, sessionID)
	defer unsubscribe()

	promptDone := make(chan error, 1)
	go func() {
		promptDone <- a.runtime.Prompt(runCtx, a.runtimeID, sessionID, prompt)
	}()

	var output strings.Builder
	promptReturned := false
	stopPrompt := func(result TurnResult) TurnResult {
		cancel()
		if !promptReturned {
			<-promptDone
			promptReturned = true
		}
		return result
	}
	runtimeDone := false
	for {
		if runtimeDone && promptReturned {
			return TurnResult{Status: TurnSucceeded, Output: output.String()}
		}
		select {
		case <-runCtx.Done():
			return stopPrompt(resultFromContext(runCtx, runCtx.Err()))
		case err := <-promptDone:
			promptReturned = true
			if err != nil {
				return resultFromContext(runCtx, err)
			}
		case event, ok := <-events:
			if !ok {
				runtimeDone = true
				continue
			}
			if strings.TrimSpace(event.RuntimeID) != strings.TrimSpace(a.runtimeID) || strings.TrimSpace(event.SessionID) != strings.TrimSpace(sessionID) {
				continue
			}
			switch event.Kind {
			case activity.RuntimeEventPromptFailed:
				message := strings.TrimSpace(event.Error)
				if message == "" {
					message = "agent turn ended without a final response"
				}
				return stopPrompt(failedResult(ErrorRuntimeFailed, message))
			case activity.RuntimeEventActionRequest, activity.RuntimeEventUserInputRequest:
				return stopPrompt(failedResult(ErrorInteractionUnsupported, errInteractiveTurnUnsupported.Error()))
			case activity.RuntimeEventStructuredOutput:
				artifact, ok := event.Payload.(activity.StructuredOutputArtifact)
				if ok && artifact.RequestUserInput != nil {
					return stopPrompt(failedResult(ErrorInteractionUnsupported, errInteractiveTurnUnsupported.Error()))
				}
			case activity.RuntimeEventTextDelta:
				phase := runtimeEventPhase(event)
				if phase != "" && phase != "final_answer" {
					continue
				}
				if err := emitTurnEvent(runCtx, sink, TurnEvent{Kind: TurnEventTextDelta, Text: event.Text}); err != nil {
					return stopPrompt(resultFromContext(runCtx, err))
				}
				_, _ = output.WriteString(event.Text)
			case activity.RuntimeEventToolCallStart:
				if err := emitTurnEvent(runCtx, sink, TurnEvent{Kind: TurnEventToolCallStart, Tool: toolActivity(event)}); err != nil {
					return stopPrompt(resultFromContext(runCtx, err))
				}
			case activity.RuntimeEventToolCallUpdate:
				if err := emitTurnEvent(runCtx, sink, TurnEvent{Kind: TurnEventToolCallUpdate, Tool: toolActivity(event)}); err != nil {
					return stopPrompt(resultFromContext(runCtx, err))
				}
			case activity.RuntimeEventPromptCompleted:
				runtimeDone = true
			}
		}
	}
}

func (a codexRuntimeAdapter) prepareInput(input []InputPart) (string, *TurnError) {
	text := make([]string, 0, len(input))
	for _, part := range input {
		switch part.Kind {
		case InputPartText:
			text = append(text, strings.TrimSpace(part.Text))
		case InputPartFile:
			return "", &TurnError{Code: ErrorFileUnavailable, Message: "Codex file input is not supported yet"}
		}
	}
	return strings.Join(text, "\n\n"), nil
}

func emitTurnEvent(ctx context.Context, sink EventSink, event TurnEvent) error {
	if sink == nil {
		return nil
	}
	return sink.Emit(ctx, event)
}

func toolActivity(event activity.RuntimeEvent) *ToolActivity {
	return &ToolActivity{
		ID:            event.ToolCallID,
		Kind:          event.ToolKind,
		Title:         event.ToolTitle,
		Status:        event.ToolStatus,
		InputSummary:  event.ToolInputSummary,
		OutputSummary: event.ToolOutputSummary,
		Payload:       event.Payload,
	}
}

func runtimeEventPhase(event activity.RuntimeEvent) string {
	payload, ok := event.Payload.(map[string]any)
	if !ok {
		return ""
	}
	phase, ok := payload["phase"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(strings.ToLower(phase))
}

func resultFromContext(ctx context.Context, err error) TurnResult {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return TurnResult{Status: TurnCanceled, Error: &TurnError{Code: ErrorRuntimeFailed, Message: err.Error()}}
	}
	return failedResult(ErrorRuntimeFailed, err.Error())
}
