package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larknormalize "github.com/larksuite/oapi-sdk-go/v3/channel/normalize"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	larkdispatcher "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkcallback "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

const ingressStartTimeout = 15 * time.Second

type oapiIngress struct {
	mu sync.Mutex

	appID   string
	client  *lark.Client
	socket  *larkws.Client
	handler func(context.Context, Event) error
	cancel  context.CancelFunc
	started bool
}

func newOAPIIngress(appID, appSecret string, client *lark.Client) (*oapiIngress, error) {
	if strings.TrimSpace(appID) == "" || strings.TrimSpace(appSecret) == "" || client == nil {
		return nil, ErrInvalidConfig
	}
	ingress := &oapiIngress{appID: appID, client: client}
	dispatcher := newOAPIEventDispatcher(ingress)
	ingress.socket = larkws.NewClient(
		appID,
		appSecret,
		larkws.WithEventHandler(dispatcher),
		larkws.WithDomain(lark.FeishuBaseUrl),
	)
	return ingress, nil
}

func newOAPIEventDispatcher(ingress *oapiIngress) *larkdispatcher.EventDispatcher {
	dispatcher := larkdispatcher.NewEventDispatcher("", "")
	dispatcher.OnP2MessageReceiveV1(ingress.handleMessage)
	dispatcher.OnCustomizedEvent("drive.notice.comment_add_v1", ingress.handleComment)
	dispatcher.OnP2CardActionTrigger(ingress.handleCardAction)
	// Processing reactions are outbound presentation state, not Agent input.
	// Feishu may echo their lifecycle events back to this connection.
	dispatcher.OnP2MessageReactionCreatedV1(func(context.Context, *larkim.P2MessageReactionCreatedV1) error { return nil })
	dispatcher.OnP2MessageReactionDeletedV1(func(context.Context, *larkim.P2MessageReactionDeletedV1) error { return nil })
	return dispatcher
}

func (i *oapiIngress) Connect(ctx context.Context, handler func(context.Context, Event) error) error {
	if i == nil || i.socket == nil || handler == nil {
		return ErrInvalidConfig
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithCancel(ctx)
	ready := make(chan struct{}, 1)
	done := make(chan error, 1)

	i.mu.Lock()
	if i.started {
		i.mu.Unlock()
		cancel()
		return ErrAlreadyStarted
	}
	i.started = true
	i.handler = handler
	i.cancel = cancel
	i.socket.SetOnReady(func() {
		select {
		case ready <- struct{}{}:
		default:
		}
	})
	i.mu.Unlock()

	go func() { done <- i.socket.Start(runCtx) }()
	timer := time.NewTimer(ingressStartTimeout)
	defer timer.Stop()
	select {
	case <-ready:
		return nil
	case err := <-done:
		i.resetStart()
		if err == nil {
			return errors.New("feishu websocket stopped before becoming ready")
		}
		return err
	case <-timer.C:
		cancel()
		i.socket.Close()
		i.resetStart()
		return errors.New("feishu websocket start timed out")
	case <-ctx.Done():
		cancel()
		i.socket.Close()
		i.resetStart()
		return ctx.Err()
	}
}

func (i *oapiIngress) Disconnect(context.Context) error {
	if i == nil || i.socket == nil {
		return nil
	}
	i.mu.Lock()
	cancel := i.cancel
	i.cancel = nil
	i.handler = nil
	i.started = false
	i.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	i.socket.Close()
	return nil
}

func (i *oapiIngress) resetStart() {
	i.mu.Lock()
	i.cancel = nil
	i.handler = nil
	i.started = false
	i.mu.Unlock()
}

func (i *oapiIngress) Identity(ctx context.Context) (Identity, error) {
	if i == nil || i.client == nil {
		return Identity{}, ErrInvalidConfig
	}
	response, err := i.client.Get(ctx, "/open-apis/bot/v3/info", nil, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return Identity{}, requestAPIError("get bot identity", err)
	}
	if response == nil {
		return Identity{}, missingAPIResponse("get bot identity")
	}
	if response.StatusCode != http.StatusOK {
		return Identity{}, &APIError{Operation: "get bot identity", HTTPStatus: response.StatusCode}
	}
	var wire struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Bot  struct {
			OpenID  string `json:"open_id"`
			AppName string `json:"app_name"`
		} `json:"bot"`
	}
	if err := json.Unmarshal(response.RawBody, &wire); err != nil {
		return Identity{}, fmt.Errorf("decode feishu bot identity: %w", err)
	}
	if wire.Code != 0 {
		return Identity{}, &APIError{Operation: "get bot identity", Code: wire.Code, Message: wire.Msg}
	}
	if strings.TrimSpace(wire.Bot.OpenID) == "" {
		return Identity{}, errors.New("feishu bot identity open_id is missing")
	}
	return Identity{OpenID: strings.TrimSpace(wire.Bot.OpenID), Name: strings.TrimSpace(wire.Bot.AppName)}, nil
}

func (i *oapiIngress) dispatch(ctx context.Context, event Event) error {
	i.mu.Lock()
	handler := i.handler
	i.mu.Unlock()
	if handler == nil {
		return ErrNotStarted
	}
	return handler(ctx, event)
}

func (i *oapiIngress) handleMessage(ctx context.Context, input *larkim.P2MessageReceiveV1) error {
	event := Event{Kind: EventMessage}
	if input == nil || input.Event == nil || input.Event.Message == nil {
		return i.dispatch(ctx, event)
	}
	event.EventID, event.OccurredAt = eventBase(input.EventV2Base)
	message := input.Event.Message
	messageType := stringValue(message.MessageType)
	text, raw, resources := parseMessageContent(messageType, stringValue(message.Content))
	mapped := Message{
		ID:          stringValue(message.MessageId),
		ChatID:      stringValue(message.ChatId),
		ChatType:    ChatType(stringValue(message.ChatType)),
		ThreadID:    stringValue(message.ThreadId),
		RootID:      stringValue(message.RootId),
		ParentID:    stringValue(message.ParentId),
		Text:        text,
		ContentType: messageType,
		RawContent:  raw,
		Resources:   resources,
		CreatedAt:   parseMillis(stringValue(message.CreateTime)),
	}
	if mapped.CreatedAt.IsZero() {
		mapped.CreatedAt = event.OccurredAt
	}
	if sender := input.Event.Sender; sender != nil {
		mapped.SenderType = SenderType(stringValue(sender.SenderType))
		if sender.SenderId != nil {
			mapped.Sender = Identity{
				OpenID:  stringValue(sender.SenderId.OpenId),
				UserID:  stringValue(sender.SenderId.UserId),
				UnionID: stringValue(sender.SenderId.UnionId),
			}
		}
	}
	for _, mention := range message.Mentions {
		if mention == nil {
			continue
		}
		item := Mention{Key: stringValue(mention.Key), Name: stringValue(mention.Name)}
		if mention.Id != nil {
			item.OpenID = stringValue(mention.Id.OpenId)
			item.UserID = stringValue(mention.Id.UserId)
		}
		mapped.Mentions = append(mapped.Mentions, item)
	}
	event.Message = &mapped
	return i.dispatch(ctx, event)
}

func (i *oapiIngress) handleComment(ctx context.Context, input *larkevent.EventReq) error {
	event := Event{Kind: EventComment}
	if input == nil {
		return i.dispatch(ctx, event)
	}
	raw := append(json.RawMessage(nil), input.Body...)
	var wire struct {
		Header struct {
			EventID    string `json:"event_id"`
			CreateTime string `json:"create_time"`
		} `json:"header"`
		Event struct {
			CommentID   string `json:"comment_id"`
			ReplyID     string `json:"reply_id"`
			FileToken   string `json:"file_token"`
			FileType    string `json:"file_type"`
			CreateTime  string `json:"create_time"`
			ActionTime  string `json:"action_time"`
			IsMentioned *bool  `json:"is_mentioned"`
			IsMention   *bool  `json:"is_mention"`
			UserID      *struct {
				OpenID  string `json:"open_id"`
				UserID  string `json:"user_id"`
				UnionID string `json:"union_id"`
			} `json:"user_id"`
			NoticeMeta *struct {
				FileToken   string `json:"file_token"`
				FileType    string `json:"file_type"`
				NoticeType  string `json:"notice_type"`
				Timestamp   string `json:"timestamp"`
				IsMentioned *bool  `json:"is_mentioned"`
				FromUserID  *struct {
					OpenID  string `json:"open_id"`
					UserID  string `json:"user_id"`
					UnionID string `json:"union_id"`
				} `json:"from_user_id"`
			} `json:"notice_meta"`
		} `json:"event"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	event.EventID = strings.TrimSpace(wire.Header.EventID)
	event.OccurredAt = parseMillis(wire.Header.CreateTime)
	comment := Comment{
		FileToken: strings.TrimSpace(wire.Event.FileToken),
		FileType:  strings.TrimSpace(wire.Event.FileType),
		CommentID: strings.TrimSpace(wire.Event.CommentID),
		ReplyID:   strings.TrimSpace(wire.Event.ReplyID),
		CreatedAt: parseMillis(ingressFirstNonEmpty(wire.Event.CreateTime, wire.Event.ActionTime)),
	}
	if wire.Event.UserID != nil {
		comment.Operator = Identity{OpenID: wire.Event.UserID.OpenID, UserID: wire.Event.UserID.UserID, UnionID: wire.Event.UserID.UnionID}
	}
	if notice := wire.Event.NoticeMeta; notice != nil {
		comment.FileToken = ingressFirstNonEmpty(comment.FileToken, notice.FileToken)
		comment.FileType = ingressFirstNonEmpty(comment.FileType, notice.FileType)
		if comment.CreatedAt.IsZero() {
			comment.CreatedAt = parseMillis(notice.Timestamp)
		}
		if notice.FromUserID != nil {
			comment.Operator = Identity{OpenID: notice.FromUserID.OpenID, UserID: notice.FromUserID.UserID, UnionID: notice.FromUserID.UnionID}
		}
		if notice.IsMentioned != nil {
			comment.MentionedBot = *notice.IsMentioned
		}
	}
	if wire.Event.IsMentioned != nil {
		comment.MentionedBot = *wire.Event.IsMentioned
	} else if wire.Event.IsMention != nil {
		comment.MentionedBot = *wire.Event.IsMention
	}
	if comment.CreatedAt.IsZero() {
		comment.CreatedAt = event.OccurredAt
	}
	event.Comment = &comment
	return i.dispatch(ctx, event)
}

func (i *oapiIngress) handleCardAction(ctx context.Context, input *larkcallback.CardActionTriggerEvent) (*larkcallback.CardActionTriggerResponse, error) {
	event := Event{Kind: EventCardAction}
	if input == nil || input.Event == nil {
		return nil, i.dispatch(ctx, event)
	}
	event.EventID, event.OccurredAt = eventBase(input.EventV2Base)
	request := input.Event
	action := CardAction{
		Token:        strings.TrimSpace(request.Token),
		Host:         strings.TrimSpace(request.Host),
		DeliveryType: strings.TrimSpace(request.DeliveryType),
		CreatedAt:    event.OccurredAt,
	}
	if request.Operator != nil {
		action.Operator = Identity{OpenID: strings.TrimSpace(request.Operator.OpenID), UserID: stringValue(request.Operator.UserID)}
	}
	if request.Action != nil {
		action.ActionValue = cloneAnyMap(request.Action.Value)
		action.FormValue = cloneAnyMap(request.Action.FormValue)
	}
	if request.Context != nil {
		action.MessageID = strings.TrimSpace(request.Context.OpenMessageID)
		action.ChatID = strings.TrimSpace(request.Context.OpenChatID)
	}
	action.ChatID, action.ChatType = ingressCardChat(action.ChatID, action.Operator.OpenID)
	action.Mode = ingressChatMode(string(action.ChatType), "")
	event.CardAction = &action
	return nil, i.dispatch(ctx, event)
}

func eventBase(base *larkevent.EventV2Base) (string, time.Time) {
	if base == nil || base.Header == nil {
		return "", time.Time{}
	}
	return strings.TrimSpace(base.Header.EventID), parseMillis(base.Header.CreateTime)
}

func parseMessageContent(kind, rawValue string) (string, json.RawMessage, []Resource) {
	raw := json.RawMessage(strings.TrimSpace(rawValue))
	if len(raw) == 0 {
		return "", nil, nil
	}
	fields := make(map[string]json.RawMessage)
	if kind == "post" {
		text, normalized := larknormalize.ParseContent(kind, preferredPostContent(raw))
		resources := make([]Resource, 0, len(normalized))
		for _, resource := range normalized {
			fileKey := strings.TrimSpace(resource.FileKey)
			if fileKey == "" {
				continue
			}
			resources = append(resources, Resource{
				Kind: strings.TrimSpace(resource.Type),
				ID:   fileKey,
				Name: strings.TrimSpace(resource.FileName),
			})
		}
		return text, raw, resources
	}
	if json.Unmarshal(raw, &fields) != nil {
		return string(raw), raw, nil
	}
	stringField := func(name string) string {
		var value string
		_ = json.Unmarshal(fields[name], &value)
		return strings.TrimSpace(value)
	}
	switch kind {
	case "text":
		return stringField("text"), raw, nil
	case "image":
		key := stringField("image_key")
		return fmt.Sprintf("![image](%s)", key), raw, []Resource{{Kind: "image", ID: key}}
	case "file":
		key, name := stringField("file_key"), stringField("file_name")
		return fmt.Sprintf(`<file key="%s" name="%s"/>`, html.EscapeString(key), html.EscapeString(name)), raw,
			[]Resource{{Kind: "file", ID: key, Name: name}}
	case "audio":
		key := stringField("file_key")
		return fmt.Sprintf(`<audio key="%s"/>`, html.EscapeString(key)), raw, []Resource{{Kind: "audio", ID: key}}
	case "media", "video":
		key, name := stringField("file_key"), stringField("file_name")
		return fmt.Sprintf(`<video key="%s" name="%s"/>`, html.EscapeString(key), html.EscapeString(name)), raw,
			[]Resource{{Kind: "video", ID: key, Name: name}}
	case "interactive":
		return string(raw), raw, nil
	default:
		if text := stringField("text"); text != "" {
			return text, raw, nil
		}
		return "[unsupported message]", raw, nil
	}
}

func preferredPostContent(raw json.RawMessage) string {
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(raw, &fields); err != nil {
		return string(raw)
	}
	if isPostBody(fields) {
		return localizedPostContent("zh_cn", raw, raw)
	}

	selectedLocale := ""
	selectedBody := json.RawMessage(nil)
	selectedRank := 0
	for locale, body := range fields {
		bodyFields := make(map[string]json.RawMessage)
		if json.Unmarshal(body, &bodyFields) != nil || !isPostBody(bodyFields) {
			continue
		}
		rank := postLocaleRank(locale)
		if selectedLocale == "" || rank < selectedRank || rank == selectedRank && locale < selectedLocale {
			selectedLocale, selectedBody, selectedRank = locale, body, rank
		}
	}
	if selectedLocale == "" {
		return string(raw)
	}
	return localizedPostContent(selectedLocale, selectedBody, raw)
}

func localizedPostContent(locale string, body, fallback json.RawMessage) string {
	selected, err := json.Marshal(map[string]json.RawMessage{locale: body})
	if err != nil {
		return string(fallback)
	}
	return string(selected)
}

func isPostBody(fields map[string]json.RawMessage) bool {
	for _, name := range []string{"content_v2", "content"} {
		var paragraphs []json.RawMessage
		if value, ok := fields[name]; ok && json.Unmarshal(value, &paragraphs) == nil {
			return true
		}
	}
	return false
}

func postLocaleRank(locale string) int {
	switch strings.ReplaceAll(strings.ToLower(strings.TrimSpace(locale)), "-", "_") {
	case "zh_cn":
		return 0
	case "en_us":
		return 1
	default:
		return 2
	}
}

func parseMillis(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	var millis int64
	if _, err := fmt.Sscan(value, &millis); err != nil {
		return time.Time{}
	}
	if millis >= 1_000_000_000_000_000_000 {
		return time.Unix(0, millis).UTC()
	}
	if millis >= 1_000_000_000_000_000 {
		return time.UnixMicro(millis).UTC()
	}
	if millis < 1_000_000_000_000 {
		return time.Unix(millis, 0).UTC()
	}
	return time.UnixMilli(millis).UTC()
}

func ingressFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func ingressChatMode(chatType, threadID string) ChatMode {
	if strings.TrimSpace(threadID) != "" {
		return ModeTopic
	}
	if strings.EqualFold(strings.TrimSpace(chatType), string(ChatP2P)) {
		return ModeP2P
	}
	return ModeGroup
}

func ingressCardChat(chatID, operatorOpenID string) (string, ChatType) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		chatID = strings.TrimSpace(operatorOpenID)
	}
	if strings.HasPrefix(chatID, "ou_") || strings.HasPrefix(chatID, "on_") || strings.HasPrefix(chatID, "un_") {
		return chatID, ChatP2P
	}
	return chatID, ChatGroup
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

var _ ingressConnection = (*oapiIngress)(nil)
var _ ingressIdentitySource = (*oapiIngress)(nil)
