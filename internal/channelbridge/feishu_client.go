package channelbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/im"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkdispatcher "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

var mentionPlaceholder = regexp.MustCompile(`@_user_\d+`)
var atMentionRegexp = regexp.MustCompile(`(?i)<at\s+user_id="([^"]+)"`)

const defaultBridgeReconnectWait = 500 * time.Millisecond
const bridgeThreadRelationType = "m.thread"

// FeishuClient adapts Feishu events into BotEvent and sends responses to Feishu.
type FeishuClient struct {
	Svc           *feishu.Service
	MentionOnly   bool
	ReconnectWait time.Duration
}

func NewFeishuClient(svc *feishu.Service) *FeishuClient {
	return &FeishuClient{
		Svc:           svc,
		MentionOnly:   true,
		ReconnectWait: defaultBridgeReconnectWait,
	}
}

func (c *FeishuClient) StreamEvents(ctx context.Context, botID, lastEventID string) (<-chan BotEvent, <-chan error) {
	events := make(chan BotEvent, 16)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		botID = strings.TrimSpace(botID)
		if botID == "" {
			errs <- fmt.Errorf("feishu bridge stream: bot id is required")
			return
		}
		if c == nil || c.Svc == nil {
			errs <- fmt.Errorf("feishu bridge stream: service is required")
			return
		}
		if ctx == nil {
			ctx = context.Background()
		}
		reconnectWait := c.ReconnectWait
		if reconnectWait <= 0 {
			reconnectWait = defaultBridgeReconnectWait
		}
		lastEventID = strings.TrimSpace(lastEventID)
		var lastEventIDMu sync.Mutex
		requireBotMention := c.MentionOnly

		provider := c.Svc.ConfigProvider()
		if provider == nil {
			errs <- fmt.Errorf("feishu bridge stream: feishu provider not configured")
			return
		}
		app, ok := provider.BotConfig(botID)
		if !ok {
			errs <- fmt.Errorf("feishu bridge stream: bot config not found for %q", botID)
			return
		}

		botOpenID := ""
		if c.MentionOnly {
			if value, _, err := c.Svc.ResolveBotOpenID(ctx, botID); err == nil {
				botOpenID = strings.TrimSpace(value)
				if botOpenID == "" {
					errs <- fmt.Errorf("feishu bridge stream: resolve bot open_id returned empty value for %q", botID)
					return
				}
			} else {
				errs <- fmt.Errorf("feishu bridge stream: resolve bot open_id for %q: %w", botID, err)
				return
			}
		}

		for {
			if ctx.Err() != nil {
				return
			}

			slog.Debug("feishu bridge stream starting", "participant_id", botID)
			dispatcher := larkdispatcher.NewEventDispatcher("", "").
				OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
					c.emitBotEvent(ctx, botID, botOpenID, requireBotMention, event, events, &lastEventID, &lastEventIDMu)
					return nil
				})
			wsClient := larkws.NewClient(
				app.AppID,
				app.AppSecret,
				larkws.WithEventHandler(dispatcher),
				larkws.WithDomain(lark.FeishuBaseUrl),
			)

			err := wsClient.Start(ctx)
			if err != nil && ctx.Err() == nil {
				slog.Warn("feishu bridge stream failed", "participant_id", botID, "error", err)
				select {
				case errs <- fmt.Errorf("feishu bridge stream %q: %w", botID, err):
				default:
				}
			}
			if ctx.Err() != nil {
				return
			}
			if err == nil {
				return
			}
			select {
			case <-time.After(reconnectWait):
			case <-ctx.Done():
				return
			}
		}
	}()

	return events, errs
}

func (c *FeishuClient) SendMessage(ctx context.Context, botID string, req SendMessageRequest) (SendMessageResponse, error) {
	if c == nil || c.Svc == nil {
		return SendMessageResponse{}, fmt.Errorf("feishu bridge send: service is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	senderID := strings.TrimSpace(botID)
	if senderID == "" {
		return SendMessageResponse{}, fmt.Errorf("feishu bridge send: bot id is required")
	}
	roomID := strings.TrimSpace(req.RoomID)
	if roomID == "" {
		return SendMessageResponse{}, fmt.Errorf("feishu bridge send: room id is required")
	}

	text := strings.TrimSpace(req.Text)
	if text == "" {
		return SendMessageResponse{}, nil
	}

	var relatesTo *im.MessageRelation
	if threadRootID := strings.TrimSpace(req.ThreadRootID); threadRootID != "" {
		relatesTo = &im.MessageRelation{
			RelType: bridgeThreadRelationType,
			EventID: threadRootID,
		}
	}

	mode := "create"
	if relatesTo != nil {
		mode = "reply"
	}
	sendStartedAt := time.Now()
	message, err := c.Svc.SendMessage(im.CreateMessageRequest{
		RoomID:    roomID,
		SenderID:  senderID,
		Content:   text,
		RelatesTo: relatesTo,
	})
	if err != nil {
		slog.Warn("feishu bridge send failed",
			"participant_id", senderID,
			"room_id", roomID,
			"thread_root_id", strings.TrimSpace(req.ThreadRootID),
			"mode", mode,
			"text_bytes", len(text),
			"duration", time.Since(sendStartedAt),
			"error", err,
		)
		return SendMessageResponse{}, err
	}
	slog.Debug("feishu bridge send completed",
		"participant_id", senderID,
		"room_id", roomID,
		"sent_message_id", strings.TrimSpace(message.ID),
		"thread_root_id", strings.TrimSpace(req.ThreadRootID),
		"mode", mode,
		"text_bytes", len(text),
		"duration", time.Since(sendStartedAt),
	)
	return SendMessageResponse{MessageID: strings.TrimSpace(message.ID)}, nil
}

func (c *FeishuClient) UpdateMessage(ctx context.Context, botID string, req UpdateMessageRequest) (UpdateMessageResponse, error) {
	if c == nil || c.Svc == nil {
		return UpdateMessageResponse{}, fmt.Errorf("feishu bridge update: service is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	senderID := strings.TrimSpace(botID)
	if senderID == "" {
		return UpdateMessageResponse{}, fmt.Errorf("feishu bridge update: bot id is required")
	}
	roomID := strings.TrimSpace(req.RoomID)
	if roomID == "" {
		return UpdateMessageResponse{}, fmt.Errorf("feishu bridge update: room id is required")
	}
	messageID := strings.TrimSpace(req.MessageID)
	if messageID == "" {
		return UpdateMessageResponse{}, fmt.Errorf("feishu bridge update: message id is required")
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return UpdateMessageResponse{}, nil
	}

	updateStartedAt := time.Now()
	message, err := c.Svc.UpdateMessageWithContext(ctx, feishu.UpdateMessageRequest{
		RoomID:    roomID,
		SenderID:  senderID,
		MessageID: messageID,
		Content:   text,
	})
	if err != nil {
		slog.Warn("feishu bridge update failed",
			"participant_id", senderID,
			"room_id", roomID,
			"message_id", messageID,
			"text_bytes", len(text),
			"duration", time.Since(updateStartedAt),
			"error", err,
		)
		return UpdateMessageResponse{}, err
	}
	slog.Debug("feishu bridge update completed",
		"participant_id", senderID,
		"room_id", roomID,
		"message_id", messageID,
		"updated_message_id", strings.TrimSpace(message.ID),
		"text_bytes", len(text),
		"duration", time.Since(updateStartedAt),
	)
	return UpdateMessageResponse{MessageID: strings.TrimSpace(message.ID)}, nil
}

func (c *FeishuClient) AddMessageReaction(ctx context.Context, botID string, req AddMessageReactionRequest) (AddMessageReactionResponse, error) {
	if c == nil || c.Svc == nil {
		return AddMessageReactionResponse{}, fmt.Errorf("feishu bridge reaction: service is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	senderID := strings.TrimSpace(botID)
	if senderID == "" {
		return AddMessageReactionResponse{}, fmt.Errorf("feishu bridge reaction: bot id is required")
	}
	messageID := strings.TrimSpace(req.MessageID)
	if messageID == "" {
		return AddMessageReactionResponse{}, fmt.Errorf("feishu bridge reaction: message id is required")
	}
	emojiType := strings.TrimSpace(req.EmojiType)
	if emojiType == "" {
		return AddMessageReactionResponse{}, fmt.Errorf("feishu bridge reaction: emoji type is required")
	}

	reactionStartedAt := time.Now()
	reaction, err := c.Svc.CreateMessageReactionWithContext(ctx, feishu.CreateMessageReactionRequest{
		SenderID:  senderID,
		MessageID: messageID,
		EmojiType: emojiType,
	})
	if err != nil {
		slog.Warn("feishu bridge add reaction failed",
			"participant_id", senderID,
			"message_id", messageID,
			"emoji_type", emojiType,
			"duration", time.Since(reactionStartedAt),
			"error", err,
		)
		return AddMessageReactionResponse{}, err
	}
	slog.Debug("feishu bridge add reaction completed",
		"participant_id", senderID,
		"message_id", messageID,
		"reaction_id", strings.TrimSpace(reaction.ReactionID),
		"emoji_type", emojiType,
		"duration", time.Since(reactionStartedAt),
	)
	return AddMessageReactionResponse{ReactionID: strings.TrimSpace(reaction.ReactionID)}, nil
}

func (c *FeishuClient) DeleteMessageReaction(ctx context.Context, botID string, req DeleteMessageReactionRequest) error {
	if c == nil || c.Svc == nil {
		return fmt.Errorf("feishu bridge reaction delete: service is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	senderID := strings.TrimSpace(botID)
	if senderID == "" {
		return fmt.Errorf("feishu bridge reaction delete: bot id is required")
	}
	messageID := strings.TrimSpace(req.MessageID)
	if messageID == "" {
		return fmt.Errorf("feishu bridge reaction delete: message id is required")
	}
	reactionID := strings.TrimSpace(req.ReactionID)
	if reactionID == "" {
		return fmt.Errorf("feishu bridge reaction delete: reaction id is required")
	}

	reactionStartedAt := time.Now()
	err := c.Svc.DeleteMessageReactionWithContext(ctx, feishu.DeleteMessageReactionRequest{
		SenderID:   senderID,
		MessageID:  messageID,
		ReactionID: reactionID,
	})
	if err != nil {
		slog.Warn("feishu bridge delete reaction failed",
			"participant_id", senderID,
			"message_id", messageID,
			"reaction_id", reactionID,
			"duration", time.Since(reactionStartedAt),
			"error", err,
		)
		return err
	}
	slog.Debug("feishu bridge delete reaction completed",
		"participant_id", senderID,
		"message_id", messageID,
		"reaction_id", reactionID,
		"duration", time.Since(reactionStartedAt),
	)
	return nil
}

func (c *FeishuClient) emitBotEvent(
	ctx context.Context,
	participantID string,
	botOpenID string,
	requireBotMention bool,
	event *larkim.P2MessageReceiveV1,
	out chan<- BotEvent,
	lastEventID *string,
	lastEventIDMu *sync.Mutex,
) {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return
	}
	message := event.Event.Message
	roomID := strings.TrimSpace(stringValue(message.ChatId))
	if roomID == "" {
		return
	}
	messageID := strings.TrimSpace(stringValue(message.MessageId))
	if messageID == "" {
		return
	}
	if !updateLastEventID(messageID, lastEventID, lastEventIDMu) {
		return
	}
	messageType := strings.TrimSpace(stringValue(message.MessageType))
	if messageType == "" {
		messageType = larkim.MsgTypeText
	}
	text := normalizeFeishuText(messageType, stringValue(message.Content))
	if text == "" {
		return
	}

	senderOpenID := ""
	if event.Event.Sender != nil && event.Event.Sender.SenderId != nil {
		senderOpenID = strings.TrimSpace(stringValue(event.Event.Sender.SenderId.OpenId))
	}
	if botOpenID != "" && senderOpenID == botOpenID {
		return
	}

	chatType := strings.TrimSpace(stringValue(message.ChatType))
	mentions := eventMentions(message.Mentions)
	if strings.EqualFold(chatType, "group") && c.MentionOnly {
		if requireBotMention {
			if botOpenID == "" || !hasInboundBotMention(text, botOpenID, mentions, message.Mentions) {
				return
			}
		} else if !hasAnyMention(mentions, text) {
			return
		}
		if hasAnyMention(mentions, text) {
			text = stripMentionPlaceholders(text, message.Mentions)
		}
	}

	threadRootID := firstNonEmpty(
		stringValue(message.RootId),
		stringValue(message.ParentId),
		stringValue(message.ThreadId),
	)
	threadRootID = strings.TrimSpace(threadRootID)
	botEvent := BotEvent{
		Channel:       "feishu",
		ParticipantID: strings.TrimSpace(participantID),
		MessageID:     messageID,
		RoomID:        roomID,
		ChatType:      chatType,
		Text:          text,
		Mentions:      mentions,
		ThreadRootID:  threadRootID,
	}
	if botEvent.RoomID == "" {
		return
	}
	slog.Debug("feishu bridge inbound message",
		"participant_id", strings.TrimSpace(participantID),
		"room_id", botEvent.RoomID,
		"message_id", botEvent.MessageID,
		"thread_root_id", botEvent.ThreadRootID,
		"chat_type", botEvent.ChatType,
		"text_bytes", len(botEvent.Text),
		"mentions", len(botEvent.Mentions),
	)
	select {
	case <-ctx.Done():
	case out <- botEvent:
	}
}

func updateLastEventID(messageID string, lastEventID *string, lastEventIDMu *sync.Mutex) bool {
	if strings.TrimSpace(messageID) == "" {
		return false
	}
	if lastEventID == nil || lastEventIDMu == nil {
		return true
	}
	lastEventIDMu.Lock()
	defer lastEventIDMu.Unlock()
	current := strings.TrimSpace(*lastEventID)
	if current != "" && messageID == current {
		return false
	}
	*lastEventID = messageID
	return true
}

func hasAnyMention(mentions []string, text string) bool {
	if len(mentions) > 0 {
		return true
	}
	return atMentionRegexp.MatchString(text)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func normalizeFeishuText(messageType, rawContent string) string {
	rawContent = strings.TrimSpace(rawContent)
	if rawContent == "" {
		return ""
	}
	const (
		textType        = larkim.MsgTypeText
		postType        = larkim.MsgTypePost
		interactiveType = larkim.MsgTypeInteractive
		imageType       = larkim.MsgTypeImage
		fileType        = larkim.MsgTypeFile
		audioType       = larkim.MsgTypeAudio
		mediaType       = larkim.MsgTypeMedia
	)

	switch messageType {
	case textType:
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(rawContent), &payload); err == nil {
			if payload.Text != "" {
				return strings.TrimSpace(payload.Text)
			}
		}
		return rawContent
	case postType:
		return extractPostText(rawContent)
	case interactiveType:
		return rawContent
	case imageType, fileType, audioType, mediaType:
		return ""
	default:
		return rawContent
	}
}

func eventMentions(mentions []*larkim.MentionEvent) []string {
	if len(mentions) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		if mention == nil {
			continue
		}
		if mention.Id != nil && mention.Id.OpenId != nil {
			openID := strings.TrimSpace(stringValue(mention.Id.OpenId))
			if openID != "" {
				if _, ok := seen[openID]; !ok {
					seen[openID] = struct{}{}
					out = append(out, openID)
				}
			}
		}
		if mention.Name != nil {
			name := strings.TrimSpace(stringValue(mention.Name))
			if name != "" {
				if _, ok := seen[name]; !ok {
					seen[name] = struct{}{}
					out = append(out, name)
				}
			}
		}
	}
	return out
}

func hasInboundBotMention(text, botOpenID string, mentions []string, mentionEvents []*larkim.MentionEvent) bool {
	botOpenID = strings.TrimSpace(botOpenID)
	if botOpenID == "" {
		return false
	}
	for _, mention := range mentions {
		if strings.TrimSpace(mention) == botOpenID {
			return true
		}
	}
	for _, mention := range mentionEvents {
		if mention == nil || mention.Id == nil || mention.Id.OpenId == nil {
			continue
		}
		if strings.TrimSpace(stringValue(mention.Id.OpenId)) == botOpenID {
			return true
		}
	}
	for _, match := range atMentionRegexp.FindAllStringSubmatch(text, -1) {
		if len(match) < 2 {
			continue
		}
		if strings.TrimSpace(match[1]) == botOpenID {
			return true
		}
	}
	return false
}

func stripMentionPlaceholders(content string, mentions []*larkim.MentionEvent) string {
	content = strings.TrimSpace(content)
	for _, mention := range mentions {
		if mention == nil || mention.Key == nil || *mention.Key == "" {
			continue
		}
		content = strings.ReplaceAll(content, *mention.Key, "")
	}
	content = mentionPlaceholder.ReplaceAllString(content, "")
	return strings.TrimSpace(content)
}

func extractPostText(content string) string {
	var payload struct {
		Title   string             `json:"title"`
		Content [][]map[string]any `json:"content"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return ""
	}

	var lines []string
	if title := strings.TrimSpace(payload.Title); title != "" {
		lines = append(lines, title)
	}
	for _, line := range payload.Content {
		var b strings.Builder
		for _, elem := range line {
			switch tag, _ := elem["tag"].(string); tag {
			case "text", "a":
				b.WriteString(postStringField(elem, "text"))
			case "at":
				name := postStringField(elem, "user_name")
				if name == "" {
					name = postStringField(elem, "text")
				}
				if name != "" {
					b.WriteString("@")
					b.WriteString(name)
				}
			}
		}
		if text := strings.TrimSpace(b.String()); text != "" {
			lines = append(lines, text)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func postStringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	value, _ := m[key].(string)
	return strings.TrimSpace(value)
}
