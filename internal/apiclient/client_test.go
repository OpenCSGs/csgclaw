package apiclient

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
)

type recordingHTTPClient struct {
	status   int
	body     string
	requests []string
}

func (c *recordingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.requests = append(c.requests, req.Method+" "+req.URL.RequestURI())
	status := c.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(c.body)),
		Header:     make(http.Header),
	}, nil
}

func TestClientUsesCsgclawChannelRoutes(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		status int
		body   string
		want   string
		call   func(*Client) error
	}{
		{
			name: "list rooms",
			body: `[]`,
			want: "GET /api/v1/channels/csgclaw/rooms",
			call: func(c *Client) error {
				_, err := c.ListRoomsByChannel(ctx, "csgclaw")
				return err
			},
		},
		{
			name: "create room",
			body: `{}`,
			want: "POST /api/v1/channels/csgclaw/rooms",
			call: func(c *Client) error {
				_, err := c.CreateRoomByChannel(ctx, "csgclaw", apitypes.CreateRoomRequest{Title: "room", CreatorID: "u-admin"})
				return err
			},
		},
		{
			name: "send message",
			body: `{}`,
			want: "POST /api/v1/channels/csgclaw/messages",
			call: func(c *Client) error {
				_, err := c.SendMessageByChannel(ctx, "csgclaw", apitypes.CreateMessageRequest{RoomID: "room-1", SenderID: "u-admin", Content: "hello"})
				return err
			},
		},
		{
			name: "list messages",
			body: `[]`,
			want: "GET /api/v1/channels/csgclaw/messages?room_id=room-1",
			call: func(c *Client) error {
				_, err := c.ListMessagesByChannel(ctx, "csgclaw", "room-1")
				return err
			},
		},
		{
			name: "add room member",
			body: `{}`,
			want: "POST /api/v1/channels/csgclaw/rooms/room-1/members",
			call: func(c *Client) error {
				_, err := c.AddRoomMemberByChannel(ctx, "csgclaw", apitypes.AddRoomMembersRequest{RoomID: "room-1", InviterID: "u-admin", UserIDs: []string{"u-alice"}})
				return err
			},
		},
		{
			name: "list room members",
			body: `[]`,
			want: "GET /api/v1/channels/csgclaw/rooms/room-1/members",
			call: func(c *Client) error {
				_, err := c.ListRoomMembersByChannel(ctx, "csgclaw", "room-1")
				return err
			},
		},
		{
			name:   "delete room",
			status: http.StatusNoContent,
			want:   "DELETE /api/v1/channels/csgclaw/rooms/room-1",
			call: func(c *Client) error {
				return c.DeleteRoom(ctx, "csgclaw", "room-1")
			},
		},
		{
			name: "list users",
			body: `[]`,
			want: "GET /api/v1/channels/csgclaw/users",
			call: func(c *Client) error {
				_, err := c.ListUsersByChannel(ctx, "csgclaw")
				return err
			},
		},
		{
			name: "create user",
			body: `{}`,
			want: "POST /api/v1/channels/csgclaw/users",
			call: func(c *Client) error {
				_, err := c.CreateUser(ctx, "csgclaw", apitypes.CreateUserRequest{ID: "u-alice", Name: "Alice"})
				return err
			},
		},
		{
			name:   "delete user",
			status: http.StatusNoContent,
			want:   "DELETE /api/v1/channels/csgclaw/users/u-alice",
			call: func(c *Client) error {
				return c.DeleteUser(ctx, "csgclaw", "u-alice")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordingHTTPClient{status: tt.status, body: tt.body}
			client := New("http://example.test", "", rec)
			if err := tt.call(client); err != nil {
				t.Fatalf("client call error = %v", err)
			}
			if len(rec.requests) != 1 {
				t.Fatalf("requests = %+v, want one request", rec.requests)
			}
			if got := rec.requests[0]; got != tt.want {
				t.Fatalf("request = %q, want %q", got, tt.want)
			}
		})
	}
}
