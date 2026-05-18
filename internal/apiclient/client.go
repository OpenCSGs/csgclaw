package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"csgclaw/internal/apitypes"
)

const DefaultHTTPPort = "18080"

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	endpoint string
	token    string
	client   HTTPClient
}

func DefaultAPIBaseURL() string {
	return "http://" + net.JoinHostPort("127.0.0.1", DefaultHTTPPort)
}

func New(endpoint, token string, client HTTPClient) *Client {
	if endpoint == "" {
		endpoint = DefaultAPIBaseURL()
	}
	if client == nil {
		client = &http.Client{}
	}
	return &Client{
		endpoint: strings.TrimRight(endpoint, "/"),
		token:    token,
		client:   client,
	}
}

func (c *Client) ListBots(ctx context.Context, channel, role string) ([]apitypes.Bot, error) {
	var bots []apitypes.Bot
	values := url.Values{}
	if strings.TrimSpace(role) != "" {
		values.Set("role", strings.TrimSpace(role))
	}
	path, err := botCollectionPath(channel)
	if err != nil {
		return nil, err
	}
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	if err := c.GetJSON(ctx, path, &bots); err != nil {
		return nil, err
	}
	return bots, nil
}

func (c *Client) CreateBot(ctx context.Context, req apitypes.CreateBotRequest) (apitypes.Bot, error) {
	var created apitypes.Bot
	path, err := botCollectionPath(req.Channel)
	if err != nil {
		return apitypes.Bot{}, err
	}
	if err := c.DoJSON(ctx, http.MethodPost, path, req, &created); err != nil {
		return apitypes.Bot{}, err
	}
	return created, nil
}

func (c *Client) DeleteBot(ctx context.Context, channel, id string) error {
	path, err := botItemPath(channel, id)
	if err != nil {
		return err
	}
	return c.DoNoContent(ctx, http.MethodDelete, path)
}

func (c *Client) ListRooms(ctx context.Context) ([]apitypes.Room, error) {
	return c.ListRoomsByChannel(ctx, "csgclaw")
}

func (c *Client) ListRoomsByChannel(ctx context.Context, channel string) ([]apitypes.Room, error) {
	var rooms []apitypes.Room
	path, err := channelPath(channel, "rooms")
	if err != nil {
		return nil, err
	}
	if err := c.GetJSON(ctx, path, &rooms); err != nil {
		return nil, err
	}
	return rooms, nil
}

func (c *Client) CreateRoom(ctx context.Context, req apitypes.CreateRoomRequest) (apitypes.Room, error) {
	return c.CreateRoomByChannel(ctx, "csgclaw", req)
}

func (c *Client) CreateRoomByChannel(ctx context.Context, channel string, req apitypes.CreateRoomRequest) (apitypes.Room, error) {
	var created apitypes.Room
	path, err := channelPath(channel, "rooms")
	if err != nil {
		return apitypes.Room{}, err
	}
	if err := c.DoJSON(ctx, http.MethodPost, path, req, &created); err != nil {
		return apitypes.Room{}, err
	}
	return created, nil
}

func (c *Client) SendMessageByChannel(ctx context.Context, channel string, req apitypes.CreateMessageRequest) (apitypes.Message, error) {
	var created apitypes.Message
	path, err := channelPath(channel, "messages")
	if err != nil {
		return apitypes.Message{}, err
	}
	if err := c.DoJSON(ctx, http.MethodPost, path, req, &created); err != nil {
		return apitypes.Message{}, err
	}
	return created, nil
}

func (c *Client) ListMessagesByChannel(ctx context.Context, channel, roomID string) ([]apitypes.Message, error) {
	var messages []apitypes.Message
	path, err := messageListPath(channel, roomID)
	if err != nil {
		return nil, err
	}
	if err := c.GetJSON(ctx, path, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (c *Client) AddRoomMemberByChannel(ctx context.Context, channel string, req apitypes.AddRoomMembersRequest) (apitypes.Room, error) {
	var updated apitypes.Room
	path, err := memberCreatePath(channel, req.RoomID)
	if err != nil {
		return apitypes.Room{}, err
	}
	if err := c.DoJSON(ctx, http.MethodPost, path, req, &updated); err != nil {
		return apitypes.Room{}, err
	}
	return updated, nil
}

func (c *Client) ListRoomMembersByChannel(ctx context.Context, channel, roomID string) ([]apitypes.User, error) {
	var users []apitypes.User
	path, err := roomMembersPath(channel, roomID, "list")
	if err != nil {
		return nil, err
	}
	if err := c.GetJSON(ctx, path, &users); err != nil {
		return nil, err
	}
	return users, nil
}

func (c *Client) DeleteRoom(ctx context.Context, channel, id string) error {
	path, err := roomDeletePath(channel, id)
	if err != nil {
		return err
	}
	return c.DoNoContent(ctx, http.MethodDelete, path)
}

func (c *Client) ListUsers(ctx context.Context) ([]apitypes.User, error) {
	return c.ListUsersByChannel(ctx, "csgclaw")
}

func (c *Client) ListUsersByChannel(ctx context.Context, channel string) ([]apitypes.User, error) {
	var users []apitypes.User
	path, err := channelPath(channel, "users")
	if err != nil {
		return nil, err
	}
	if err := c.GetJSON(ctx, path, &users); err != nil {
		return nil, err
	}
	return users, nil
}

func (c *Client) CreateUser(ctx context.Context, channel string, req apitypes.CreateUserRequest) (apitypes.User, error) {
	var created apitypes.User
	path, err := channelPath(channel, "users")
	if err != nil {
		return apitypes.User{}, err
	}
	if err := c.DoJSON(ctx, http.MethodPost, path, req, &created); err != nil {
		return apitypes.User{}, err
	}
	return created, nil
}

func (c *Client) DeleteUser(ctx context.Context, channel, id string) error {
	path, err := userDeletePath(channel, id)
	if err != nil {
		return err
	}
	return c.DoNoContent(ctx, http.MethodDelete, path)
}

func (c *Client) Stream(ctx context.Context, path string, values url.Values, w io.Writer) error {
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+path, nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ExtractAPIError(resp)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

func (c *Client) GetJSON(ctx context.Context, path string, out any) error {
	return c.DoJSON(ctx, http.MethodGet, path, nil, out)
}

func (c *Client) DoNoContent(ctx context.Context, method, path string) error {
	return c.DoJSON(ctx, method, path, nil, nil)
}

func (c *Client) DoJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
		reader = &buf
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ExtractAPIError(resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return err
	}
	return nil
}

func QueryInt(values url.Values, key string, value int) {
	if value > 0 {
		values.Set(key, strconv.Itoa(value))
	}
}

func ExtractAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if msg := ExtractAPIErrorMessage(body); msg != "" {
		return fmt.Errorf("%s", msg)
	}
	return fmt.Errorf("request failed")
}

func ExtractAPIErrorMessage(body []byte) string {
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return ""
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		for _, key := range []string{"error", "message"} {
			if value, ok := payload[key].(string); ok {
				value = strings.TrimSpace(value)
				if value != "" {
					return value
				}
			}
		}
	}

	return msg
}

func channelPath(channelName, resource string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(channelName)) {
	case "":
		return "/api/v1/" + resource, nil
	case "csgclaw":
		return "/api/v1/channels/csgclaw/" + resource, nil
	case "feishu":
		return "/api/v1/channels/feishu/" + resource, nil
	default:
		return "", fmt.Errorf("unsupported channel %q", channelName)
	}
}

func botCollectionPath(channelName string) (string, error) {
	channelName = strings.ToLower(strings.TrimSpace(channelName))
	if channelName == "" {
		channelName = "csgclaw"
	}
	switch channelName {
	case "csgclaw", "feishu":
		return "/api/v1/channels/" + channelName + "/bots", nil
	default:
		return "", fmt.Errorf("unsupported channel %q", channelName)
	}
}

func botItemPath(channelName, botID string) (string, error) {
	path, err := botCollectionPath(channelName)
	if err != nil {
		return "", err
	}
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return "", fmt.Errorf("bot id is required")
	}
	return path + "/" + url.PathEscape(botID), nil
}

func memberCreatePath(channelName, roomID string) (string, error) {
	return roomMembersPath(channelName, roomID, "create")
}

func roomMembersPath(channelName, roomID, operation string) (string, error) {
	channelName = strings.ToLower(strings.TrimSpace(channelName))
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return "", fmt.Errorf("room_id is required")
	}

	switch channelName {
	case "feishu":
		return "/api/v1/channels/feishu/rooms/" + url.PathEscape(roomID) + "/members", nil
	case "csgclaw":
		return "/api/v1/channels/csgclaw/rooms/" + url.PathEscape(roomID) + "/members", nil
	case "":
		return "/api/v1/rooms/" + url.PathEscape(roomID) + "/members", nil
	default:
		return "", fmt.Errorf("unsupported channel %q", channelName)
	}
}

func roomDeletePath(channelName, roomID string) (string, error) {
	channelName = strings.ToLower(strings.TrimSpace(channelName))
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return "", fmt.Errorf("room id is required")
	}
	switch channelName {
	case "":
		return "/api/v1/rooms/" + url.PathEscape(roomID), nil
	case "csgclaw":
		return "/api/v1/channels/csgclaw/rooms/" + url.PathEscape(roomID), nil
	case "feishu":
		return "/api/v1/channels/feishu/rooms/" + url.PathEscape(roomID), nil
	default:
		return "", fmt.Errorf("unsupported channel %q", channelName)
	}
}

func userDeletePath(channelName, userID string) (string, error) {
	channelName = strings.ToLower(strings.TrimSpace(channelName))
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", fmt.Errorf("user id is required")
	}
	switch channelName {
	case "":
		return "/api/v1/users/" + url.PathEscape(userID), nil
	case "csgclaw":
		return "/api/v1/channels/csgclaw/users/" + url.PathEscape(userID), nil
	case "feishu":
		return "/api/v1/channels/feishu/users/" + url.PathEscape(userID), nil
	default:
		return "", fmt.Errorf("unsupported channel %q", channelName)
	}
}

func messageListPath(channelName, roomID string) (string, error) {
	path, err := channelPath(channelName, "messages")
	if err != nil {
		return "", err
	}
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return "", fmt.Errorf("room_id is required")
	}
	return path + "?room_id=" + url.QueryEscape(roomID), nil
}
