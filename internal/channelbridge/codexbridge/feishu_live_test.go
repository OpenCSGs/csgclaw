//go:build feishu_live

package codexbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/api"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/channel/feishu/participantprovider"
	runtimecodex "csgclaw/internal/runtime/codex"
)

func TestFeishuLiveCodexBridgeRepliesThroughOpenAPI(t *testing.T) {
	if os.Getenv("CSGCLAW_FEISHU_LIVE") != "1" {
		t.Skip("set CSGCLAW_FEISHU_LIVE=1 to run live Feishu OpenAPI bridge test")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("user home: %v", err)
	}
	participantsPath := strings.TrimSpace(os.Getenv("CSGCLAW_FEISHU_PARTICIPANTS"))
	if participantsPath == "" {
		participantsPath = filepath.Join(home, ".csgclaw", "im", "participants.json")
	}
	roomID := strings.TrimSpace(os.Getenv("CSGCLAW_FEISHU_LIVE_ROOM"))
	if roomID == "" {
		t.Fatal("CSGCLAW_FEISHU_LIVE_ROOM is required")
	}
	senderID := firstNonEmpty(os.Getenv("CSGCLAW_FEISHU_LIVE_SENDER"), "manager")
	botID := firstNonEmpty(os.Getenv("CSGCLAW_FEISHU_LIVE_BOT"), "agent-2z5jvj")

	provider := participantprovider.New(participantsPath)
	feishuSvc := feishu.NewServiceWithProvider(provider)
	token := "live-test-token"
	handler := api.NewHandlerWithAccessToken(nil, nil, nil, nil, feishuSvc, nil, token)
	server := httptest.NewServer(handler.Routes())
	defer server.Close()

	sink := runtimecodex.NewEventSink()
	replyText := "codex bridge live reply " + time.Now().UTC().Format("20060102T150405.000000000")
	prompter := liveFeishuPrompter{sink: sink, replyText: replyText}
	recordingClient := &recordingLiveBotClient{
		inner: &FeishuHTTPClient{
			BaseURL: server.URL,
			Token:   token,
		},
		sent: make(chan liveSendRecord, 1),
	}
	bridge := NewService(recordingClient, prompter, sink)
	defer bridge.Close()
	if err := bridge.StartBot(context.Background(), Binding{
		BotID:     botID,
		RuntimeID: "rt-live-feishu",
		SessionID: "sess-live-feishu",
	}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}

	promptText := "codex bridge live prompt " + time.Now().UTC().Format("20060102T150405.000000000")
	postJSON(t, server.URL+"/api/v1/channels/feishu/messages", token, map[string]string{
		"room_id":    roomID,
		"sender_id":  senderID,
		"mention_id": botID,
		"content":    promptText,
	})

	select {
	case sent := <-recordingClient.sent:
		if sent.botID != botID || sent.req.RoomID != roomID || sent.req.Text != replyText || sent.resp.MessageID == "" {
			t.Fatalf("bridge send = %+v, want Feishu reply from %s in %s", sent, botID, roomID)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for bridge Feishu reply send")
	}

	if err := waitForFeishuMessage(t, server.URL, token, roomID, replyText); err != nil {
		if strings.Contains(err.Error(), "need scope: im:message.group_msg") {
			t.Logf("Feishu message list verification skipped: %v", err)
			return
		}
		t.Fatal(err)
	}
}

type liveFeishuPrompter struct {
	sink      *runtimecodex.EventSink
	replyText string
}

func (p liveFeishuPrompter) Prompt(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) (runtimecodex.PromptResponse, error) {
	p.sink.Publish(runtimecodex.SessionEvent{
		RuntimeID: handle.RuntimeID,
		SessionID: req.SessionID,
		Kind:      runtimecodex.SessionEventTextDelta,
		Text:      p.replyText,
	})
	p.sink.Publish(runtimecodex.SessionEvent{
		RuntimeID:  handle.RuntimeID,
		SessionID:  req.SessionID,
		Kind:       runtimecodex.SessionEventPromptCompleted,
		StopReason: runtimecodex.StopReasonEndTurn,
		ReceivedAt: time.Now().UTC(),
	})
	return runtimecodex.PromptResponse{StopReason: runtimecodex.StopReasonEndTurn}, nil
}

type recordingLiveBotClient struct {
	inner BotClient
	sent  chan liveSendRecord
}

type liveSendRecord struct {
	botID string
	req   SendMessageRequest
	resp  SendMessageResponse
}

func (c *recordingLiveBotClient) StreamEvents(ctx context.Context, botID, lastEventID string) (<-chan BotEvent, <-chan error) {
	return c.inner.StreamEvents(ctx, botID, lastEventID)
}

func (c *recordingLiveBotClient) SendMessage(ctx context.Context, botID string, req SendMessageRequest) (SendMessageResponse, error) {
	resp, err := c.inner.SendMessage(ctx, botID, req)
	if err == nil {
		select {
		case c.sent <- liveSendRecord{botID: botID, req: req, resp: resp}:
		default:
		}
	}
	return resp, err
}

func postJSON(t *testing.T, url, token string, payload any) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("post %s status = %d, want %d", url, resp.StatusCode, http.StatusCreated)
	}
}

func waitForFeishuMessage(t *testing.T, baseURL, token, roomID, text string) error {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/channels/feishu/messages?room_id="+roomID, nil)
		if err != nil {
			return fmt.Errorf("new request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("get Feishu messages: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read Feishu messages response: %w", readErr)
		}
		var messages []struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("list Feishu messages status = %d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		decodeErr := json.Unmarshal(body, &messages)
		if decodeErr != nil {
			return fmt.Errorf("decode Feishu messages: %w", decodeErr)
		}
		for _, message := range messages {
			if strings.Contains(message.Content, text) {
				return nil
			}
		}
		time.Sleep(1500 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for Feishu reply containing %q", text)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
