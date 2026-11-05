package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
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
		decoder := json.NewDecoder(bytes.NewReader(note.Params))
		decoder.UseNumber()
		if err := decoder.Decode(&params); err != nil {
			if live.appClient != nil {
				live.appClient.logDebug("ignore malformed codex app-server notification params", "method", note.Method, "error", err)
			}
			return
		}
	}
	if params == nil {
		params = map[string]any{}
	}

	if waiter := live.appServerTurnWaiter(appServerString(params, "threadId")); waiter != nil {
		waiter.captureDiagnosticNotification(appServerNotificationTurnID(params), map[string]any{"method": note.Method, "params": params, "received_at": note.ReceivedAt})
	}

	if note.Method == "turn/completed" {
		at := note.ReceivedAt
		if at.IsZero() {
			at = time.Now()
		}
		threadID := appServerString(params, "threadId")
		if waiter := live.appServerTurnWaiter(threadID); waiter != nil {
			waiter.recordDiagnosticCompletion(appServerNotificationTurnID(params), at)
		}
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
	if isRawAppServerNotificationMethod(note.Method) {
		live.appServerProtocol(note.Method)
		m.handleRawAppServerNotification(runtimeID, live, note.Method, params)
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

func isRawAppServerNotificationMethod(method string) bool {
	return strings.HasPrefix(method, "turn/") ||
		strings.HasPrefix(method, "thread/") ||
		strings.HasPrefix(method, "item/") ||
		method == "error"
}

func appServerRequestIDValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		if value, err := typed.Int64(); err == nil {
			return fmt.Sprintf("%d", value)
		}
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
	turnID := appServerNotificationTurnID(params)
	turnEvent := turnID != "" || strings.HasPrefix(method, "turn/") || strings.HasPrefix(method, "item/") || method == "error"
	if method == "turn/completed" {
		threadID = live.nativeNotificationThread(threadID, turnID, false)
		if threadID != "" && live.appServerTracksThread(threadID) {
			m.handleRawTurnCompleted(runtimeID, live, threadID, params, nil)
		}
		return
	}
	if turnEvent {
		threadID, turnID = live.beginTurnNotification(threadID, turnID, notificationItemID(params))
		if turnID != "" && appServerNotificationTurnID(params) == "" {
			params["turnId"] = turnID
		}
	} else if threadID == "" {
		threadID = m.appServerPrimaryThreadID(live)
	}
	if threadID == "" || !live.appServerTracksThread(threadID) {
		return
	}
	live.trackAppServerTurn(threadID, turnID)

	switch method {
	case "thread/tokenUsage/updated":
		m.publishContextUsage(runtimeID, live, threadID, params, "")
		live.notifyAppServerTurn(threadID, appServerTurnResult{
			turnID:   appServerNotificationTurnID(params),
			activity: method,
			progress: true,
		})
	case "turn/started":
		live.notifyAppServerTurn(threadID, appServerTurnResult{
			turnID:   appServerNotificationTurnID(params),
			activity: "status:running",
			started:  true,
		})
	case "thread/status/changed":
		if strings.EqualFold(appServerNestedString(params, "status", "type"), "idle") {
			// Thread status has no turn identity. Only turn/completed can release
			// admission for the next prompt on this thread.
			live.notifyAppServerTurn(threadID, appServerTurnResult{activity: "status:idle"})
		}
	case "error":
		m.handleRawErrorNotification(runtimeID, live, threadID, params)
	default:
		if strings.HasPrefix(method, "item/") {
			if appServerNestedString(params, "item", "type") == "contextCompaction" {
				m.publishContextUsage(runtimeID, live, threadID, params, method)
			}
			m.handleRawItemNotification(runtimeID, live, threadID, method, params)
		}
	}
}

func (m *appServerManager) handleRawTurnCompleted(runtimeID string, live *liveSession, threadID string, params map[string]any, quietVersion *uint64) bool {
	live.mu.Lock()
	wasCompacting := live.contextUsage[threadID].Compacting
	live.mu.Unlock()
	status := strings.ToLower(appServerNestedString(params, "turn", "status"))
	if status == "" {
		status = strings.ToLower(appServerString(params, "status"))
	}
	waiter := live.appServerTurnWaiter(threadID)
	result := appServerTurnResult{turnID: appServerNotificationTurnID(params), activity: "turn:" + status}
	cancelInput := false
	switch status {
	case "", "completed", "success", "succeeded":
		result.success, result.stopReason, result.activity = true, StopReasonEndTurn, "turn:completed"
	case "cancelled", "canceled", "aborted", "interrupted":
		if waiter != nil && waiter.hasStructuredOutputBoundary() {
			result.success, result.stopReason, result.activity = true, StopReasonEndTurn, "turn:structured-output"
		} else {
			result.err = fmt.Errorf("codex turn %s", status)
			cancelInput = true
		}
	default:
		errMsg := appServerNestedString(params, "turn", "error", "message")
		if errMsg == "" {
			errMsg = "codex turn failed"
		}
		result.err = fmt.Errorf("%s", errMsg)
		if wasCompacting {
			result.err = errContextCompaction
		} else if turn, _ := params["turn"].(map[string]any); turn != nil {
			if details, _ := turn["error"].(map[string]any); details != nil && contextErrorInfo(details) {
				result.err = &contextWindowError{}
			}
		}
	}
	turn, _ := params["turn"].(map[string]any)
	items, _ := turn["items"].([]any)
	waiter, accepted := live.recordNativeCompletion(threadID, &result, quietVersion, items)
	if !accepted {
		return false
	}
	if wasCompacting {
		defer m.publishContextUsage(runtimeID, live, threadID, nil, "item/completed")
	}
	defer live.clearAppServerCommandOutputs(threadID, result.turnID)
	if cancelInput && m.deps.UserInput != nil {
		if result.turnID != "" {
			m.deps.UserInput.CancelTurn(runtimeID, threadID, result.turnID)
		} else {
			m.deps.UserInput.CancelSession(runtimeID, threadID)
		}
	}
	if waiter != nil {
		if quietVersion == nil {
			waiter.wake(result)
		}
		return true
	}
	if result.success {
		m.publishAppServerEvent(promptCompletedEvent(runtimeID, threadID, PromptResponse{StopReason: result.stopReason}))
	} else {
		m.publishAppServerEvent(SessionEvent{
			RuntimeID: runtimeID, SessionID: threadID, Kind: SessionEventPromptFailed,
			Error: result.err.Error(), Payload: params,
		})
	}
	return true
}

func (m *appServerManager) handleRawErrorNotification(runtimeID string, live *liveSession, threadID string, params map[string]any) {
	live.mu.Lock()
	compacting := live.contextUsage[threadID].Compacting
	live.mu.Unlock()
	if compacting {
		return
	} // Native turn/completed owns the compaction outcome.

	if details, _ := params["error"].(map[string]any); details != nil && contextErrorInfo(details) {
		return
	} // Wait for turn/completed before any recovery.
	willRetry, _ := params["willRetry"].(bool)
	activity := "error:retry"
	if !willRetry {
		activity = "error:terminal"
	}
	errMsg := appServerNestedString(params, "error", "message")
	if errMsg == "" {
		errMsg = appServerString(params, "message")
	}
	result := appServerTurnResult{turnID: appServerNotificationTurnID(params), activity: activity}
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
	// Streaming progress carries itemId rather than a complete item object.
	// Account for it before the item lifecycle handler below.
	switch method {
	case "item/reasoning/textDelta", "item/reasoning/summaryTextDelta", "item/reasoning/summaryPartAdded", "item/mcpToolCall/progress":
		live.notifyAppServerTurn(threadID, appServerTurnResult{
			turnID:   appServerNotificationTurnID(params),
			activity: strings.TrimPrefix(method, "item/") + ":" + appServerString(params, "itemId"),
			progress: true,
		})
		return
	}
	if method == "item/agentMessage/delta" {
		itemID := appServerString(params, "itemId")
		delta, _ := params["delta"].(string)
		if itemID != "" && delta != "" {
			payload := cloneAppServerParams(params)
			phase := strings.TrimSpace(strings.ToLower(appServerString(payload, "phase")))
			if phase == "" {
				phase = live.agentMessagePhase(itemID)
			}
			if phase == "" {
				phase = "unknown"
			}
			payload["phase"] = phase
			live.setAgentMessagePhase(itemID, phase)
			live.markStreamedAgentMessage(itemID)
			if phase == "final_answer" {
				live.markStreamedAgentThread(threadID)
			}
			if cleanedDelta := live.filterAgentMessageDelta(itemID, delta); cleanedDelta != "" {
				m.publishAppServerEvent(SessionEvent{
					RuntimeID: runtimeID,
					SessionID: threadID,
					TurnID:    appServerNotificationTurnID(params),
					Kind:      SessionEventTextDelta,
					MessageID: itemID,
					Text:      cleanedDelta,
					Payload:   payload,
				})
			}
		}
		live.notifyAppServerTurn(threadID, appServerTurnResult{
			turnID:   appServerNotificationTurnID(params),
			activity: "agentMessage:delta:" + itemID,
			progress: true,
		})
		return
	}
	if method == "item/commandExecution/outputDelta" {
		itemID := appServerString(params, "itemId")
		preview := live.appendAppServerCommandOutput(threadID, appServerNotificationTurnID(params), itemID, appServerString(params, "delta"))
		if preview != "" {
			m.publishAppServerEvent(SessionEvent{RuntimeID: runtimeID, SessionID: threadID, TurnID: appServerNotificationTurnID(params), Kind: SessionEventToolCallUpdate, ToolCallID: itemID, ToolKind: "exec_command", ToolStatus: "running", Payload: map[string]any{"output": preview, "preview_truncated": len(preview) >= appServerCommandOutputPreviewBytes}})
		}
		live.notifyAppServerTurn(threadID, appServerTurnResult{
			turnID:   appServerNotificationTurnID(params),
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
		turnID:   appServerNotificationTurnID(params),
		activity: activity,
		progress: appServerItemIsProgress(itemType),
	})

	switch {
	case method == "item/started" && itemType == "agentMessage":
		phase := appServerString(item, "phase")
		if strings.TrimSpace(phase) == "" {
			// Some OpenAI-compatible providers, including qwen3.6-plus,
			// omit the Codex-specific phase field. A typed agentMessage item
			// is still streamable assistant output.
			phase = "unknown"
		}
		live.setAgentMessagePhase(itemID, phase)
	case method == "item/completed" && itemType == "agentMessage":
		defer live.clearAgentMessageState(itemID)
		previousPhase := live.agentMessagePhase(itemID)
		live.setAgentMessagePhase(itemID, appServerString(item, "phase"))
		text := appServerString(item, "text")
		cleanedText := m.decodeAndPublishStructuredAssistantOutput(runtimeID, threadID, itemID, text, live)
		if live.hasStreamedAgentMessage(itemID) {
			if live.agentMessageNeedsReplacement(itemID, cleanedText) {
				payload := cloneAppServerParams(item)
				payload["text_snapshot"] = true
				m.publishAppServerEvent(SessionEvent{RuntimeID: runtimeID, SessionID: threadID, MessageID: itemID, Kind: SessionEventTextDelta, Text: cleanedText, Payload: payload})
				return
			}

			if remainder := live.finishAgentMessageStream(itemID, cleanedText); remainder != "" || (cleanedText != "" && appServerString(item, "phase") != "" && previousPhase != appServerString(item, "phase")) {
				m.publishAppServerEvent(SessionEvent{
					RuntimeID: runtimeID,
					SessionID: threadID,
					Kind:      SessionEventTextDelta,
					MessageID: itemID,
					Text:      remainder,
					Payload:   item,
				})
			}
			return
		}
		if cleanedText != "" {
			m.publishAppServerEvent(SessionEvent{
				RuntimeID: runtimeID,
				SessionID: threadID,
				Kind:      SessionEventTextDelta,
				MessageID: itemID,
				Text:      cleanedText,
				Payload:   item,
			})
		}
		// A final-answer item can precede native turn completion. Keep the
		// waiter active until turn/completed so turn/start cannot steer it.
	default:
		if method != "item/started" && method != "item/completed" {
			return
		}
		completed := method == "item/completed"
		if completed && itemType == "commandExecution" {
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
				output = m.decodeAndPublishStructuredToolOutput(runtimeID, threadID, itemID, "exec_command", status, output, live)
			}
			item = cloneAppServerParams(item)
			item["aggregatedOutput"] = output
		}
		if event, ok := nativeToolEvent(item, completed); ok {
			event.RuntimeID, event.SessionID = runtimeID, threadID
			event.TurnID = appServerNotificationTurnID(params)
			m.publishAppServerEvent(event)
		}
	}
}

func cloneAppServerParams(params map[string]any) map[string]any {
	cloned := make(map[string]any, len(params)+1)
	for key, value := range params {
		cloned[key] = value
	}
	return cloned
}

func (s *liveSession) setAgentMessagePhase(itemID, phase string) {
	itemID = strings.TrimSpace(itemID)
	phase = strings.TrimSpace(strings.ToLower(phase))
	if itemID == "" || phase == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.agentMessagePhases == nil {
		s.agentMessagePhases = make(map[string]string)
	}
	s.agentMessagePhases[itemID] = phase
}

func (s *liveSession) agentMessagePhase(itemID string) string {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.agentMessagePhases[itemID]
}

func (s *liveSession) clearAgentMessageState(itemID string) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return
	}
	s.mu.Lock()
	delete(s.agentMessagePhases, itemID)
	delete(s.streamedAgentMessages, itemID)
	delete(s.agentMessageStreams, itemID)
	s.mu.Unlock()
}

func (s *liveSession) filterAgentMessageDelta(itemID, delta string) string {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" || delta == "" {
		return delta
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.agentMessageStreams == nil {
		s.agentMessageStreams = make(map[string]*assistantStructuredOutputStream)
	}
	stream := s.agentMessageStreams[itemID]
	if stream == nil {
		stream = newAssistantStructuredOutputStream()
		s.agentMessageStreams[itemID] = stream
	}
	return stream.append(delta)
}

func (s *liveSession) finishAgentMessageStream(itemID, cleaned string) string {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return cleaned
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stream := s.agentMessageStreams[itemID]
	if stream == nil {
		return cleaned
	}
	return stream.finish(cleaned)
}

func (s *liveSession) markStreamedAgentMessage(itemID string) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.streamedAgentMessages == nil {
		s.streamedAgentMessages = make(map[string]struct{})
	}
	s.streamedAgentMessages[itemID] = struct{}{}
}

func (s *liveSession) hasStreamedAgentMessage(itemID string) bool {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.streamedAgentMessages[itemID]
	return ok
}

func (s *liveSession) markStreamedAgentThread(threadID string) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.streamedAgentThreads == nil {
		s.streamedAgentThreads = make(map[string]struct{})
	}
	s.streamedAgentThreads[threadID] = struct{}{}
}

func (s *liveSession) hasStreamedAgentThread(threadID string) bool {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.streamedAgentThreads[threadID]
	return ok
}

func (s *liveSession) clearStreamedAgentThread(threadID string) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return
	}
	s.mu.Lock()
	delete(s.streamedAgentThreads, threadID)
	s.mu.Unlock()
}

func (m *appServerManager) handleLegacyAppServerEvent(runtimeID string, live *liveSession, params map[string]any) {
	threadID := m.appServerLegacyThreadID(live, params)
	if threadID == "" {
		return
	}
	msgType := appServerString(params, "type")
	switch msgType {
	case "task_started":
		live.clearStreamedAgentThread(threadID)
		live.notifyAppServerTurn(threadID, appServerTurnResult{turnID: legacyNotificationTurnID(params), activity: "status:running", started: true})
	case "agent_message":
		text := appServerString(params, "message")
		text = m.decodeAndPublishStructuredAssistantOutput(runtimeID, threadID, appServerString(params, "id"), text, live)
		if text != "" {
			if live.hasStreamedAgentThread(threadID) {
				live.notifyAppServerTurn(threadID, legacyMessageTurnResult(params, "legacy:agent_message"))
				return
			}
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
		if _, ok := live.recordLegacyTool(threadID, callID, legacyToolCall{name: "exec_command"}); !ok {
			return
		}
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
		call, ok := live.recordLegacyTool(threadID, callID, legacyToolCall{name: "exec_command", finished: true})
		if !ok {
			return
		}
		output := firstNonEmpty(appServerString(params, "aggregated_output"), appServerString(params, "output"), appServerString(params, "formatted_output"))
		status := legacyToolEndStatus(params, output)
		output = m.decodeAndPublishStructuredToolOutput(runtimeID, threadID, callID, call.name, status, output, live)
		kind, title, payload := legacyResultTool(call.name, params)
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:         runtimeID,
			SessionID:         threadID,
			Kind:              SessionEventToolCallUpdate,
			ToolCallID:        callID,
			ToolKind:          kind,
			ToolTitle:         title,
			ToolStatus:        status,
			ToolOutputSummary: summarizeToolValue(map[string]any{"output": output}),
			Payload:           payload,
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
			ToolStatus: legacyToolEndStatus(params, ""),
			Payload:    params,
		})
		live.notifyAppServerTurn(threadID, appServerTurnResult{activity: "legacy:patch_apply_end", progress: true})
	case "task_complete":
		if !live.notifyAppServerTurn(threadID, appServerTurnResult{turnID: legacyNotificationTurnID(params), success: true, stopReason: StopReasonEndTurn, activity: "legacy:task_complete"}) {
			m.publishAppServerEvent(promptCompletedEvent(runtimeID, threadID, PromptResponse{StopReason: StopReasonEndTurn}))
		}
	case "turn_aborted":
		if waiter := live.appServerTurnWaiter(threadID); waiter != nil && waiter.hasStructuredOutputBoundary() {
			if !live.notifyAppServerTurn(threadID, appServerTurnResult{turnID: legacyNotificationTurnID(params), success: true, stopReason: StopReasonEndTurn, activity: "legacy:turn_structured-output"}) {
				m.publishAppServerEvent(promptCompletedEvent(runtimeID, threadID, PromptResponse{StopReason: StopReasonEndTurn}))
			}
			return
		}
		err := fmt.Errorf("codex turn aborted")
		if !live.notifyAppServerTurn(threadID, appServerTurnResult{turnID: legacyNotificationTurnID(params), err: err, activity: "legacy:turn_aborted"}) {
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
		text = m.decodeAndPublishStructuredAssistantOutput(runtimeID, threadID, appServerString(params, "id"), text, live)
		if text != "" {
			if live.hasStreamedAgentThread(threadID) {
				live.notifyAppServerTurn(threadID, legacyMessageTurnResult(params, "legacy:response_item:message"))
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
		}
		live.markReplayedAgentMessage(params)
		live.notifyAppServerTurn(threadID, legacyMessageTurnResult(params, "legacy:response_item:message"))
	case "function_call":
		callID := appServerString(params, "call_id")
		name := appServerString(params, "name")
		args := decodeLegacyFunctionCallArguments(appServerString(params, "arguments"))
		call := legacyToolCall{name: name}
		if name == "write_stdin" {
			call.ptySession = appServerRequestIDValue(args["session_id"])
		}
		_, ok := live.recordLegacyTool(threadID, callID, call)
		if !ok {
			return
		}
		kind, title := "dynamic_tool_call", name
		payload := map[string]any{"tool": name, "arguments": args}
		if name == "exec_command" {
			kind, title = name, "Run shell command"
			payload = map[string]any{"command": args["cmd"], "workdir": args["workdir"]}
			args = payload
		}
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:        runtimeID,
			SessionID:        threadID,
			Kind:             SessionEventToolCallStart,
			ToolCallID:       callID,
			ToolKind:         kind,
			ToolTitle:        title,
			ToolStatus:       "started",
			ToolInputSummary: summarizeToolValue(args),
			Payload:          payload,
		})
		live.notifyAppServerTurn(threadID, appServerTurnResult{activity: "legacy:response_item:function_call", progress: true})
	case "function_call_output":
		callID := appServerString(params, "call_id")
		call, ok := live.recordLegacyTool(threadID, callID, legacyToolCall{finished: true})
		if !ok {
			return
		}
		name := call.name
		status, output := m.legacyFunctionResult(runtimeID, threadID, callID, call, params, live)
		kind, title, payload := legacyResultTool(name, params)
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:         runtimeID,
			SessionID:         threadID,
			Kind:              SessionEventToolCallUpdate,
			ToolCallID:        callID,
			ToolKind:          kind,
			ToolTitle:         title,
			ToolStatus:        status,
			ToolOutputSummary: summarizeToolValue(map[string]any{"output": output}),
			Payload:           payload,
		})
		live.notifyAppServerTurn(threadID, appServerTurnResult{activity: "legacy:response_item:function_call_output", progress: true})
	}
}

func legacyResultTool(name string, params map[string]any) (string, string, map[string]any) {
	switch name {
	case "":
		return "", "", params
	case "exec_command":
		return name, "Run shell command", params
	default:
		payload := cloneAppServerParams(params)
		payload["tool"] = name
		return "dynamic_tool_call", name, payload
	}
}

func (m *appServerManager) decodeAndPublishStructuredAssistantOutput(
	runtimeID string,
	threadID string,
	messageID string,
	output string,
	live *liveSession,
) string {
	if !strings.Contains(output, "csgclaw-output::") {
		return output
	}
	cleaned, artifact, decodeErrors := decodeStructuredCommandOutput(output)
	m.publishStructuredOutputArtifact(runtimeID, threadID, messageID, "assistant_message", artifact, decodeErrors, cleaned, live)
	return cleaned
}

func (m *appServerManager) decodeAndPublishStructuredToolOutput(
	runtimeID, threadID, toolCallID, toolKind, status, output string,
	live *liveSession,
) string {
	if !structuredOutputToolStatusSuccessful(status) || !strings.Contains(output, structuredOutputPrefix) {
		return output
	}
	cleaned, artifact, decodeErrors := decodeStructuredCommandOutput(output)
	m.publishStructuredOutputArtifact(runtimeID, threadID, toolCallID, toolKind, artifact, decodeErrors, cleaned, live)
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
	m.publishStructuredOutputArtifact(runtimeID, threadID, toolCallID, "exec_command", artifact, decodeErrors, fallbackText, live)
}

func (m *appServerManager) publishStructuredOutputArtifact(
	runtimeID string,
	threadID string,
	sourceID string,
	sourceKind string,
	artifact activitypkg.StructuredOutputArtifact,
	decodeErrors []error,
	fallbackText string,
	live *liveSession,
) {
	for _, err := range decodeErrors {
		if live != nil && live.appClient != nil {
			live.appClient.logDebug("ignore invalid structured output", "source_id", sourceID, "source_kind", sourceKind, "error", err)
		}
	}
	if structuredOutputArtifactEmpty(artifact) {
		return
	}
	m.publishAppServerEvent(SessionEvent{
		RuntimeID:  runtimeID,
		SessionID:  threadID,
		Kind:       SessionEventStructuredOutput,
		ToolCallID: sourceID,
		ToolKind:   sourceKind,
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
	event.RuntimeKind = agentruntime.KindCodex
	event.RuntimeID = strings.TrimSpace(event.RuntimeID)
	event.SessionID = strings.TrimSpace(event.SessionID)
	if live := m.liveSession(event.RuntimeID); live != nil && live.isMemoryMaintenanceThread(event.SessionID) {
		return
	}
	if live := m.liveSession(event.RuntimeID); live != nil {
		live.mu.Lock()
		compacting := live.compactingThreads[event.SessionID]
		waiter := live.turnWaiters[event.SessionID]
		live.mu.Unlock()
		if compacting && event.Kind != activitypkg.RuntimeEventContextUsage && event.Kind != activitypkg.RuntimeEventTokenUsage {
			return
		}
		if waiter != nil && contextVisibleEvent(event.Kind) {
			waiter.mu.Lock()
			waiter.visibleActivity = true
			waiter.mu.Unlock()
		}
	}
	event.ReceivedAt = time.Now().UTC()
	if m.deps.EventSink != nil {
		m.deps.EventSink.Publish(event)
	}
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

func (s *liveSession) appServerPublishesFilesForThread(threadID string) bool {
	threadID = strings.TrimSpace(threadID)
	if s == nil || threadID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.filePublishingThreads[threadID]
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
	turnID := legacyNotificationTurnID(params)
	itemID := appServerString(params, "call_id")
	if itemID == "" {
		itemID = appServerString(params, "id")
	}
	threadID, owner := live.beginTurnNotification(appServerNotificationThreadID(params), turnID, itemID)
	if turnID == "" && owner != "" {
		params["turnId"] = owner
	}
	return threadID
}

func notificationItemID(params map[string]any) string {
	if id := appServerString(params, "itemId"); id != "" {
		return id
	}
	return appServerNestedString(params, "item", "id")
}

func legacyNotificationTurnID(params map[string]any) string {
	if turnID := appServerNotificationTurnID(params); turnID != "" {
		return turnID
	}
	if turnID := appServerString(params, "turn_id"); turnID != "" {
		return turnID
	}
	return appServerNestedString(params, "internal_chat_message_metadata_passthrough", "turn_id")
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

func legacyFunctionCallOutputText(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	items, _ := value.([]any)
	var parts []string
	for _, item := range items {
		part, _ := item.(map[string]any)
		if part["type"] == "input_text" {
			if text, ok := part["text"].(string); ok {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func legacyToolEndStatus(params map[string]any, output string) string {
	if status := legacyExplicitToolEndStatus(params); status != "" {
		return status
	}
	return legacyFunctionCallOutputStatus(output)
}

func legacyExplicitToolEndStatus(params map[string]any) string {
	if status := activitypkg.TerminalToolStatus(appServerToolStatus(params, "")); status != "" {
		return status
	}
	success, hasSuccess := params["success"].(bool)
	if code, ok := params["exit_code"].(json.Number); ok {
		if exitCode, err := code.Int64(); err == nil {
			success, hasSuccess = exitCode == 0, true
		}
	}
	if !hasSuccess {
		return ""
	}
	if success {
		return "completed"
	}
	return "failed"
}

func (m *appServerManager) legacyFunctionResult(runtimeID, threadID, callID string, call legacyToolCall, params map[string]any, live *liveSession) (string, string) {
	output := legacyFunctionCallOutputText(params["output"])
	status := legacyExplicitToolEndStatus(params)
	switch call.name {
	case "", "exec_command", "shell_command", "write_stdin":
	default:
		return firstNonEmpty(status, "completed"), output
	}
	process, runningSession := legacyShellProcessStatus(output)
	if runningSession != "" {
		// Each PTY poll returns to its caller while the process continues.
		// Subsequent polls only carry new output.
		if (status == "" || status == "completed") && strings.Contains(output, structuredOutputPrefix) {
			cleaned, artifact, _ := decodeStructuredCommandOutput(output)
			if !structuredOutputArtifactEmpty(artifact) {
				live.holdLegacyShellArtifact(threadID, runningSession, cleaned, artifact)
			}
		}
		return firstNonEmpty(status, "completed"), output
	}
	status = firstNonEmpty(status, process)
	if status == "" {
		header, _, formatted := strings.Cut(output, "\nOutput:")
		if formatted && (strings.HasPrefix(header, "Wall time:") || strings.HasPrefix(header, "Chunk ID:")) {
			// A formatted poll without an exit status cannot confirm success.
			return "completed", output
		}
		// Older function_call_output records can contain only the command's stdout.
		status = legacyFunctionCallOutputStatus(output)
		if call.name == "write_stdin" && strings.HasPrefix(output, "write_stdin failed:") {
			status = "failed"
		}
	}
	pending := live.takeLegacyShellArtifact(threadID, call.ptySession)
	if pending == nil || status != "completed" {
		return status, m.decodeAndPublishStructuredToolOutput(runtimeID, threadID, callID, call.name, status, output, live)
	}
	cleaned, artifact, decodeErrors := decodeStructuredCommandOutput(output)
	pending.mergeArtifact(artifact)
	pending.appendCleaned(cleaned, false)
	m.publishStructuredOutputArtifact(runtimeID, threadID, callID, call.name, pending.artifact,
		append(pending.decodeErrors, decodeErrors...), pending.cleanedPreview.String(), live)
	return status, cleaned
}

// Codex puts process metadata before Output:. Text printed by the program may
// contain the same words and must not change the reported exit status.
func legacyShellProcessStatus(output string) (status, runningSession string) {
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "Output:" {
			break
		}
		if id, ok := strings.CutPrefix(line, "Process running with session ID "); ok {
			if _, err := strconv.Atoi(id); err == nil {
				return "", id
			}
		}
		value, ok := strings.CutPrefix(line, "Process exited with code ")
		if !ok {
			value, ok = strings.CutPrefix(line, "Exit code: ")
		}
		if ok {
			if code, err := strconv.Atoi(value); err == nil {
				if code == 0 {
					status = "completed"
				} else {
					status = "failed"
				}
			}
		}
	}
	return status, ""
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
	result := appServerTurnResult{turnID: legacyNotificationTurnID(params), activity: activity, progress: true}
	if strings.EqualFold(appServerString(params, "phase"), "final_answer") {
		result.success = true
		result.stopReason = StopReasonEndTurn
	}
	return result
}

type legacyToolCall struct {
	name       string
	finished   bool
	ptySession string
}

type legacyTurnTools struct {
	calls          map[string]legacyToolCall
	pendingOutputs map[string]*appServerCommandOutputAccumulator
}

func (s *liveSession) holdLegacyShellArtifact(threadID, ptySession, cleaned string, artifact activitypkg.StructuredOutputArtifact) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tools := s.legacyTools[threadID]
	if tools == nil {
		return
	}
	if tools.pendingOutputs == nil {
		tools.pendingOutputs = make(map[string]*appServerCommandOutputAccumulator)
	}
	output := tools.pendingOutputs[ptySession]
	if output == nil {
		output = &appServerCommandOutputAccumulator{}
		tools.pendingOutputs[ptySession] = output
	}
	output.mergeArtifact(artifact)
	output.appendCleaned(cleaned, true)
}

func (s *liveSession) takeLegacyShellArtifact(threadID, ptySession string) *appServerCommandOutputAccumulator {
	if ptySession == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tools := s.legacyTools[threadID]
	if tools == nil {
		return nil
	}
	output := tools.pendingOutputs[ptySession]
	delete(tools.pendingOutputs, ptySession)
	return output
}

// Rollout results carry only a call ID. Typed exec events may describe the same call.
func (s *liveSession) recordLegacyTool(threadID, callID string, call legacyToolCall) (legacyToolCall, bool) {
	if callID == "" || (!call.finished && call.name == "") {
		return legacyToolCall{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tools := s.legacyTools[threadID]
	if tools == nil {
		if s.legacyTools == nil {
			s.legacyTools = make(map[string]*legacyTurnTools)
		}
		tools = &legacyTurnTools{calls: make(map[string]legacyToolCall)}
		s.legacyTools[threadID] = tools
	}
	if previous, ok := tools.calls[callID]; ok {
		// shell_command also emits an exec_command_end with its function call ID.
		shellEnd := call.name == "exec_command" && previous.name == "shell_command"
		if !call.finished || previous.finished || (call.name != "" && call.name != previous.name && !shellEnd) {
			return legacyToolCall{}, false
		}
		call = previous
		call.finished = true
	}
	tools.calls[callID] = call
	return call, true
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
	case "agentMessage", "commandExecution", "fileChange", "mcpToolCall", "dynamicToolCall", "webSearch", "reasoning", "contextCompaction":
		return true
	default:
		return false
	}
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

func (s *liveSession) agentMessageNeedsReplacement(itemID, completed string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	stream := s.agentMessageStreams[itemID]
	return stream != nil && !strings.HasPrefix(completed, stream.emitted.String())
}
