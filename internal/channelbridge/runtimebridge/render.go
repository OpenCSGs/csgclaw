package runtimebridge

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"csgclaw/internal/activity"
)

const (
	AgentActivityVersion = 1
	AgentActivityType    = "com.opencsg.csgclaw.agent.activity"
	AgentToolMsgType     = "com.opencsg.csgclaw.agent.tool"
	AgentActionMsgType   = "com.opencsg.csgclaw.agent.action"
	AgentQuestionMsgType = "com.opencsg.csgclaw.agent.question"
	CSGClawMetadataKey   = "csgclaw"
	AgentActivityMetaKey = "agent_activity"
)

type TurnRenderer struct {
	text                      strings.Builder
	toolSnapshots             map[string]activityTool
	toolSignatures            map[string]string
	promptError               string
	userInput                 *activity.RequestUserInputArgs
	userInputFallback         string
	userInputFallbackExplicit bool
	resourceLinks             []activity.ResourceLink
	resourceURIs              map[string]struct{}
}

func NewTurnRenderer() *TurnRenderer {
	return &TurnRenderer{
		toolSnapshots:  make(map[string]activityTool),
		toolSignatures: make(map[string]string),
		resourceURIs:   make(map[string]struct{}),
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
	if r.userInputFallbackExplicit && strings.TrimSpace(r.userInputFallback) != "" {
		messages = append(messages, appendResourceLinks(strings.TrimSpace(r.userInputFallback), r.resourceLinks))
	} else if text := strings.TrimSpace(r.text.String()); text != "" {
		messages = append(messages, appendResourceLinks(text, r.resourceLinks))
	} else if fallback := strings.TrimSpace(r.userInputFallback); fallback != "" {
		messages = append(messages, appendResourceLinks(fallback, r.resourceLinks))
	} else if links := resourceLinksMarkdown(r.resourceLinks); links != "" {
		messages = append(messages, links)
	}
	if r.promptError != "" {
		messages = append(messages, fmt.Sprintf("Runtime error: %s", r.promptError))
	}
	return messages
}

func (r *TurnRenderer) ApplyStructuredOutput(event activity.RuntimeEvent) {
	if r == nil || event.Kind != activity.RuntimeEventStructuredOutput {
		return
	}
	artifact, ok := event.Payload.(activity.StructuredOutputArtifact)
	if !ok {
		return
	}
	if r.userInput == nil && artifact.RequestUserInput != nil {
		copy := *artifact.RequestUserInput
		copy.Questions = append([]activity.RequestUserInputQuestion(nil), artifact.RequestUserInput.Questions...)
		for index := range copy.Questions {
			copy.Questions[index].Options = append([]activity.RequestUserInputOption(nil), artifact.RequestUserInput.Questions[index].Options...)
		}
		r.userInput = &copy
		r.userInputFallback = strings.TrimSpace(event.Text)
		r.userInputFallbackExplicit = r.userInputFallback != ""
		if r.userInputFallback == "" {
			r.userInputFallback = "Please answer the questions below."
		}
	}
	for _, link := range artifact.ResourceLinks {
		if len(r.resourceLinks) >= 16 {
			break
		}
		if _, exists := r.resourceURIs[link.URI]; exists {
			continue
		}
		r.resourceURIs[link.URI] = struct{}{}
		r.resourceLinks = append(r.resourceLinks, link)
	}
}

func (r *TurnRenderer) RequestUserInput() *activity.RequestUserInputArgs {
	if r == nil || r.userInput == nil {
		return nil
	}
	copy := *r.userInput
	copy.Questions = append([]activity.RequestUserInputQuestion(nil), r.userInput.Questions...)
	for index := range copy.Questions {
		copy.Questions[index].Options = append([]activity.RequestUserInputOption(nil), r.userInput.Questions[index].Options...)
	}
	return &copy
}

func (r *TurnRenderer) DiscardStructuredOutput() {
	if r == nil {
		return
	}
	r.userInput = nil
	r.userInputFallback = ""
	r.userInputFallbackExplicit = false
	r.resourceLinks = nil
	clear(r.resourceURIs)
}

func appendResourceLinks(text string, links []activity.ResourceLink) string {
	markdown := resourceLinksMarkdown(links)
	if markdown == "" {
		return text
	}
	return strings.TrimSpace(text) + "\n\n" + markdown
}

func resourceLinksMarkdown(links []activity.ResourceLink) string {
	if len(links) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Links\n")
	for _, link := range links {
		label := strings.TrimSpace(link.Title)
		if label == "" {
			label = strings.TrimSpace(link.Name)
		}
		label = strings.NewReplacer("[", "\\[", "]", "\\]").Replace(label)
		b.WriteString("- ")
		if iconURI := resourceLinkIconURI(link); iconURI != "" {
			b.WriteString(`<img class="resource-link-icon" src="`)
			b.WriteString(html.EscapeString(iconURI))
			b.WriteString(`" alt="" aria-hidden="true"> `)
		}
		b.WriteString("[")
		b.WriteString(label)
		b.WriteString("](<")
		b.WriteString(link.URI)
		b.WriteString(">)")
		if description := strings.TrimSpace(link.Description); description != "" {
			b.WriteString(" - ")
			b.WriteString(strings.ReplaceAll(description, "\n", " "))
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func resourceLinkIconURI(link activity.ResourceLink) string {
	for _, icon := range link.Icons {
		raw, ok := icon["src"].(string)
		if !ok {
			continue
		}
		iconURI := strings.TrimSpace(raw)
		parsed, err := url.Parse(iconURI)
		if err != nil || parsed.Host == "" {
			continue
		}
		if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
			continue
		}
		return iconURI
	}
	return ""
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
	case activity.RuntimeEventUserInputRequest, activity.RuntimeEventUserInputResolved:
		snapshot, ok := event.Payload.(activity.UserInputSnapshot)
		if !ok {
			return RenderedActivity{}, false
		}
		if event.UserInputID == "" {
			event.UserInputID = snapshot.ID
		}
		return renderQuestionActivity(event, channel, roomID, senderID, snapshot)
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
	Metadata  map[string]any
}

func renderActivityPayload(event activity.RuntimeEvent, channel, roomID, senderID string, content any) (RenderedActivity, bool) {
	payload := newAgentActivityPayload(event, channel, roomID, senderID, content)
	data, err := json.Marshal(payload)
	if err != nil {
		return RenderedActivity{}, false
	}
	return RenderedActivity{MessageID: payload.EventID, Text: string(data)}, true
}

func renderQuestionActivity(event activity.RuntimeEvent, channel, roomID, senderID string, snapshot activity.UserInputSnapshot) (RenderedActivity, bool) {
	payload := newAgentActivityPayload(event, channel, roomID, senderID, questionActivityContent(snapshot))
	text := activity.UserInputQuestionMarkdown(snapshot)
	if text == "" {
		return RenderedActivity{}, false
	}
	return RenderedActivity{
		MessageID: payload.EventID,
		Text:      text,
		Metadata: map[string]any{
			CSGClawMetadataKey: map[string]any{AgentActivityMetaKey: payload},
		},
	}, true
}

func newAgentActivityPayload(event activity.RuntimeEvent, channel, roomID, senderID string, content any) agentActivityPayload {
	originServerTS := time.Now().UTC().UnixMilli()
	if !event.ReceivedAt.IsZero() {
		originServerTS = event.ReceivedAt.UnixMilli()
	}
	return agentActivityPayload{
		Type:           AgentActivityType,
		Version:        AgentActivityVersion,
		Channel:        strings.TrimSpace(channel),
		EventID:        activityEventID(event),
		RoomID:         strings.TrimSpace(roomID),
		Sender:         strings.TrimSpace(senderID),
		OriginServerTS: originServerTS,
		Content:        content,
	}
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

type questionActivity struct {
	MsgType  string                     `json:"msgtype"`
	Body     string                     `json:"body"`
	Question activity.UserInputSnapshot `json:"question"`
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

func questionActivityContent(snapshot activity.UserInputSnapshot) questionActivity {
	body := "Question pending"
	switch snapshot.Status {
	case activity.UserInputStatusAnswered:
		body = "Question answered"
	case activity.UserInputStatusSkipped:
		body = "Question skipped"
	case activity.UserInputStatusExpired:
		body = "Question expired"
	case activity.UserInputStatusCanceled:
		body = "Question canceled"
	case activity.UserInputStatusInterrupted:
		body = "Question interrupted"
	}
	return questionActivity{
		MsgType:  AgentQuestionMsgType,
		Body:     body,
		Question: snapshot,
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
	if event.UserInputID != "" {
		return joinActivityIDParts([]string{"question", strings.TrimSpace(event.UserInputID)})
	}
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

// InterruptPendingQuestionActivity converts a persisted question that can no
// longer have a live app-server request into a terminal interrupted snapshot.
func InterruptPendingQuestionActivity(content string, metadata map[string]any, now time.Time) (string, map[string]any, bool) {
	value, ok := agentActivityMetadata(metadata)
	if !ok {
		var legacy any
		if json.Unmarshal([]byte(strings.TrimSpace(content)), &legacy) != nil {
			return content, metadata, false
		}
		value = legacy
	}
	data, err := json.Marshal(value)
	if err != nil {
		return content, metadata, false
	}
	var payload struct {
		Type           string           `json:"type"`
		Version        int              `json:"version"`
		Channel        string           `json:"channel,omitempty"`
		EventID        string           `json:"event_id"`
		RoomID         string           `json:"room_id"`
		Sender         string           `json:"sender"`
		OriginServerTS int64            `json:"origin_server_ts"`
		Content        questionActivity `json:"content"`
	}
	if json.Unmarshal(data, &payload) != nil || payload.Type != AgentActivityType || payload.Content.MsgType != AgentQuestionMsgType || payload.Content.Question.Status != activity.UserInputStatusPending {
		return content, metadata, false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	payload.Content.Question.Status = activity.UserInputStatusInterrupted
	resolvedAt := now.UTC()
	payload.Content.Question.ResolvedAt = &resolvedAt
	payload.Content.Question.Answers = nil
	payload.Content = questionActivityContent(payload.Content.Question)
	updated := cloneMetadata(metadata)
	namespace := metadataNamespace(updated)
	namespace[AgentActivityMetaKey] = payload
	return activity.UserInputQuestionMarkdown(payload.Content.Question), updated, true
}

func agentActivityMetadata(metadata map[string]any) (any, bool) {
	if metadata == nil {
		return nil, false
	}
	namespace, ok := metadata[CSGClawMetadataKey].(map[string]any)
	if !ok {
		return nil, false
	}
	value, ok := namespace[AgentActivityMetaKey]
	return value, ok && value != nil
}

func cloneMetadata(metadata map[string]any) map[string]any {
	out := make(map[string]any, len(metadata)+1)
	for key, value := range metadata {
		out[key] = value
	}
	if namespace, ok := metadata[CSGClawMetadataKey].(map[string]any); ok {
		cloned := make(map[string]any, len(namespace)+1)
		for key, value := range namespace {
			cloned[key] = value
		}
		out[CSGClawMetadataKey] = cloned
	}
	return out
}

func metadataNamespace(metadata map[string]any) map[string]any {
	if namespace, ok := metadata[CSGClawMetadataKey].(map[string]any); ok {
		return namespace
	}
	namespace := make(map[string]any)
	metadata[CSGClawMetadataKey] = namespace
	return namespace
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
