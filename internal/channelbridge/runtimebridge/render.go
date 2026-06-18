package runtimebridge

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"csgclaw/internal/activity"
)

const (
	AgentActivityVersion = 1
	AgentActivityType    = "com.opencsg.csgclaw.agent.activity"
	AgentToolMsgType     = "com.opencsg.csgclaw.agent.tool"
	AgentActionMsgType   = "com.opencsg.csgclaw.agent.action"
)

type TurnRenderer struct {
	text           strings.Builder
	toolSnapshots  map[string]activityTool
	toolSignatures map[string]string
	promptError    string
}

func NewTurnRenderer() *TurnRenderer {
	return &TurnRenderer{
		toolSnapshots:  make(map[string]activityTool),
		toolSignatures: make(map[string]string),
	}
}

func (r *TurnRenderer) ApplyText(event activity.RuntimeEvent) {
	if r == nil {
		return
	}

	switch event.Kind {
	case activity.RuntimeEventTextDelta:
		if event.Text != "" {
			_, _ = r.text.WriteString(event.Text)
		}
	case activity.RuntimeEventPromptFailed:
		r.promptError = strings.TrimSpace(event.Error)
	}
}

func (r *TurnRenderer) FinalMessages() []string {
	if r == nil {
		return nil
	}
	var messages []string
	if text := strings.TrimSpace(r.text.String()); text != "" {
		messages = append(messages, text)
	}
	if r.promptError != "" {
		messages = append(messages, fmt.Sprintf("Runtime error: %s", r.promptError))
	}
	return messages
}

func (r *TurnRenderer) SetPromptError(err string) {
	if r != nil {
		r.promptError = strings.TrimSpace(err)
	}
}

func (r *TurnRenderer) RenderActivity(event activity.RuntimeEvent, channel, roomID, senderID string) (RenderedActivity, bool) {
	if r == nil {
		return RenderedActivity{}, false
	}
	switch event.Kind {
	case activity.RuntimeEventToolCallStart:
		tool, changed := r.mergeToolSnapshot(event)
		if !changed {
			return RenderedActivity{}, false
		}
		return renderActivityPayload(event, channel, roomID, senderID, toolActivityContent(event, tool))
	case activity.RuntimeEventToolCallUpdate:
		tool, changed := r.mergeToolSnapshot(event)
		if !changed {
			return RenderedActivity{}, false
		}
		return renderActivityPayload(event, channel, roomID, senderID, toolActivityContent(event, tool))
	case activity.RuntimeEventActionRequest, activity.RuntimeEventActionDecision:
		snapshot, ok := event.Payload.(activity.ActivitySnapshot)
		if !ok {
			return RenderedActivity{}, false
		}
		if event.ActionID == "" {
			event.ActionID = snapshot.ID
		}
		return renderActivityPayload(event, channel, roomID, senderID, actionActivityContent(event, snapshot))
	default:
		return RenderedActivity{}, false
	}
}

func (r *TurnRenderer) mergeToolSnapshot(event activity.RuntimeEvent) (activityTool, bool) {
	toolID := strings.TrimSpace(event.ToolCallID)
	if toolID == "" {
		return activityTool{}, false
	}

	tool := r.toolSnapshots[toolID]
	tool.ID = publicToolActivityID(event)
	mergeString(&tool.Kind, event.ToolKind)
	mergeString(&tool.Title, event.ToolTitle)
	mergeString(&tool.InputSummary, event.ToolInputSummary)
	mergeString(&tool.OutputSummary, event.ToolOutputSummary)
	if strings.TrimSpace(event.ToolStatus) != "" {
		tool.Status = normalizedToolStatus(event.ToolStatus)
	}
	if tool.Title == "" {
		tool.Title = "Run tool"
	}
	if tool.Status == "" {
		tool.Status = "running"
	}

	signature := toolSignature(tool)
	if r.toolSignatures[toolID] == signature {
		return tool, false
	}
	r.toolSnapshots[toolID] = tool
	r.toolSignatures[toolID] = signature
	return tool, true
}

func displayToolTitle(event activity.RuntimeEvent) string {
	title := strings.TrimSpace(event.ToolTitle)
	if title == "" {
		title = "Run tool"
	}
	return title
}

func normalizedToolStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "in_progress":
		return "running"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

type RenderedActivity struct {
	MessageID string
	Text      string
}

func renderActivityPayload(event activity.RuntimeEvent, channel, roomID, senderID string, content any) (RenderedActivity, bool) {
	eventID := activityEventID(event)
	originServerTS := time.Now().UTC().UnixMilli()
	if !event.ReceivedAt.IsZero() {
		originServerTS = event.ReceivedAt.UnixMilli()
	}
	payload := agentActivityPayload{
		Type:           AgentActivityType,
		Version:        AgentActivityVersion,
		Channel:        strings.TrimSpace(channel),
		EventID:        eventID,
		RoomID:         strings.TrimSpace(roomID),
		Sender:         strings.TrimSpace(senderID),
		OriginServerTS: originServerTS,
		Content:        content,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return RenderedActivity{}, false
	}
	return RenderedActivity{MessageID: eventID, Text: string(data)}, true
}

type agentActivityPayload struct {
	Type           string `json:"type"`
	Version        int    `json:"version"`
	Channel        string `json:"channel,omitempty"`
	EventID        string `json:"event_id"`
	RoomID         string `json:"room_id"`
	Sender         string `json:"sender"`
	OriginServerTS int64  `json:"origin_server_ts"`
	Content        any    `json:"content"`
}

type toolActivity struct {
	MsgType string       `json:"msgtype"`
	Body    string       `json:"body"`
	Tool    activityTool `json:"tool"`
}

type activityTool struct {
	ID            string `json:"id"`
	Kind          string `json:"kind,omitempty"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	InputSummary  string `json:"input_summary,omitempty"`
	OutputSummary string `json:"output_summary,omitempty"`
}

type actionActivity struct {
	MsgType string         `json:"msgtype"`
	Body    string         `json:"body"`
	Action  activityAction `json:"action"`
}

type activityAction struct {
	ID          string                           `json:"id"`
	Kind        string                           `json:"kind"`
	Title       string                           `json:"title"`
	Status      string                           `json:"status"`
	RequestedAt string                           `json:"requested_at,omitempty"`
	ExpiresAt   string                           `json:"expires_at,omitempty"`
	Options     []activity.ActionOptionSnapshot  `json:"options,omitempty"`
	Decision    *activity.ActionDecisionSnapshot `json:"decision,omitempty"`
}

func toolActivityContent(event activity.RuntimeEvent, tool activityTool) toolActivity {
	if tool.Status == "" {
		tool.Status = "running"
	}
	if tool.Title == "" {
		tool.Title = displayToolTitle(event)
	}
	return toolActivity{
		MsgType: AgentToolMsgType,
		Body:    fmt.Sprintf("Tool %s: %s", tool.Status, tool.Title),
		Tool:    tool,
	}
}

func actionActivityContent(event activity.RuntimeEvent, snapshot activity.ActivitySnapshot) actionActivity {
	status := string(snapshot.Status)
	bodyStatus := "Permission required"
	switch snapshot.Status {
	case activity.ActionStatusAllowed:
		bodyStatus = "Permission allowed"
	case activity.ActionStatusRejected:
		bodyStatus = "Permission rejected"
	case activity.ActionStatusExpired:
		bodyStatus = "Permission expired"
	case activity.ActionStatusCanceled:
		bodyStatus = "Permission canceled"
	}
	return actionActivity{
		MsgType: AgentActionMsgType,
		Body:    fmt.Sprintf("%s: %s", bodyStatus, displayToolTitle(event)),
		Action: activityAction{
			ID:          strings.TrimSpace(snapshot.ID),
			Kind:        firstActivityText(snapshot.Kind, activity.ActionKindPermission),
			Title:       firstActivityText(snapshot.Title, displayToolTitle(event)),
			Status:      status,
			RequestedAt: formatActivityTime(snapshot.RequestedAt),
			ExpiresAt:   formatActivityTime(snapshot.ExpiresAt),
			Options:     snapshot.Options,
			Decision:    snapshot.Decision,
		},
	}
}

func mergeString(target *string, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		*target = value
	}
}

func toolSignature(tool activityTool) string {
	return strings.Join([]string{
		tool.ID,
		tool.Kind,
		tool.Title,
		tool.Status,
		tool.InputSummary,
		tool.OutputSummary,
	}, "\x00")
}

func activityEventID(event activity.RuntimeEvent) string {
	if event.ActionID != "" {
		return joinActivityIDParts([]string{"act", strings.TrimSpace(event.ActionID)})
	}
	if event.ToolCallID != "" {
		return joinActivityIDParts([]string{"tool", publicToolActivityID(event)})
	}
	parts := []string{"evt", string(event.Kind)}
	if event.MessageID != "" {
		parts = append(parts, event.MessageID)
	}
	if !event.ReceivedAt.IsZero() {
		parts = append(parts, fmt.Sprintf("%d", event.ReceivedAt.UnixNano()))
	}
	return joinActivityIDParts(parts)
}

func joinActivityIDParts(parts []string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, "-")
}

func publicToolActivityID(event activity.RuntimeEvent) string {
	return opaqueActivityIDPart(event.RuntimeKind, event.RuntimeID, event.SessionID, event.ToolCallID)
}

func opaqueActivityIDPart(parts ...string) string {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized = append(normalized, strings.TrimSpace(part))
	}
	sum := sha256.Sum256([]byte(strings.Join(normalized, "\x00")))
	return fmt.Sprintf("%x", sum[:12])
}

func formatActivityTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func firstActivityText(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
