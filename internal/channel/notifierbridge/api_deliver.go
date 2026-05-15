package notifierbridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/runtime/notifier"
)

// RoomLister returns IM room IDs for a member (typically *im.Service).
type RoomLister interface {
	RoomIDsForMember(memberID string) []string
}

// APIDeliver fans out notifier content by posting to POST /api/v1/messages per room.
type APIDeliver struct {
	Rooms   RoomLister
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewAPIDeliver wires room membership lookup and the local HTTP API base URL.
func NewAPIDeliver(rooms RoomLister, baseURL, accessToken string) *APIDeliver {
	return &APIDeliver{
		Rooms:   rooms,
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Token:   strings.TrimSpace(accessToken),
		HTTP:    &http.Client{},
	}
}

func (d *APIDeliver) httpClient() *http.Client {
	if d != nil && d.HTTP != nil {
		return d.HTTP
	}
	return http.DefaultClient
}

// RoomIDsForAgent implements notifier.RoomMessenger.
func (d *APIDeliver) RoomIDsForAgent(agentID string) []string {
	if d == nil || d.Rooms == nil {
		return nil
	}
	return d.Rooms.RoomIDsForMember(agentID)
}

// PostMessage implements notifier.RoomMessenger.
func (d *APIDeliver) PostMessage(req apitypes.CreateMessageRequest) error {
	if d == nil || strings.TrimSpace(d.BaseURL) == "" {
		return fmt.Errorf("notifier api deliver: base url is required")
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal create message: %w", err)
	}
	url := d.BaseURL + "/api/v1/messages"
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if tok := strings.TrimSpace(d.Token); tok != "" {
		httpReq.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := d.httpClient().Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("post message: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// DeliverNotifierFanout implements notifier.Fanouter.
func (d *APIDeliver) DeliverNotifierFanout(agentID, content string) error {
	return notifier.DeliverNotifierFanout(agentID, content, d)
}
