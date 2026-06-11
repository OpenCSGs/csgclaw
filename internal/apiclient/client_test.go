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

func TestClientUsesExpectedRoutes(t *testing.T) {
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
			name: "clear room messages",
			body: `{"id":"room-1","messages":[]}`,
			want: "POST /api/v1/rooms/room-1:clearMessages",
			call: func(c *Client) error {
				_, err := c.ClearRoomMessages(ctx, "room-1")
				return err
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
		{
			name: "list teams",
			body: `[]`,
			want: "GET /api/v1/teams",
			call: func(c *Client) error {
				_, err := c.ListTeams(ctx)
				return err
			},
		},
		{
			name: "create team",
			body: `{}`,
			want: "POST /api/v1/teams",
			call: func(c *Client) error {
				_, err := c.CreateTeam(ctx, apitypes.CreateTeamRequest{Channel: "csgclaw", LeadAgentID: "u-manager"})
				return err
			},
		},
		{
			name: "list global tasks",
			body: `[]`,
			want: "GET /api/v1/tasks",
			call: func(c *Client) error {
				_, err := c.ListGlobalTasks(ctx)
				return err
			},
		},
		{
			name: "list team tasks",
			body: `[]`,
			want: "GET /api/v1/teams/team-1/tasks",
			call: func(c *Client) error {
				_, err := c.ListTeamTasks(ctx, "team-1")
				return err
			},
		},
		{
			name: "create team tasks batch",
			body: `{"tasks":[]}`,
			want: "POST /api/v1/teams/team-1/tasks/batch",
			call: func(c *Client) error {
				_, err := c.CreateTeamTasksBatch(ctx, "team-1", apitypes.CreateTeamTasksBatchRequest{CreatedBy: "bot-manager"})
				return err
			},
		},
		{
			name: "claim next team task",
			body: `{}`,
			want: "POST /api/v1/teams/team-1/tasks/claim-next",
			call: func(c *Client) error {
				_, err := c.ClaimNextTeamTask(ctx, apitypes.ClaimNextTeamTaskRequest{TeamID: "team-1", ParticipantID: "bot-worker"})
				return err
			},
		},
		{
			name: "claim specific team task",
			body: `{}`,
			want: "POST /api/v1/teams/team-1/tasks/task-1/claim",
			call: func(c *Client) error {
				_, err := c.ClaimTeamTask(ctx, "team-1", "task-1", "bot-worker")
				return err
			},
		},
		{
			name: "update team task",
			body: `{}`,
			want: "PATCH /api/v1/teams/team-1/tasks/task-1",
			call: func(c *Client) error {
				_, err := c.UpdateTeamTask(ctx, "team-1", "task-1", "bot-worker", apitypes.PatchTeamTaskRequest{Status: "completed", Result: "done"})
				return err
			},
		},
		{
			name: "assign team task",
			body: `{}`,
			want: "POST /api/v1/teams/team-1/tasks/task-1/assign",
			call: func(c *Client) error {
				_, err := c.AssignTeamTask(ctx, "team-1", "task-1", "bot-manager", "bot-worker")
				return err
			},
		},
		{
			name: "list team approvals",
			body: `[]`,
			want: "GET /api/v1/teams/team-1/approvals",
			call: func(c *Client) error {
				_, err := c.ListTeamApprovals(ctx, "team-1")
				return err
			},
		},
		{
			name: "create team approval",
			body: `{}`,
			want: "POST /api/v1/teams/team-1/approvals",
			call: func(c *Client) error {
				_, err := c.CreateTeamApproval(ctx, "team-1", apitypes.CreateTeamApprovalRequest{RequestedBy: "bot-worker", Kind: "command", Summary: "approve"})
				return err
			},
		},
		{
			name: "resolve team approval",
			body: `{}`,
			want: "POST /api/v1/teams/team-1/approvals/ap-1/resolve",
			call: func(c *Client) error {
				_, err := c.ResolveTeamApproval(ctx, "team-1", "ap-1", apitypes.ResolveTeamApprovalRequest{Status: "approved"})
				return err
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
