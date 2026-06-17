package codexbridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"csgclaw/internal/channel/feishu"
)

// FeishuHTTPClient bridges Codex workers to the participant-backed Feishu API.
type FeishuHTTPClient struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func (c *FeishuHTTPClient) StreamEvents(ctx context.Context, botID, lastEventID string) (<-chan BotEvent, <-chan error) {
	events := make(chan BotEvent, 16)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.participantEventsURL(botID), nil)
		if err != nil {
			errs <- err
			return
		}
		if token := strings.TrimSpace(c.Token); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if last := strings.TrimSpace(lastEventID); last != "" {
			req.Header.Set("Last-Event-ID", last)
		}

		resp, err := c.httpClient().Do(req)
		if err != nil {
			errs <- err
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			errs <- fmt.Errorf("stream feishu bot events: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			return
		}
		if err := decodeFeishuSSE(ctx, resp.Body, botID, events); err != nil && ctx.Err() == nil {
			errs <- err
		}
	}()

	return events, errs
}

func (c *FeishuHTTPClient) SendMessage(ctx context.Context, botID string, req SendMessageRequest) (SendMessageResponse, error) {
	payload, err := json.Marshal(map[string]string{
		"room_id":   strings.TrimSpace(req.RoomID),
		"sender_id": strings.TrimSpace(botID),
		"content":   strings.TrimSpace(req.Text),
	})
	if err != nil {
		return SendMessageResponse{}, fmt.Errorf("marshal feishu send message request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.messagesURL(), bytes.NewReader(payload))
	if err != nil {
		return SendMessageResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(c.Token); token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return SendMessageResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return SendMessageResponse{}, fmt.Errorf("send feishu bot message: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var sendResp struct {
		ID        string `json:"id"`
		MessageID string `json:"message_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sendResp); err != nil {
		return SendMessageResponse{}, fmt.Errorf("decode feishu send message response: %w", err)
	}
	messageID := strings.TrimSpace(sendResp.MessageID)
	if messageID == "" {
		messageID = strings.TrimSpace(sendResp.ID)
	}
	return SendMessageResponse{MessageID: messageID}, nil
}

func (c *FeishuHTTPClient) participantEventsURL(participantID string) string {
	baseURL := ""
	if c != nil {
		baseURL = c.BaseURL
	}
	return strings.TrimRight(baseURL, "/") + "/api/v1/channels/feishu/participants/" + url.PathEscape(strings.TrimSpace(participantID)) + "/events"
}

func (c *FeishuHTTPClient) messagesURL() string {
	baseURL := ""
	if c != nil {
		baseURL = c.BaseURL
	}
	return strings.TrimRight(baseURL, "/") + "/api/v1/channels/feishu/messages"
}

func (c *FeishuHTTPClient) httpClient() *http.Client {
	if c != nil && c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 0}
}

func decodeFeishuSSE(ctx context.Context, r io.Reader, botID string, events chan<- BotEvent) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var eventName string
	var dataLines []string
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if line == "" {
			if err := emitFeishuSSEEvent(eventName, dataLines, botID, events); err != nil {
				return err
			}
			eventName = ""
			dataLines = dataLines[:0]
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimLeft(value, " ")
		switch field {
		case "event":
			eventName = value
		case "data":
			dataLines = append(dataLines, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return emitFeishuSSEEvent(eventName, dataLines, botID, events)
}

func emitFeishuSSEEvent(eventName string, dataLines []string, botID string, events chan<- BotEvent) error {
	if eventName != "" && eventName != "message" {
		return nil
	}
	if len(dataLines) == 0 {
		return nil
	}
	var event feishu.MessageEvent
	if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &event); err != nil {
		return fmt.Errorf("decode feishu bot event: %w", err)
	}
	botEvent, ok := feishuMessageEventToBotEvent(event, botID)
	if !ok {
		return nil
	}
	events <- botEvent
	return nil
}

func feishuMessageEventToBotEvent(event feishu.MessageEvent, botID string) (BotEvent, bool) {
	if strings.TrimSpace(event.Type) != feishu.MessageEventTypeMessageCreated || event.Message == nil {
		return BotEvent{}, false
	}
	messageID := strings.TrimSpace(event.Message.ID)
	roomID := strings.TrimSpace(event.RoomID)
	if messageID == "" || roomID == "" {
		return BotEvent{}, false
	}
	mentions := make([]string, 0, len(event.Message.Mentions)+1)
	if mentionID := strings.TrimSpace(event.MentionBotID); mentionID != "" {
		mentions = append(mentions, mentionID)
	}
	for _, mention := range event.Message.Mentions {
		for _, id := range []string{mention.ID, mention.Name} {
			id = strings.TrimSpace(id)
			if id != "" {
				mentions = append(mentions, id)
			}
		}
	}
	return BotEvent{
		MessageID: messageID,
		RoomID:    roomID,
		ChatType:  "group",
		Text:      normalizeFeishuBotEventText(event.Message.Content),
		Mentions:  appendMissingString(mentions, strings.TrimSpace(botID)),
	}, true
}

func normalizeFeishuBotEventText(text string) string {
	text = strings.TrimSpace(text)
	for strings.HasPrefix(text, "<at ") {
		end := strings.Index(text, "</at>")
		if end < 0 {
			return text
		}
		text = strings.TrimSpace(text[end+len("</at>"):])
	}
	return text
}

func appendMissingString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.TrimSpace(existing) == value {
			return values
		}
	}
	return append(values, value)
}
