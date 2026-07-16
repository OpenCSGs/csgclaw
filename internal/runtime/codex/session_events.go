package codex

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	agentruntime "csgclaw/internal/runtime"
)

func permissionRequestEvent(state permissionState) SessionEvent {
	snapshot := state.snapshot
	execution := state.execution
	return SessionEvent{
		RuntimeKind:  agentruntime.KindCodex,
		RuntimeID:    strings.TrimSpace(execution.RuntimeID),
		SessionID:    strings.TrimSpace(execution.SessionID),
		Kind:         SessionEventPermissionRequest,
		ReceivedAt:   time.Now().UTC(),
		ToolCallID:   strings.TrimSpace(execution.ToolCallID),
		ToolKind:     strings.TrimSpace(execution.ToolKind),
		ToolTitle:    strings.TrimSpace(snapshot.Title),
		ActionID:     strings.TrimSpace(snapshot.ID),
		ActionStatus: string(snapshot.Status),
		Payload:      snapshot,
	}
}

func permissionDecisionEvent(state permissionState) SessionEvent {
	snapshot := state.snapshot
	execution := state.execution
	event := SessionEvent{
		RuntimeKind:  agentruntime.KindCodex,
		RuntimeID:    strings.TrimSpace(execution.RuntimeID),
		SessionID:    strings.TrimSpace(execution.SessionID),
		Kind:         SessionEventPermissionDecision,
		ReceivedAt:   time.Now().UTC(),
		ToolCallID:   strings.TrimSpace(execution.ToolCallID),
		ToolKind:     strings.TrimSpace(execution.ToolKind),
		ToolTitle:    strings.TrimSpace(snapshot.Title),
		ActionID:     strings.TrimSpace(snapshot.ID),
		ActionStatus: string(snapshot.Status),
		Payload:      snapshot,
	}
	if snapshot.Decision != nil {
		event.ActionOptionID = strings.TrimSpace(snapshot.Decision.OptionID)
		event.ActionOptionKind = strings.TrimSpace(snapshot.Decision.Kind)
	}
	return event
}

func userInputRequestEvent(state userInputState) SessionEvent {
	snapshot := publicUserInputSnapshot(state.snapshot)
	return SessionEvent{
		RuntimeKind:     agentruntime.KindCodex,
		RuntimeID:       strings.TrimSpace(state.execution.RuntimeID),
		SessionID:       strings.TrimSpace(state.execution.SessionID),
		TurnID:          strings.TrimSpace(state.execution.TurnID),
		Kind:            SessionEventUserInputRequest,
		ReceivedAt:      time.Now().UTC(),
		ToolCallID:      strings.TrimSpace(state.execution.ToolCallID),
		ToolKind:        "request_user_input",
		ToolTitle:       "Question",
		UserInputID:     strings.TrimSpace(snapshot.ID),
		UserInputStatus: string(snapshot.Status),
		Payload:         snapshot,
	}
}

func userInputResolvedEvent(state userInputState) SessionEvent {
	snapshot := publicUserInputSnapshot(state.snapshot)
	return SessionEvent{
		RuntimeKind:     agentruntime.KindCodex,
		RuntimeID:       strings.TrimSpace(state.execution.RuntimeID),
		SessionID:       strings.TrimSpace(state.execution.SessionID),
		TurnID:          strings.TrimSpace(state.execution.TurnID),
		Kind:            SessionEventUserInputResolved,
		ReceivedAt:      time.Now().UTC(),
		ToolCallID:      strings.TrimSpace(state.execution.ToolCallID),
		ToolKind:        "request_user_input",
		ToolTitle:       "Question",
		UserInputID:     strings.TrimSpace(snapshot.ID),
		UserInputStatus: string(snapshot.Status),
		Payload:         snapshot,
	}
}

func promptCompletedEvent(runtimeID string, sessionID string, resp PromptResponse) SessionEvent {
	return SessionEvent{
		RuntimeKind: agentruntime.KindCodex,
		RuntimeID:   strings.TrimSpace(runtimeID),
		SessionID:   strings.TrimSpace(sessionID),
		Kind:        SessionEventPromptCompleted,
		ReceivedAt:  time.Now().UTC(),
		MessageID:   strings.TrimSpace(resp.MessageID),
		StopReason:  strings.TrimSpace(resp.StopReason),
		Payload:     resp,
	}
}

func promptFailedEvent(runtimeID string, sessionID string, err error) SessionEvent {
	return SessionEvent{
		RuntimeKind: agentruntime.KindCodex,
		RuntimeID:   strings.TrimSpace(runtimeID),
		SessionID:   strings.TrimSpace(sessionID),
		Kind:        SessionEventPromptFailed,
		ReceivedAt:  time.Now().UTC(),
		Error:       errorString(err),
		Payload:     err,
	}
}

func summarizeToolValue(value any) string {
	if value == nil {
		return ""
	}
	sanitized := redactToolValue(value)
	data, err := json.Marshal(sanitized)
	if err != nil {
		return ""
	}
	return truncateSummary(string(data), 240)
}

func redactToolValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSecretishKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = redactToolValue(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactToolValue(item))
		}
		return out
	case string:
		return redactSecretishText(typed)
	default:
		return value
	}
}

func isSecretishKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "token") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "password") ||
		strings.Contains(key, "api_key") ||
		strings.Contains(key, "apikey")
}

func truncateSummary(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "..."
}

func redactSecretishText(value string) string {
	value = redactBearerTokenText(value)
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "token=") ||
			strings.Contains(lower, "token:") ||
			strings.Contains(lower, "api_key=") ||
			strings.Contains(lower, "api_key:") ||
			strings.Contains(lower, "apikey=") ||
			strings.Contains(lower, "apikey:") ||
			strings.Contains(lower, "authorization: bearer ") {
			lines[i] = redactSecretishLine(line)
		}
	}
	return strings.Join(lines, "\n")
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

var bearerTokenPattern = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-]+`)
var apiKeyLikePattern = regexp.MustCompile(`\bsk-[A-Za-z0-9._\-]+\b`)

func redactBearerTokenText(value string) string {
	value = bearerTokenPattern.ReplaceAllString(value, "Bearer [redacted]")
	return apiKeyLikePattern.ReplaceAllString(value, "[redacted]")
}
