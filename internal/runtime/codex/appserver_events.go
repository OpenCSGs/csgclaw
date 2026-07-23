package codex

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	activitypkg "csgclaw/internal/activity"
	agentruntime "csgclaw/internal/runtime"
)

const (
	appServerProtocolUnknown = ""
	appServerProtocolRaw     = "raw"
	appServerProtocolLegacy  = "legacy"
)

func (m *appServerManager) handleAppServerNotification(runtimeID string, live *liveSession, note appServerNotification) {
	var params map[string]any
	if len(note.Params) > 0 && string(note.Params) != "null" {
		if err := json.Unmarshal(note.Params, &params); err != nil {
			if live.appClient != nil {
				live.appClient.logDebug("ignore malformed codex app-server notification params", "method", note.Method, "error", err)
			}
			return
		}
	}
	if params == nil {
		params = map[string]any{}
	}
	if note.Method == "serverRequest/resolved" {
		if m.deps.UserInput != nil {
			m.deps.UserInput.CancelServerRequest(
				runtimeID,
				appServerString(params, "threadId"),
				appServerRequestIDValue(params["requestId"]),
			)
		}
		return
	}

	if note.Method == "codex/response_item" {
		m.handleLegacyResponseItemEvent(runtimeID, live, params)
		return
	}

	protocol := live.appServerProtocol(note.Method)
	if protocol == appServerProtocolLegacy {
		m.handleLegacyAppServerEvent(runtimeID, live, params)
		return
	}
	if protocol == appServerProtocolRaw {
		m.handleRawAppServerNotification(runtimeID, live, note.Method, params)
	}
}

func appServerRequestIDValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
	}
	return ""
}

func (s *liveSession) appServerProtocol(method string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case method == "codex/event":
		if s.appProtocol == appServerProtocolUnknown {
			s.appProtocol = appServerProtocolLegacy
		}
	case strings.HasPrefix(method, "turn/") || strings.HasPrefix(method, "thread/") || strings.HasPrefix(method, "item/") || method == "error":
		if s.appProtocol == appServerProtocolUnknown {
			s.appProtocol = appServerProtocolRaw
		}
	}
	return s.appProtocol
}

func (m *appServerManager) handleRawAppServerNotification(runtimeID string, live *liveSession, method string, params map[string]any) {
	threadID := appServerNotificationThreadID(params)
	if threadID == "" {
		threadID = m.appServerPrimaryThreadID(live)
	}
	if threadID == "" || !live.appServerTracksThread(threadID) {
		return
	}
	live.trackAppServerTurn(threadID, appServerNotificationTurnID(params))

	switch method {
	case "turn/started":
		live.notifyAppServerTurn(threadID, appServerTurnResult{
			turnID:   appServerNotificationTurnID(params),
			activity: "status:running",
			started:  true,
		})
	case "turn/completed":
		m.handleRawTurnCompleted(runtimeID, live, threadID, params)
	case "thread/status/changed":
		if strings.EqualFold(appServerNestedString(params, "status", "type"), "idle") {
			live.notifyAppServerTurn(threadID, appServerTurnResult{success: true, stopReason: StopReasonEndTurn, activity: "status:idle"})
		}
	case "error":
		m.handleRawErrorNotification(runtimeID, live, threadID, params)
	default:
		if strings.HasPrefix(method, "item/") {
			m.handleRawItemNotification(runtimeID, live, threadID, method, params)
		}
	}
}

func (m *appServerManager) handleRawTurnCompleted(runtimeID string, live *liveSession, threadID string, params map[string]any) {
	status := strings.ToLower(appServerNestedString(params, "turn", "status"))
	if status == "" {
		status = strings.ToLower(appServerString(params, "status"))
	}
	turnID := appServerNotificationTurnID(params)
	defer live.clearAppServerCommandOutputs(threadID, turnID)
	switch status {
	case "", "completed", "success", "succeeded":
		if !live.notifyAppServerTurn(threadID, appServerTurnResult{success: true, stopReason: StopReasonEndTurn, turnID: turnID, activity: "turn:completed"}) {
			m.publishAppServerEvent(promptCompletedEvent(runtimeID, threadID, PromptResponse{StopReason: StopReasonEndTurn}))
		}
	case "cancelled", "canceled", "aborted", "interrupted":
		if waiter := live.appServerTurnWaiter(threadID); waiter != nil && waiter.hasStructuredOutputBoundary() {
			if !live.notifyAppServerTurn(threadID, appServerTurnResult{success: true, stopReason: StopReasonEndTurn, turnID: turnID, activity: "turn:structured-output"}) {
				m.publishAppServerEvent(promptCompletedEvent(runtimeID, threadID, PromptResponse{StopReason: StopReasonEndTurn}))
			}
			return
		}
		if m.deps.UserInput != nil {
			m.deps.UserInput.CancelSession(runtimeID, threadID)
		}
		err := fmt.Errorf("codex turn %s", status)
		if !live.notifyAppServerTurn(threadID, appServerTurnResult{err: err, turnID: turnID, activity: "turn:" + status}) {
			m.publishAppServerEvent(SessionEvent{
				RuntimeID: runtimeID,
				SessionID: threadID,
				Kind:      SessionEventPromptFailed,
				Error:     err.Error(),
				Payload:   params,
			})
		}
	default:
		errMsg := appServerNestedString(params, "turn", "error", "message")
		if errMsg == "" {
			errMsg = "codex turn failed"
		}
		err := fmt.Errorf("%s", errMsg)
		if !live.notifyAppServerTurn(threadID, appServerTurnResult{err: err, turnID: turnID, activity: "turn:" + status}) {
			m.publishAppServerEvent(SessionEvent{
				RuntimeID: runtimeID,
				SessionID: threadID,
				Kind:      SessionEventPromptFailed,
				Error:     err.Error(),
				Payload:   params,
			})
		}
	}
}

func (m *appServerManager) handleRawErrorNotification(runtimeID string, live *liveSession, threadID string, params map[string]any) {
	willRetry, _ := params["willRetry"].(bool)
	activity := "error:retry"
	if !willRetry {
		activity = "error:terminal"
	}
	errMsg := appServerNestedString(params, "error", "message")
	if errMsg == "" {
		errMsg = appServerString(params, "message")
	}
	result := appServerTurnResult{activity: activity}
	if !willRetry {
		if errMsg == "" {
			errMsg = "codex app-server error"
		}
		result.err = fmt.Errorf("%s", errMsg)
	}
	if !live.notifyAppServerTurn(threadID, result) && result.err != nil {
		m.publishAppServerEvent(SessionEvent{
			RuntimeID: runtimeID,
			SessionID: threadID,
			Kind:      SessionEventPromptFailed,
			Error:     errMsg,
			Payload:   params,
		})
	}
}

func (m *appServerManager) handleRawItemNotification(runtimeID string, live *liveSession, threadID string, method string, params map[string]any) {
	if method == "item/commandExecution/outputDelta" {
		itemID := appServerString(params, "itemId")
		live.appendAppServerCommandOutput(threadID, appServerNotificationTurnID(params), itemID, appServerString(params, "delta"))
		live.notifyAppServerTurn(threadID, appServerTurnResult{
			activity: "commandExecution:outputDelta:" + itemID,
			progress: true,
		})
		return
	}
	item, _ := params["item"].(map[string]any)
	if item == nil {
		return
	}
	itemType := appServerString(item, "type")
	itemID := appServerString(item, "id")
	activity := strings.Trim(strings.TrimPrefix(method, "item/")+":"+itemType+":"+itemID, ":")
	live.notifyAppServerTurn(threadID, appServerTurnResult{
		activity:          activity,
		progress:          appServerItemIsProgress(itemType),
		assistantActivity: appServerItemSignalsAssistantActivity(itemType),
	})

	switch {
	case method == "item/started" && itemType == "commandExecution":
		command := appServerString(item, "command")
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:         runtimeID,
			SessionID:         threadID,
			Kind:              SessionEventToolCallStart,
			ToolCallID:        itemID,
			ToolKind:          "exec_command",
			ToolTitle:         "Run shell command",
			ToolStatus:        "started",
			ToolInputSummary:  summarizeToolValue(map[string]any{"command": command}),
			ToolOutputSummary: "",
			Payload:           item,
		})
	case method == "item/completed" && itemType == "commandExecution":
		status := appServerToolStatus(item, "completed")
		output := appServerString(item, "aggregatedOutput")
		if output == "" {
			output = appServerString(item, "output")
		}
		accumulated := live.takeAppServerCommandOutput(threadID, itemID)
		if output == "" && accumulated != nil {
			var artifact activitypkg.StructuredOutputArtifact
			var decodeErrors []error
			output, artifact, decodeErrors = accumulated.finish(structuredOutputToolStatusSuccessful(status))
			m.publishStructuredCommandArtifact(runtimeID, threadID, itemID, artifact, decodeErrors, output, live)
		} else {
			output = m.decodeAndPublishStructuredCommandOutput(runtimeID, threadID, itemID, status, output, item, live)
		}
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:         runtimeID,
			SessionID:         threadID,
			Kind:              SessionEventToolCallUpdate,
			ToolCallID:        itemID,
			ToolKind:          "exec_command",
			ToolTitle:         "Run shell command",
			ToolStatus:        status,
			ToolOutputSummary: summarizeToolValue(map[string]any{"output": output}),
			Payload:           item,
		})
	case method == "item/started" && itemType == "fileChange":
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:        runtimeID,
			SessionID:        threadID,
			Kind:             SessionEventToolCallStart,
			ToolCallID:       itemID,
			ToolKind:         "patch_apply",
			ToolTitle:        "Apply patch",
			ToolStatus:       "started",
			ToolInputSummary: summarizeToolValue(item),
			Payload:          item,
		})
	case method == "item/completed" && itemType == "fileChange":
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:         runtimeID,
			SessionID:         threadID,
			Kind:              SessionEventToolCallUpdate,
			ToolCallID:        itemID,
			ToolKind:          "patch_apply",
			ToolTitle:         "Apply patch",
			ToolStatus:        appServerToolStatus(item, "completed"),
			ToolOutputSummary: summarizeToolValue(item),
			Payload:           item,
		})
	case method == "item/started" && itemType == "mcpToolCall":
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:         runtimeID,
			SessionID:         threadID,
			Kind:              SessionEventToolCallStart,
			ToolCallID:        itemID,
			ToolKind:          "mcp_tool_call",
			ToolTitle:         appServerToolTitle(item, "Run MCP tool"),
			ToolStatus:        appServerToolStatus(item, "started"),
			ToolInputSummary:  summarizeToolValue(map[string]any{"server": item["server"], "tool": item["tool"], "arguments": item["arguments"]}),
			ToolOutputSummary: "",
			Payload:           item,
		})
	case method == "item/completed" && itemType == "mcpToolCall":
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:         runtimeID,
			SessionID:         threadID,
			Kind:              SessionEventToolCallUpdate,
			ToolCallID:        itemID,
			ToolKind:          "mcp_tool_call",
			ToolTitle:         appServerToolTitle(item, "Run MCP tool"),
			ToolStatus:        appServerToolStatus(item, "completed"),
			ToolOutputSummary: summarizeToolValue(map[string]any{"result": item["result"], "error": item["error"]}),
			Payload:           item,
		})
	case method == "item/started" && itemType == "dynamicToolCall":
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:         runtimeID,
			SessionID:         threadID,
			Kind:              SessionEventToolCallStart,
			ToolCallID:        itemID,
			ToolKind:          "dynamic_tool_call",
			ToolTitle:         appServerToolTitle(item, "Run dynamic tool"),
			ToolStatus:        appServerToolStatus(item, "started"),
			ToolInputSummary:  summarizeToolValue(map[string]any{"tool": item["tool"], "arguments": item["arguments"]}),
			ToolOutputSummary: "",
			Payload:           item,
		})
	case method == "item/completed" && itemType == "dynamicToolCall":
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:         runtimeID,
			SessionID:         threadID,
			Kind:              SessionEventToolCallUpdate,
			ToolCallID:        itemID,
			ToolKind:          "dynamic_tool_call",
			ToolTitle:         appServerToolTitle(item, "Run dynamic tool"),
			ToolStatus:        appServerToolStatus(item, "completed"),
			ToolOutputSummary: summarizeToolValue(map[string]any{"content_items": item["contentItems"], "success": item["success"]}),
			Payload:           item,
		})
	case method == "item/started" && itemType == "webSearch":
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:         runtimeID,
			SessionID:         threadID,
			Kind:              SessionEventToolCallStart,
			ToolCallID:        itemID,
			ToolKind:          "web_search",
			ToolTitle:         "Web search",
			ToolStatus:        appServerToolStatus(item, "started"),
			ToolInputSummary:  summarizeToolValue(map[string]any{"query": item["query"], "action": item["action"]}),
			ToolOutputSummary: "",
			Payload:           item,
		})
	case method == "item/completed" && itemType == "webSearch":
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:         runtimeID,
			SessionID:         threadID,
			Kind:              SessionEventToolCallUpdate,
			ToolCallID:        itemID,
			ToolKind:          "web_search",
			ToolTitle:         "Web search",
			ToolStatus:        appServerToolStatus(item, "completed"),
			ToolOutputSummary: summarizeToolValue(item),
			Payload:           item,
		})
	case method == "item/completed" && itemType == "agentMessage":
		text := appServerString(item, "text")
		if text != "" {
			m.publishAppServerEvent(SessionEvent{
				RuntimeID: runtimeID,
				SessionID: threadID,
				Kind:      SessionEventTextDelta,
				MessageID: itemID,
				Text:      text,
				Payload:   item,
			})
		}
	}
}

func (m *appServerManager) handleLegacyAppServerEvent(runtimeID string, live *liveSession, params map[string]any) {
	threadID := m.appServerLegacyThreadID(live, params)
	if threadID == "" {
		return
	}
	msgType := appServerString(params, "type")
	switch msgType {
	case "task_started":
		live.notifyAppServerTurn(threadID, appServerTurnResult{activity: "status:running", started: true})
	case "agent_message":
		text := appServerString(params, "message")
		if text != "" {
			if live.hasReplayedAgentMessage(params) {
				return
			}
			m.publishAppServerEvent(SessionEvent{
				RuntimeID: runtimeID,
				SessionID: threadID,
				Kind:      SessionEventTextDelta,
				Text:      text,
				Payload:   params,
			})
			live.markReplayedAgentMessage(params)
			live.notifyAppServerTurn(threadID, legacyMessageTurnResult(params, "legacy:agent_message"))
		}
	case "exec_command_begin":
		callID := appServerString(params, "call_id")
		command := appServerString(params, "command")
		live.markReplayedExecCommand(callID)
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:        runtimeID,
			SessionID:        threadID,
			Kind:             SessionEventToolCallStart,
			ToolCallID:       callID,
			ToolKind:         "exec_command",
			ToolTitle:        "Run shell command",
			ToolStatus:       "started",
			ToolInputSummary: summarizeToolValue(map[string]any{"command": command}),
			Payload:          params,
		})
		live.notifyAppServerTurn(threadID, appServerTurnResult{activity: "legacy:exec_command_begin", progress: true})
	case "exec_command_end":
		callID := appServerString(params, "call_id")
		output := appServerString(params, "output")
		status := legacyFunctionCallOutputStatus(output)
		output = m.decodeAndPublishStructuredCommandOutput(runtimeID, threadID, callID, status, output, params, live)
		live.markReplayedExecCommand(callID + ":output")
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:         runtimeID,
			SessionID:         threadID,
			Kind:              SessionEventToolCallUpdate,
			ToolCallID:        callID,
			ToolKind:          "exec_command",
			ToolTitle:         "Run shell command",
			ToolStatus:        status,
			ToolOutputSummary: summarizeToolValue(map[string]any{"output": output}),
			Payload:           params,
		})
		live.notifyAppServerTurn(threadID, appServerTurnResult{activity: "legacy:exec_command_end", progress: true})
	case "patch_apply_begin":
		callID := appServerString(params, "call_id")
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:  runtimeID,
			SessionID:  threadID,
			Kind:       SessionEventToolCallStart,
			ToolCallID: callID,
			ToolKind:   "patch_apply",
			ToolTitle:  "Apply patch",
			ToolStatus: "started",
			Payload:    params,
		})
		live.notifyAppServerTurn(threadID, appServerTurnResult{activity: "legacy:patch_apply_begin", progress: true})
	case "patch_apply_end":
		callID := appServerString(params, "call_id")
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:  runtimeID,
			SessionID:  threadID,
			Kind:       SessionEventToolCallUpdate,
			ToolCallID: callID,
			ToolKind:   "patch_apply",
			ToolTitle:  "Apply patch",
			ToolStatus: "completed",
			Payload:    params,
		})
		live.notifyAppServerTurn(threadID, appServerTurnResult{activity: "legacy:patch_apply_end", progress: true})
	case "task_complete":
		if !live.notifyAppServerTurn(threadID, appServerTurnResult{success: true, stopReason: StopReasonEndTurn, activity: "legacy:task_complete"}) {
			m.publishAppServerEvent(promptCompletedEvent(runtimeID, threadID, PromptResponse{StopReason: StopReasonEndTurn}))
		}
	case "turn_aborted":
		if waiter := live.appServerTurnWaiter(threadID); waiter != nil && waiter.hasStructuredOutputBoundary() {
			if !live.notifyAppServerTurn(threadID, appServerTurnResult{success: true, stopReason: StopReasonEndTurn, activity: "legacy:turn_structured-output"}) {
				m.publishAppServerEvent(promptCompletedEvent(runtimeID, threadID, PromptResponse{StopReason: StopReasonEndTurn}))
			}
			return
		}
		err := fmt.Errorf("codex turn aborted")
		if !live.notifyAppServerTurn(threadID, appServerTurnResult{err: err, activity: "legacy:turn_aborted"}) {
			m.publishAppServerEvent(SessionEvent{
				RuntimeID: runtimeID,
				SessionID: threadID,
				Kind:      SessionEventPromptFailed,
				Error:     err.Error(),
				Payload:   params,
			})
		}
	}
}

func (m *appServerManager) handleLegacyResponseItemEvent(runtimeID string, live *liveSession, params map[string]any) {
	threadID := m.appServerLegacyThreadID(live, params)
	if threadID == "" {
		return
	}
	switch appServerString(params, "type") {
	case "message":
		if appServerString(params, "role") != "assistant" {
			return
		}
		text := appServerResponseItemText(params)
		if text == "" || live.hasReplayedAgentMessage(params) {
			return
		}
		m.publishAppServerEvent(SessionEvent{
			RuntimeID: runtimeID,
			SessionID: threadID,
			Kind:      SessionEventTextDelta,
			MessageID: appServerString(params, "id"),
			Text:      text,
			Payload:   params,
		})
		live.markReplayedAgentMessage(params)
		live.notifyAppServerTurn(threadID, legacyMessageTurnResult(params, "legacy:response_item:message"))
	case "function_call":
		if appServerString(params, "name") != "exec_command" {
			return
		}
		callID := appServerString(params, "call_id")
		if callID == "" || live.hasReplayedExecCommand(callID) {
			return
		}
		args := decodeLegacyFunctionCallArguments(appServerString(params, "arguments"))
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:        runtimeID,
			SessionID:        threadID,
			Kind:             SessionEventToolCallStart,
			ToolCallID:       callID,
			ToolKind:         "exec_command",
			ToolTitle:        "Run shell command",
			ToolStatus:       "started",
			ToolInputSummary: summarizeToolValue(map[string]any{"command": args["cmd"], "workdir": args["workdir"]}),
			Payload:          params,
		})
		live.markReplayedExecCommand(callID)
		live.notifyAppServerTurn(threadID, appServerTurnResult{activity: "legacy:response_item:function_call", progress: true})
	case "function_call_output":
		callID := appServerString(params, "call_id")
		outputKey := callID + ":output"
		if callID == "" || live.hasReplayedExecCommand(outputKey) {
			return
		}
		output := appServerString(params, "output")
		status := legacyFunctionCallOutputStatus(output)
		output = m.decodeAndPublishStructuredCommandOutput(runtimeID, threadID, callID, status, output, params, live)
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:         runtimeID,
			SessionID:         threadID,
			Kind:              SessionEventToolCallUpdate,
			ToolCallID:        callID,
			ToolKind:          "exec_command",
			ToolTitle:         "Run shell command",
			ToolStatus:        status,
			ToolOutputSummary: summarizeToolValue(map[string]any{"output": output}),
			Payload:           params,
		})
		live.markReplayedExecCommand(outputKey)
		live.notifyAppServerTurn(threadID, appServerTurnResult{activity: "legacy:response_item:function_call_output", progress: true})
	}
}

func (m *appServerManager) decodeAndPublishStructuredCommandOutput(
	runtimeID string,
	threadID string,
	toolCallID string,
	status string,
	output string,
	payload any,
	live *liveSession,
) string {
	if !structuredOutputToolStatusSuccessful(status) || !strings.Contains(output, structuredOutputPrefix) {
		return output
	}
	cleaned, artifact, decodeErrors := decodeStructuredCommandOutput(output)
	m.publishStructuredCommandArtifact(runtimeID, threadID, toolCallID, artifact, decodeErrors, cleaned, live)
	return cleaned
}

func (m *appServerManager) publishStructuredCommandArtifact(
	runtimeID string,
	threadID string,
	toolCallID string,
	artifact activitypkg.StructuredOutputArtifact,
	decodeErrors []error,
	fallbackText string,
	live *liveSession,
) {
	for _, err := range decodeErrors {
		if live != nil && live.appClient != nil {
			live.appClient.logDebug("ignore invalid structured command output", "tool_call_id", toolCallID, "error", err)
		}
	}
	if structuredOutputArtifactEmpty(artifact) {
		return
	}
	m.publishAppServerEvent(SessionEvent{
		RuntimeID:  runtimeID,
		SessionID:  threadID,
		Kind:       SessionEventStructuredOutput,
		ToolCallID: toolCallID,
		ToolKind:   "exec_command",
		Text:       strings.TrimSpace(fallbackText),
		Payload:    artifact,
	})
	if artifact.RequestUserInput != nil {
		m.interruptAppServerStructuredOutputTurn(live, threadID)
	}
}

func structuredOutputToolStatusSuccessful(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "success", "succeeded":
		return true
	default:
		return false
	}
}

func (m *appServerManager) publishAppServerEvent(event SessionEvent) {
	if m.deps.EventSink == nil {
		return
	}
	event.RuntimeKind = agentruntime.KindCodex
	event.RuntimeID = strings.TrimSpace(event.RuntimeID)
	event.SessionID = strings.TrimSpace(event.SessionID)
	event.ReceivedAt = time.Now().UTC()
	m.deps.EventSink.Publish(event)
}

func (s *liveSession) appServerTracksThread(threadID string) bool {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != nil && strings.TrimSpace(s.session.SessionID) == threadID {
		return true
	}
	for _, sessionID := range s.conversationSessions {
		if strings.TrimSpace(sessionID) == threadID {
			return true
		}
	}
	_, ok := s.turnWaiters[threadID]
	return ok
}

func (m *appServerManager) appServerPrimaryThreadID(live *liveSession) string {
	if live == nil || live.session == nil {
		return ""
	}
	return strings.TrimSpace(live.session.SessionID)
}

// appServerLegacyThreadID assigns legacy notifications, which omit threadId,
// to the active turn that produced them. Codex can publish the same item over
// both the raw and legacy notification surfaces. Falling back to the runtime's
// original thread makes delivery depend on which duplicate arrives first and
// loses events from conversation-specific threads.
func (m *appServerManager) appServerLegacyThreadID(live *liveSession, params map[string]any) string {
	if live == nil {
		return ""
	}
	turnID := appServerNotificationTurnID(params)
	if turnID == "" {
		turnID = appServerNestedString(params, "internal_chat_message_metadata_passthrough", "turn_id")
	}

	live.mu.Lock()
	waiters := make(map[string]*appServerTurnWaiter, len(live.turnWaiters))
	for threadID, waiter := range live.turnWaiters {
		waiters[threadID] = waiter
	}
	live.mu.Unlock()

	if threadID := live.appServerThreadForTurn(turnID); threadID != "" {
		return threadID
	}
	if turnID != "" {
		for threadID, waiter := range waiters {
			if waiter != nil && waiter.currentTurnID() == turnID {
				return strings.TrimSpace(threadID)
			}
		}
	}
	if len(waiters) == 1 {
		for threadID := range waiters {
			return strings.TrimSpace(threadID)
		}
	}
	return m.appServerPrimaryThreadID(live)
}

func decodeLegacyFunctionCallArguments(raw string) map[string]any {
	var args map[string]any
	if raw == "" || json.Unmarshal([]byte(raw), &args) != nil {
		return map[string]any{}
	}
	return args
}

func appServerResponseItemText(params map[string]any) string {
	content, _ := params["content"].([]any)
	var parts []string
	for _, item := range content {
		part, _ := item.(map[string]any)
		if part == nil {
			continue
		}
		text := appServerString(part, "text")
		if text == "" {
			text = appServerString(part, "message")
		}
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func legacyFunctionCallOutputStatus(output string) string {
	lower := strings.ToLower(output)
	if strings.Contains(lower, "operation not permitted") ||
		(strings.Contains(lower, "exited with code ") && !strings.Contains(lower, "exited with code 0")) {
		return "failed"
	}
	return "completed"
}

func legacyMessageTurnResult(params map[string]any, activity string) appServerTurnResult {
	result := appServerTurnResult{activity: activity, progress: true}
	if strings.EqualFold(appServerString(params, "phase"), "final_answer") {
		result.success = true
		result.stopReason = StopReasonEndTurn
	}
	return result
}

func (s *liveSession) hasReplayedExecCommand(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.replayedExecCommands[key]
	return ok
}

func (s *liveSession) markReplayedExecCommand(key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.replayedExecCommands == nil {
		s.replayedExecCommands = make(map[string]struct{})
	}
	s.replayedExecCommands[key] = struct{}{}
}

func (s *liveSession) hasReplayedAgentMessage(params map[string]any) bool {
	key := replayedAgentMessageKey(params)
	if key == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.replayedAgentMessages[key]
	return ok
}

func (s *liveSession) markReplayedAgentMessage(params map[string]any) {
	key := replayedAgentMessageKey(params)
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.replayedAgentMessages == nil {
		s.replayedAgentMessages = make(map[string]struct{})
	}
	s.replayedAgentMessages[key] = struct{}{}
}

func replayedAgentMessageKey(params map[string]any) string {
	text := appServerString(params, "message")
	if text == "" {
		text = appServerResponseItemText(params)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return strings.TrimSpace(strings.ToLower(appServerString(params, "phase"))) + "\x00" + text
}

func appServerNotificationThreadID(params map[string]any) string {
	if threadID := appServerString(params, "threadId"); threadID != "" {
		return threadID
	}
	return appServerNestedString(params, "thread", "id")
}

func appServerNotificationTurnID(params map[string]any) string {
	if turnID := appServerString(params, "turnId"); turnID != "" {
		return turnID
	}
	return appServerNestedString(params, "turn", "id")
}

func appServerItemIsProgress(itemType string) bool {
	switch strings.TrimSpace(itemType) {
	case "agentMessage", "commandExecution", "fileChange", "mcpToolCall", "dynamicToolCall", "webSearch":
		return true
	default:
		return false
	}
}

func appServerItemSignalsAssistantActivity(itemType string) bool {
	itemType = strings.TrimSpace(itemType)
	return itemType != "" && itemType != "userMessage"
}

func appServerToolStatus(item map[string]any, fallback string) string {
	for _, key := range []string{"status", "state"} {
		if status := appServerString(item, key); status != "" {
			return status
		}
	}
	return fallback
}

func appServerToolTitle(item map[string]any, fallback string) string {
	for _, key := range []string{"title", "tool"} {
		if title := appServerString(item, key); title != "" {
			return title
		}
	}
	return fallback
}

func appServerString(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func appServerNestedString(values map[string]any, path ...string) string {
	var current any = values
	for _, key := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = m[key]
	}
	value, _ := current.(string)
	return strings.TrimSpace(value)
}
