package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"csgclaw/internal/apitypes"
)

// RoomLister returns IM room IDs for a member (typically *im.Service).
type RoomLister interface {
	RoomIDsForMember(memberID string) []string
}

// APIDeliver fans out notification content by posting to POST /api/v1/messages per room.
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

// RoomIDsForMember implements RoomMessenger.
func (d *APIDeliver) RoomIDsForMember(memberID string) []string {
	if d == nil || d.Rooms == nil {
		return nil
	}
	return d.Rooms.RoomIDsForMember(memberID)
}

// PostMessage implements RoomMessenger.
func (d *APIDeliver) PostMessage(req apitypes.CreateMessageRequest) error {
	if d == nil || strings.TrimSpace(d.BaseURL) == "" {
		return fmt.Errorf("notification api deliver: base url is required")
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

// DeliverFanout implements Fanouter.
func (d *APIDeliver) DeliverFanout(memberID, content string) error {
	return DeliverFanout(memberID, content, d)
}
