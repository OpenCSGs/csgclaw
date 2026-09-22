package dsh

import (
	"context"
	"csgclaw/internal/modelcap"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine/contract"
)

type permissionRequestParams struct {
	SessionID string `json:"sessionId"`
	ToolCall  struct {
		ToolCallID string `json:"toolCallId"`
		Title      string `json:"title"`
		Kind       string `json:"kind"`
	} `json:"toolCall"`
	Options []permissionOption `json:"options"`
}

type permissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

func (r *Runtime) handleNotification(proc *process, note notification) {
	if note.Method != "session/update" && note.Method != "csgclaw/context" {
		return
	}
	var params struct {
		SessionID string         `json:"sessionId"`
		Update    map[string]any `json:"update"`
	}
	if json.Unmarshal(note.Params, &params) != nil {
		return
	}
	proc.mu.Lock()
	turn := proc.active[params.SessionID]
	if turn == nil {
		proc.mu.Unlock()
		return
	}
	kind, _ := params.Update["sessionUpdate"].(string)
	event := contract.TurnEvent{TurnID: turn.request.ID}
	switch kind {
	case "agent_message_chunk", "agent_thought_chunk":
		content, _ := params.Update["content"].(map[string]any)
		contentType, _ := content["type"].(string)
		text, _ := content["text"].(string)
		if contentType != "text" || text == "" {
			proc.mu.Unlock()
			return
		}
		if kind == "agent_message_chunk" {
			event.Kind = contract.TurnEventTextDelta
			event.Text = text
			turn.output.WriteString(text)
		} else {
			event.Kind = contract.TurnEventThoughtDelta
			event.Thought = text
		}
	case "tool_call", "tool_call_update":
		id, _ := params.Update["toolCallId"].(string)
		if turn.tools == nil {
			turn.tools = make(map[string]contract.ToolActivity)
		}
		tool := mergeDSHToolActivity(turn.tools[id], params.Update)
		turn.tools[id] = tool
		if kind == "tool_call_update" {
			turn.recordPresentedFiles(tool)
		}
		event.Tool = &tool
		if kind == "tool_call" {
			event.Kind = contract.TurnEventToolCallStart
		} else {
			event.Kind = contract.TurnEventToolCallUpdate
		}
	case "context_error":
		turn.contextExceeded = true
		proc.mu.Unlock()
		return
	case "context_compaction":
		turn.compactionFailed, _ = params.Update["failed"].(bool)
		metadata := proc.profile.ModelMetadata.Normalized()
		usage, exists := proc.contextUsage[params.SessionID]
		if !exists {
			usage = modelcap.ContextUsage{SessionID: params.SessionID, ModelID: proc.profile.ModelID, ContextWindow: metadata.ContextWindow, ContextSource: metadata.ContextSource, AutoCompact: proc.profile.AutoCompact == nil || *proc.profile.AutoCompact, CompactThreshold: metadata.CompactThreshold(), Estimated: true}
		}
		usage.Compacting, _ = params.Update["compacting"].(bool)
		if !usage.Compacting {
			usage.UsedTokens = nil
			if used, ok := params.Update["used"].(float64); ok && used >= 0 && used < 1e12 {
				n := int64(used)
				usage.UsedTokens = &n
			}
		}
		usage.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if proc.contextUsage == nil {
			proc.contextUsage = make(map[string]modelcap.ContextUsage)
		}
		proc.contextUsage[params.SessionID] = usage
		event.Kind = contract.TurnEventActivityUpdate
		event.Activity = &contract.ActivityUpdate{ID: params.SessionID, Kind: modelcap.ContextUsageKind, Payload: usage}
	case "usage_update":
		metadata := proc.profile.ModelMetadata.Normalized()
		raw, _ := json.Marshal(params.Update)
		var values struct {
			Used *int64 `json:"used"`
			Size int64  `json:"size"`
		}
		if json.Unmarshal(raw, &values) != nil || values.Used == nil || *values.Used < 0 {
			proc.mu.Unlock()
			return
		}
		if values.Size <= 0 {
			values.Size = metadata.ContextWindow
		}
		usage := modelcap.ContextUsage{SessionID: params.SessionID, ModelID: proc.profile.ModelID, UsedTokens: values.Used, ContextWindow: values.Size, ContextSource: metadata.ContextSource, AutoCompact: proc.profile.AutoCompact == nil || *proc.profile.AutoCompact, CompactThreshold: metadata.CompactThreshold(), Estimated: true, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		if !usage.Valid() {
			proc.mu.Unlock()
			return
		}
		if proc.contextUsage == nil {
			proc.contextUsage = make(map[string]modelcap.ContextUsage)
		}
		proc.contextUsage[params.SessionID] = usage
		event.Kind = contract.TurnEventActivityUpdate
		event.Activity = &contract.ActivityUpdate{ID: params.SessionID, Kind: modelcap.ContextUsageKind, Status: "updated", Payload: usage}
	case "config_option_update":
		event.Kind = contract.TurnEventActivityUpdate
		event.Activity = &contract.ActivityUpdate{ID: kind, Kind: kind, Status: "updated", Payload: params.Update}
	default:
		proc.mu.Unlock()
		return
	}
	turn.seq++
	event.Sequence = turn.seq
	sink := turn.sink
	proc.mu.Unlock()
	if sink != nil {
		if err := sink.Emit(context.Background(), event); err != nil {
			proc.mu.Lock()
			if turn.interactionError == nil {
				turn.interactionError = &contract.TurnError{Code: contract.ErrorRuntimeFailed, Message: fmt.Sprintf("emit DSH turn event: %v", err)}
			}
			proc.mu.Unlock()
			_ = proc.client.notify("session/cancel", map[string]any{"sessionId": params.SessionID})
		}
	}
}

func (turn *activeTurn) recordPresentedFiles(tool contract.ToolActivity) {
	if !strings.EqualFold(strings.TrimSpace(tool.Status), "completed") ||
		!strings.EqualFold(strings.TrimSpace(tool.Kind), "present") {
		return
	}
	payload, ok := tool.Payload.(map[string]any)
	if !ok {
		return
	}
	rawInput, ok := payload["rawInput"].(map[string]any)
	if !ok {
		return
	}
	files, ok := rawInput["files"].([]any)
	if !ok {
		return
	}
	if turn.presentedPaths == nil {
		turn.presentedPaths = make(map[string]bool, len(files))
	}
	for _, item := range files {
		file, ok := item.(map[string]any)
		if !ok {
			continue
		}
		path, _ := file["path"].(string)
		path = strings.TrimSpace(path)
		if path == "" || turn.presentedPaths[path] {
			continue
		}
		turn.presentedPaths[path] = true
		turn.presentedFiles = append(turn.presentedFiles, presentedFile{Path: path})
	}
}

func mergeDSHToolActivity(previous contract.ToolActivity, update map[string]any) contract.ToolActivity {
	tool := previous
	mergeDSHString(&tool.ID, update["toolCallId"])
	mergeDSHString(&tool.Title, update["title"])
	mergeDSHString(&tool.Status, update["status"])

	var reportedKind string
	mergeDSHString(&reportedKind, update["kind"])
	if reportedKind != "" || tool.Kind == "" {
		if normalized := normalizedDSHToolKind(reportedKind, tool.Title); normalized != "" {
			tool.Kind = normalized
		}
	}
	if rawInput, ok := update["rawInput"]; ok {
		tool.InputSummary = summarizeDSHToolValue(rawInput)
	}
	if content, ok := update["content"]; ok {
		if output := dshToolOutputText(content); output != "" {
			tool.OutputSummary = summarizeDSHToolValue(map[string]any{"output": output})
		} else {
			tool.OutputSummary = summarizeDSHToolValue(content)
		}
	}
	tool.Payload = mergeDSHToolPayload(previous.Payload, update)
	return tool
}

func normalizedDSHToolKind(reported, title string) string {
	reported = strings.ToLower(strings.TrimSpace(reported))
	if reported != "" && reported != "other" {
		return reported
	}
	name := strings.ToLower(strings.TrimSpace(title))
	name = nonToolKindCharacter.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	switch name {
	case "bash", "command", "exec", "execute", "shell", "sh", "zsh":
		return "exec_command"
	case "":
		return reported
	default:
		return name
	}
}

func mergeDSHString(target *string, value any) {
	text, _ := value.(string)
	if text = strings.TrimSpace(text); text != "" {
		*target = text
	}
}

func mergeDSHToolPayload(previous any, update map[string]any) map[string]any {
	merged := make(map[string]any, len(update)+4)
	if prior, ok := previous.(map[string]any); ok {
		for key, value := range prior {
			merged[key] = value
		}
	}
	for key, value := range update {
		merged[key] = value
	}
	return merged
}

func summarizeDSHToolValue(value any) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(redactDSHToolValue(value))
	if err != nil {
		return ""
	}
	const limit = 240
	text := strings.TrimSpace(string(data))
	if len(text) <= limit {
		return text
	}
	return strings.TrimSpace(text[:limit]) + "..."
}

func redactDSHToolValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for key, item := range typed {
			lower := strings.ToLower(strings.TrimSpace(key))
			if strings.Contains(lower, "token") || strings.Contains(lower, "secret") ||
				strings.Contains(lower, "password") || strings.Contains(lower, "api_key") ||
				strings.Contains(lower, "apikey") {
				redacted[key] = "[redacted]"
				continue
			}
			redacted[key] = redactDSHToolValue(item)
		}
		return redacted
	case []any:
		redacted := make([]any, 0, len(typed))
		for _, item := range typed {
			redacted = append(redacted, redactDSHToolValue(item))
		}
		return redacted
	case string:
		return dshBearerTokenPattern.ReplaceAllString(
			dshAPIKeyPattern.ReplaceAllString(typed, "[redacted]"),
			"Bearer [redacted]",
		)
	default:
		return value
	}
}

func dshToolOutputText(value any) string {
	var values []string
	collectDSHToolOutputText(value, &values)
	return strings.Join(values, "\n")
}

func collectDSHToolOutputText(value any, values *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
			*values = append(*values, text)
			return
		}
		if content, ok := typed["content"]; ok {
			collectDSHToolOutputText(content, values)
		}
	case []any:
		for _, item := range typed {
			collectDSHToolOutputText(item, values)
		}
	}
}

var (
	nonToolKindCharacter  = regexp.MustCompile(`[^a-z0-9_-]+`)
	dshBearerTokenPattern = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-]+`)
	dshAPIKeyPattern      = regexp.MustCompile(`\bsk-[A-Za-z0-9._\-]+\b`)
)

func (r *Runtime) handleServerRequest(proc *process, request serverRequest) {
	if request.Method != "session/request_permission" {
		_ = proc.client.respond(request.ID, nil, &rpcError{Code: -32601, Message: "method not supported"})
		return
	}
	var params permissionRequestParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		_ = proc.client.respond(request.ID, nil, &rpcError{Code: -32602, Message: "invalid permission request"})
		return
	}
	proc.mu.Lock()
	turn := proc.active[params.SessionID]
	proc.mu.Unlock()
	if turn == nil {
		_ = proc.client.respond(request.ID, map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}, nil)
		return
	}
	if turn.request.Interaction != contract.InteractionResolve {
		if turn.request.Interaction == contract.InteractionSkipUserInput {
			if optionID, ok := unattendedLarkCLIPermission(proc, turn, params); ok {
				_ = proc.client.respond(request.ID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": optionID}}, nil)
				return
			}
			for _, option := range params.Options {
				if strings.Contains(strings.ToLower(option.Kind), "reject") {
					_ = proc.client.respond(request.ID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": option.OptionID}}, nil)
					return
				}
			}
		}
		if turn.request.Interaction == contract.InteractionReject {
			proc.mu.Lock()
			turn.interactionError = &contract.TurnError{Code: contract.ErrorInteractionUnsupported, Message: "Runtime interaction is not supported by this caller"}
			proc.mu.Unlock()
		}
		_ = proc.client.respond(request.ID, map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}, nil)
		if turn.request.Interaction == contract.InteractionReject {
			_ = proc.client.notify("session/cancel", map[string]any{"sessionId": params.SessionID})
		}
		return
	}
	r.mu.Lock()
	r.nextPerm++
	interactionID := fmt.Sprintf("dsh-permission-%d", r.nextPerm)
	options := make([]activity.ActionOptionSnapshot, 0, len(params.Options))
	allowed := make(map[string]bool, len(params.Options))
	for _, option := range params.Options {
		options = append(options, activity.ActionOptionSnapshot{ID: option.OptionID, Label: option.Name, Kind: option.Kind})
		allowed[option.OptionID] = true
	}
	title := strings.TrimSpace(params.ToolCall.Title)
	if title == "" {
		title = "Run tool"
	}
	snapshot := activity.ActivitySnapshot{
		ID: interactionID, Kind: activity.ActionKindPermission, Title: title,
		Status: activity.ActionStatusPending, RequestedAt: time.Now().UTC(), Options: options,
	}
	interaction := contract.InteractionRequest{
		ID: interactionID, Kind: contract.InteractionPermission, Title: title, Payload: snapshot,
	}
	r.pending[interactionID] = &pendingPermission{runtimeID: proc.meta.RuntimeID, conversation: turn.request.ConversationKey, request: interaction, requestID: request.ID, client: proc.client, allowedOptions: allowed}
	r.mu.Unlock()

	proc.mu.Lock()
	turn.seq++
	event := contract.TurnEvent{TurnID: turn.request.ID, Sequence: turn.seq, Kind: contract.TurnEventInteractionRequest, Interaction: &interaction}
	sink := turn.sink
	proc.mu.Unlock()
	if sink != nil {
		if err := sink.Emit(context.Background(), event); err != nil {
			r.mu.Lock()
			delete(r.pending, interactionID)
			r.mu.Unlock()
			proc.mu.Lock()
			if turn.interactionError == nil {
				turn.interactionError = &contract.TurnError{Code: contract.ErrorRuntimeFailed, Message: fmt.Sprintf("emit DSH permission request: %v", err)}
			}
			proc.mu.Unlock()
			_ = proc.client.respond(request.ID, map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}, nil)
			_ = proc.client.notify("session/cancel", map[string]any{"sessionId": params.SessionID})
		}
	}
}
