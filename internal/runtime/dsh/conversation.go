package dsh

import (
	"context"
	"csgclaw/internal/modelcap"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
)

const dshHungProcessStopTimeout = 5 * time.Second

var dshPromptCancellationTimeout = 5 * time.Second

type conversation struct {
	runtime   *Runtime
	runtimeID string
}

type sessionResult struct {
	SessionID     string            `json:"sessionId"`
	ConfigOptions []json.RawMessage `json:"configOptions"`
}

func (c *conversation) Run(ctx context.Context, request contract.TurnRequest, sink contract.EventSink) contract.TurnResult {
	proc, release, err := c.runtime.processForTurn(ctx, c.runtimeID)
	if err != nil {
		return failed(err)
	}
	defer release()
	prompt, cleanupInput, inputErr := preparePromptInput(ctx, request.ID, proc.workspace, request.Input, proc.imagePrompts)
	if inputErr != nil {
		return contract.TurnResult{Status: contract.TurnFailed, Error: inputErr}
	}
	defer cleanupInput()
	sessionID, turnErr := c.ensureSession(ctx, proc, request)
	if turnErr != nil {
		return contract.TurnResult{Status: contract.TurnFailed, Error: turnErr}
	}
	turn := &activeTurn{request: request, sink: sink, tools: make(map[string]contract.ToolActivity)}
	proc.mu.Lock()
	proc.active[sessionID] = turn
	metadata := proc.profile.ModelMetadata.Normalized()
	usage, ok := proc.contextUsage[sessionID]
	if !ok || usage.ModelID != proc.profile.ModelID {
		usage = modelcap.ContextUsage{SessionID: sessionID, ModelID: proc.profile.ModelID, ContextWindow: metadata.ContextWindow, ContextSource: metadata.ContextSource, AutoCompact: proc.profile.AutoCompact == nil || *proc.profile.AutoCompact, CompactThreshold: metadata.CompactThreshold(), Estimated: true}
	}
	turn.seq++
	initial := contract.TurnEvent{TurnID: request.ID, Sequence: turn.seq, Kind: contract.TurnEventActivityUpdate, Activity: &contract.ActivityUpdate{ID: sessionID, Kind: modelcap.ContextUsageKind, Payload: usage}}
	proc.mu.Unlock()
	if sink != nil {
		if err := sink.Emit(ctx, initial); err != nil {
			proc.mu.Lock()
			delete(proc.active, sessionID)
			proc.mu.Unlock()
			return failed(err)
		}
	}
	defer func() {
		proc.mu.Lock()
		delete(proc.active, sessionID)
		proc.mu.Unlock()
		c.runtime.cancelPendingPermissions(c.runtimeID, request.ConversationKey)
	}()

	dispatched := false
	var response struct {
		StopReason string `json:"stopReason"`
	}
	err = proc.client.callRetainingOnCancel(ctx, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    prompt,
	}, &response, func() { dispatched = true }, dshPromptCancellationTimeout, func() error {
		notifyErr := proc.client.notify("session/cancel", map[string]any{"sessionId": sessionID})
		c.runtime.cancelPendingPermissions(c.runtimeID, request.ConversationKey)
		return notifyErr
	})
	var cancellationCleanupErr error
	if errors.Is(err, errACPCancellationTimeout) {
		stopCtx, cancel := context.WithTimeout(context.Background(), dshHungProcessStopTimeout)
		_, cancellationCleanupErr = c.runtime.Stop(stopCtx, agentruntime.Handle{RuntimeID: c.runtimeID})
		cancel()
	}
	proc.mu.Lock()
	contextExceeded, compactionFailed := turn.contextExceeded, turn.compactionFailed
	interactionErr := turn.interactionError
	output := turn.output.String()
	presentedFiles := append([]presentedFile(nil), turn.presentedFiles...)
	proc.mu.Unlock()
	if interactionErr != nil {
		return contract.TurnResult{Status: contract.TurnFailed, Output: output, Dispatched: dispatched, Error: interactionErr}
	}
	if err != nil {
		if ctx.Err() != nil {
			message := ctx.Err().Error()
			if cancellationCleanupErr != nil {
				message = fmt.Sprintf("%s; stop unresponsive DSH process: %v", message, cancellationCleanupErr)
			}
			return contract.TurnResult{Status: contract.TurnCanceled, Output: output, Dispatched: dispatched, Error: &contract.TurnError{Code: contract.ErrorCanceled, Message: message}}
		}
		result := failed(err)
		if compactionFailed {
			result.Error = &contract.TurnError{Code: contract.ErrorCode("context_compaction_failed"), Message: "Conversation compaction failed. Your history is preserved. Retry or choose a model with a larger context window."}
		} else if contextExceeded {
			result.Error = &contract.TurnError{Code: contract.ErrorCode("context_length_exceeded"), Message: "The input exceeds the model context window. Split the input or check the model capacity."}
		}
		result.Output = output
		result.Dispatched = dispatched
		return result
	}
	switch strings.ToLower(strings.TrimSpace(response.StopReason)) {
	case "end_turn", "max_tokens":
		files, fileErr := authorizePresentedFiles(ctx, c.runtimeID, proc.workspace, presentedFiles)
		if fileErr != nil {
			return contract.TurnResult{Status: contract.TurnFailed, Output: output, Dispatched: dispatched, Error: fileErr}
		}
		return contract.TurnResult{Status: contract.TurnSucceeded, Output: output, Dispatched: dispatched, RuntimeFiles: files}
	case "cancelled":
		return contract.TurnResult{Status: contract.TurnCanceled, Output: output, Dispatched: dispatched, Error: &contract.TurnError{Code: contract.ErrorCanceled, Message: "DSH turn was canceled"}}
	default:
		result := failed(fmt.Errorf("DSH turn stopped with reason %q", response.StopReason))
		result.Output = output
		result.Dispatched = dispatched
		return result
	}
}

func (c *conversation) ensureSession(ctx context.Context, proc *process, request contract.TurnRequest) (string, *contract.TurnError) {
	key := string(request.ConversationKey)
	proc.mu.Lock()
	sessionID, exists := proc.meta.Sessions[key]
	ready := proc.ready[sessionID]
	proc.mu.Unlock()
	if !exists && request.Continuation == contract.ContinuationRequireExisting {
		return "", &contract.TurnError{Code: contract.ErrorConversationNotResumable, Message: "conversation has no DSH session mapping"}
	}
	if ready {
		return sessionID, nil
	}
	if exists {
		var resumed sessionResult
		if err := proc.client.call(ctx, "session/resume", map[string]any{"sessionId": sessionID, "cwd": proc.workspace, "mcpServers": proc.mcp}, &resumed, nil); err != nil {
			return "", &contract.TurnError{Code: contract.ErrorConversationNotResumable, Message: fmt.Sprintf("resume DSH session: %v", err)}
		}
		if err := configureSession(ctx, proc, sessionID, resumed.ConfigOptions); err != nil {
			return "", &contract.TurnError{Code: contract.ErrorRuntimeFailed, Message: err.Error()}
		}
		proc.mu.Lock()
		proc.ready[sessionID] = true
		proc.mu.Unlock()
		return sessionID, nil
	}
	var created sessionResult
	if err := proc.client.call(ctx, "session/new", map[string]any{"cwd": proc.workspace, "mcpServers": proc.mcp}, &created, nil); err != nil {
		return "", &contract.TurnError{Code: contract.ErrorRuntimeFailed, Message: fmt.Sprintf("create DSH session: %v", err)}
	}
	if strings.TrimSpace(created.SessionID) == "" {
		return "", &contract.TurnError{Code: contract.ErrorRuntimeFailed, Message: "DSH returned an empty session ID"}
	}
	if err := configureSession(ctx, proc, created.SessionID, created.ConfigOptions); err != nil {
		return "", &contract.TurnError{Code: contract.ErrorRuntimeFailed, Message: err.Error()}
	}
	if err := proc.updateMetadata(func(meta *runtimeMetadata) {
		meta.Sessions[key] = created.SessionID
		proc.ready[created.SessionID] = true
	}); err != nil {
		return "", &contract.TurnError{Code: contract.ErrorRuntimeFailed, Message: err.Error()}
	}
	return created.SessionID, nil
}

func configureSession(ctx context.Context, proc *process, sessionID string, options []json.RawMessage) error {
	modelValue, ok := selectOption(options, "model", proc.profile.ModelID)
	if !ok {
		return fmt.Errorf("DSH ACP does not advertise configured model %q", proc.profile.ModelID)
	}
	var updated sessionResult
	if err := proc.client.call(ctx, "session/set_config_option", map[string]any{"sessionId": sessionID, "configId": "model", "value": modelValue}, &updated, nil); err != nil {
		return fmt.Errorf("select DSH model: %w", err)
	}
	if effort := strings.TrimSpace(proc.profile.ReasoningEffort); !config.UsesModelReasoningDefault(effort) {
		if effort == config.ReasoningEffortNone {
			effort = "off"
		}
		candidateOptions := updated.ConfigOptions
		if len(candidateOptions) == 0 {
			candidateOptions = options
		}
		effortValue, found := selectOption(candidateOptions, "reasoning_effort", effort)
		if !found {
			return fmt.Errorf("DSH ACP does not advertise reasoning effort %q", effort)
		}
		if err := proc.client.call(ctx, "session/set_config_option", map[string]any{"sessionId": sessionID, "configId": "reasoning_effort", "value": effortValue}, nil, nil); err != nil {
			return fmt.Errorf("select DSH reasoning effort: %w", err)
		}
	}
	return nil
}

func selectOption(options []json.RawMessage, configID, desired string) (string, bool) {
	for _, raw := range options {
		var option struct {
			ID      string          `json:"id"`
			Options json.RawMessage `json:"options"`
		}
		if json.Unmarshal(raw, &option) != nil || option.ID != configID {
			continue
		}
		var values []any
		if json.Unmarshal(option.Options, &values) != nil {
			continue
		}
		return findSelectValue(values, desired)
	}
	return "", false
}

func findSelectValue(items []any, desired string) (string, bool) {
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if nested, ok := entry["options"].([]any); ok {
			if value, found := findSelectValue(nested, desired); found {
				return value, true
			}
		}
		value, _ := entry["value"].(string)
		name, _ := entry["name"].(string)
		if strings.EqualFold(strings.TrimSpace(name), desired) || strings.EqualFold(strings.TrimSpace(value), desired) || modelValueMatches(value, desired) {
			return value, true
		}
	}
	return "", false
}

func modelValueMatches(value, desired string) bool {
	var pair []string
	return json.Unmarshal([]byte(value), &pair) == nil && len(pair) == 2 && pair[1] == desired
}

func (c *conversation) Reset(ctx context.Context, key contract.ConversationKey) *contract.TurnError {
	proc, err := c.runtime.process(c.runtimeID)
	if err != nil {
		return &contract.TurnError{Code: contract.ErrorRuntimeFailed, Message: err.Error()}
	}
	proc.mu.Lock()
	sessionID, ok := proc.meta.Sessions[string(key)]
	proc.mu.Unlock()
	if !ok {
		return nil
	}
	if err := proc.updateMetadata(func(meta *runtimeMetadata) {
		if meta.Sessions[string(key)] == sessionID {
			delete(meta.Sessions, string(key))
			delete(proc.ready, sessionID)
		}
	}); err != nil {
		return &contract.TurnError{Code: contract.ErrorRuntimeFailed, Message: err.Error()}
	}
	if err := proc.client.call(ctx, "session/close", map[string]any{"sessionId": sessionID}, nil, nil); err != nil {
		return &contract.TurnError{Code: contract.ErrorRuntimeFailed, Message: fmt.Sprintf("close DSH session: %v", err)}
	}
	return nil
}

func (c *conversation) Resolve(ctx context.Context, request contract.InteractionRequest, resolution contract.InteractionResolution) *contract.TurnError {
	if request.Kind != contract.InteractionPermission {
		return &contract.TurnError{Code: contract.ErrorInteractionUnsupported, Message: "DSH interaction kind is unsupported"}
	}
	optionID := strings.TrimSpace(resolution.OptionID)
	if optionID == "" {
		return &contract.TurnError{Code: contract.ErrorInvalidRequest, Message: "permission option ID is required"}
	}
	c.runtime.mu.Lock()
	pending := c.runtime.pending[request.ID]
	conversationMatches := resolution.ConversationKey == "" || pending != nil && pending.conversation == resolution.ConversationKey
	if pending != nil && pending.runtimeID == c.runtimeID && conversationMatches && pending.allowedOptions[optionID] {
		delete(c.runtime.pending, request.ID)
	}
	c.runtime.mu.Unlock()
	if pending == nil || pending.runtimeID != c.runtimeID || !conversationMatches || !pending.allowedOptions[optionID] {
		return &contract.TurnError{Code: contract.ErrorInteractionNotFound, Message: "DSH permission request is no longer pending"}
	}
	if err := pending.client.respond(pending.requestID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": optionID}}, nil); err != nil {
		return &contract.TurnError{Code: contract.ErrorRuntimeFailed, Message: err.Error()}
	}
	return nil
}

func (r *Runtime) cancelPendingPermissions(runtimeID string, key contract.ConversationKey) {
	r.mu.Lock()
	var pending []*pendingPermission
	for id, item := range r.pending {
		if item.runtimeID == runtimeID && item.conversation == key {
			pending = append(pending, item)
			delete(r.pending, id)
		}
	}
	r.mu.Unlock()
	for _, item := range pending {
		_ = item.client.respond(item.requestID, map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}, nil)
	}
}

func failed(err error) contract.TurnResult {
	return contract.TurnResult{Status: contract.TurnFailed, Error: &contract.TurnError{Code: contract.ErrorRuntimeFailed, Message: err.Error()}}
}
