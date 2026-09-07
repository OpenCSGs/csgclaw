package apiclient

import (
	"context"
	"testing"

	"csgclaw/internal/apitypes"
)

func TestRoomTaskRoutesDoNotRequireTeam(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name, body, route string
		call              func(*Client) error
	}{
		{"context", `{}`, "GET /api/v1/rooms/room-1/task-context", func(c *Client) error { _, err := c.RoomTaskContext(ctx, "room-1"); return err }},
		{"list", `[]`, "GET /api/v1/rooms/room-1/tasks", func(c *Client) error { _, err := c.ListRoomTasks(ctx, "room-1"); return err }},
		{"submit", `{}`, "POST /api/v1/rooms/room-1/tasks", func(c *Client) error {
			_, err := c.CreateRoomTask(ctx, "room-1", apitypes.CreateRoomTaskRequest{Title: "Build", CreatedBy: "admin", SourceMessageID: "message-1"})
			return err
		}},
		{"plan", `{}`, "POST /api/v1/rooms/room-1/tasks/task-1/plan", func(c *Client) error { _, err := c.PlanRoomTask(ctx, "room-1", "task-1"); return err }},
		{"start", `{}`, "POST /api/v1/rooms/room-1/tasks/task-1/start", func(c *Client) error { _, err := c.StartRoomTask(ctx, "room-1", "task-1"); return err }},
		{"dispatch", `{}`, "POST /api/v1/rooms/room-1/tasks/task-2/dispatch", func(c *Client) error { _, err := c.DispatchRoomTask(ctx, "room-1", "task-2", "worker"); return err }},
		{"claim", `{}`, "POST /api/v1/rooms/room-1/tasks/task-2/claim", func(c *Client) error { _, err := c.ClaimRoomTask(ctx, "room-1", "task-2", "worker", 1); return err }},
		{"update", `{}`, "PATCH /api/v1/rooms/room-1/tasks/task-2", func(c *Client) error {
			_, err := c.UpdateRoomTask(ctx, "room-1", "task-2", "worker", apitypes.UpdateRoomTaskRequest{Attempt: 1, Status: "completed", Result: "Artifact"})
			return err
		}},
		{"review", `{}`, "POST /api/v1/rooms/room-1/tasks/task-2/review", func(c *Client) error {
			_, err := c.ReviewRoomTask(ctx, "room-1", "task-2", apitypes.ReviewRoomTaskRequest{Attempt: 1, Accept: true, Summary: "Verified"})
			return err
		}},
		{"report", `{}`, "POST /api/v1/rooms/room-1/tasks/task-1/report", func(c *Client) error {
			_, err := c.ReportRoomTask(ctx, "room-1", "task-1", "issues", "Tests found defects")
			return err
		}},
		{"retry", `[]`, "POST /api/v1/rooms/room-1/tasks/retry-delivery", func(c *Client) error { _, err := c.RetryRoomDelivery(ctx, "room-1"); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordingHTTPClient{body: tt.body}
			if err := tt.call(New("http://example.test", "", rec)); err != nil {
				t.Fatal(err)
			}
			if len(rec.requests) != 1 || rec.requests[0] != tt.route {
				t.Fatalf("requests = %v, want %s", rec.requests, tt.route)
			}
		})
	}
}
