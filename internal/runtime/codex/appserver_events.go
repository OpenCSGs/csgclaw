package codex

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

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

	protocol := live.appServerProtocol(note.Method)
	if protocol == appServerProtocolLegacy {
		m.handleLegacyAppServerEvent(runtimeID, live, params)
		return
	}
	if protocol == appServerProtocolRaw {
		m.handleRawAppServerNotification(runtimeID, live, note.Method, params)
	}
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
	switch status {
	case "", "completed", "success", "succeeded":
		if !live.notifyAppServerTurn(threadID, appServerTurnResult{success: true, stopReason: StopReasonEndTurn, turnID: turnID, activity: "turn:completed"}) {
			if live.consumeFallbackCompleted(threadID) {
				if live.appClient != nil {
					live.appClient.logDebug("codex app-server ignored duplicate turn completion after agent message",
						"runtime_id", runtimeID,
						"thread_id", threadID,
						"turn_id", turnID,
					)
				}
				return
			}
			m.publishAppServerEvent(promptCompletedEvent(runtimeID, threadID, PromptResponse{StopReason: StopReasonEndTurn}))
		}
	case "cancelled", "canceled", "aborted", "interrupted":
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
	item, _ := params["item"].(map[string]any)
	if item == nil {
		return
	}
	itemType := appServerString(item, "type")
	itemID := appServerString(item, "id")
	activity := strings.Trim(strings.TrimPrefix(method, "item/")+":"+itemType+":"+itemID, ":")
	live.notifyAppServerTurn(threadID, appServerTurnResult{activity: activity, progress: appServerItemIsProgress(itemType)})

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
		output := appServerString(item, "aggregatedOutput")
		if output == "" {
			output = appServerString(item, "output")
		}
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:         runtimeID,
			SessionID:         threadID,
			Kind:              SessionEventToolCallUpdate,
			ToolCallID:        itemID,
			ToolKind:          "exec_command",
			ToolTitle:         "Run shell command",
			ToolStatus:        appServerToolStatus(item, "completed"),
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
		if live.notifyAppServerTurn(threadID, appServerTurnResult{success: true, stopReason: StopReasonEndTurn, activity: "item:agentMessage:completed", progress: true}) {
			live.markFallbackCompleted(threadID)
			if live.appClient != nil {
				live.appClient.logDebug("codex app-server completed turn from agent message",
					"runtime_id", runtimeID,
					"thread_id", threadID,
					"item_id", itemID,
				)
			}
		}
	}
}

func (m *appServerManager) handleLegacyAppServerEvent(runtimeID string, live *liveSession, params map[string]any) {
	threadID := m.appServerPrimaryThreadID(live)
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
			m.publishAppServerEvent(SessionEvent{
				RuntimeID: runtimeID,
				SessionID: threadID,
				Kind:      SessionEventTextDelta,
				Text:      text,
				Payload:   params,
			})
			live.notifyAppServerTurn(threadID, appServerTurnResult{activity: "legacy:agent_message", progress: true})
		}
	case "exec_command_begin":
		callID := appServerString(params, "call_id")
		command := appServerString(params, "command")
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
		m.publishAppServerEvent(SessionEvent{
			RuntimeID:         runtimeID,
			SessionID:         threadID,
			Kind:              SessionEventToolCallUpdate,
			ToolCallID:        callID,
			ToolKind:          "exec_command",
			ToolTitle:         "Run shell command",
			ToolStatus:        "completed",
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
	case "agentMessage", "commandExecution", "fileChange":
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
