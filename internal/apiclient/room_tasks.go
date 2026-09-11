package apiclient

import (
	"context"
	"net/http"
	"net/url"

	"csgclaw/internal/apitypes"
)

func (c *Client) ReportRoomTask(ctx context.Context, roomID, taskID, outcome, summary string) (apitypes.RoomTask, error) {
	var result apitypes.RoomTask
	req := struct {
		Outcome string `json:"outcome"`
		Summary string `json:"summary"`
	}{outcome, summary}
	err := c.DoJSON(ctx, http.MethodPost, roomTasksPath(roomID)+"/"+url.PathEscape(taskID)+"/report", req, &result)
	return result, err
}
func (c *Client) RetryRoomDelivery(ctx context.Context, roomID string) ([]apitypes.RoomTask, error) {
	var result []apitypes.RoomTask
	err := c.DoJSON(ctx, http.MethodPost, roomTasksPath(roomID)+"/retry-delivery", struct{}{}, &result)
	return result, err
}

func roomTasksPath(roomID string) string { return "/api/v1/rooms/" + url.PathEscape(roomID) + "/tasks" }

func (c *Client) GetRoomTask(ctx context.Context, roomID, taskID string) (apitypes.RoomTask, error) {
	var result apitypes.RoomTask
	err := c.GetJSON(ctx, roomTasksPath(roomID)+"/"+url.PathEscape(taskID), &result)
	return result, err
}

func (c *Client) SendRoomTaskMessage(ctx context.Context, roomID, taskID string, req apitypes.RoomTaskMessageRequest) (map[string]any, error) {
	var result map[string]any
	err := c.DoJSON(ctx, http.MethodPost, roomTasksPath(roomID)+"/"+url.PathEscape(taskID)+"/messages", req, &result)
	return result, err
}

func (c *Client) RoomTaskContext(ctx context.Context, roomID string) (map[string]any, error) {
	var result map[string]any
	err := c.GetJSON(ctx, "/api/v1/rooms/"+url.PathEscape(roomID)+"/task-context", &result)
	return result, err
}
func (c *Client) ListRoomTasks(ctx context.Context, roomID string) ([]apitypes.RoomTask, error) {
	var result []apitypes.RoomTask
	err := c.GetJSON(ctx, roomTasksPath(roomID), &result)
	return result, err
}
func (c *Client) CreateRoomTask(ctx context.Context, roomID string, req apitypes.CreateRoomTaskRequest) (apitypes.RoomTask, error) {
	var result apitypes.RoomTask
	err := c.DoJSON(ctx, http.MethodPost, roomTasksPath(roomID), req, &result)
	return result, err
}
func (c *Client) PlanRoomTask(ctx context.Context, roomID, taskID string, plans ...apitypes.PlanRoomTaskRequest) (apitypes.PlanRoomTaskResponse, error) {
	var result apitypes.PlanRoomTaskResponse
	req := apitypes.PlanRoomTaskRequest{AutoStart: true}
	if len(plans) > 0 {
		req = plans[0]
	}
	err := c.DoJSON(ctx, http.MethodPost, roomTasksPath(roomID)+"/"+url.PathEscape(taskID)+"/plan", req, &result)
	return result, err
}
func (c *Client) ClaimRoomTask(ctx context.Context, roomID, taskID string, attempt int) (apitypes.RoomTask, error) {
	var result apitypes.RoomTask
	err := c.DoJSON(ctx, http.MethodPost, roomTasksPath(roomID)+"/"+url.PathEscape(taskID)+"/claim", apitypes.ClaimRoomTaskRequest{Attempt: attempt}, &result)
	return result, err
}
func (c *Client) UpdateRoomTask(ctx context.Context, roomID, taskID string, req apitypes.UpdateRoomTaskRequest) (apitypes.RoomTask, error) {
	var result apitypes.RoomTask
	err := c.DoJSON(ctx, http.MethodPatch, roomTasksPath(roomID)+"/"+url.PathEscape(taskID), req, &result)
	return result, err
}

func (c *Client) DispatchRoomTask(ctx context.Context, roomID, taskID, assignee string) (apitypes.RoomTask, error) {
	var result apitypes.RoomTask
	err := c.DoJSON(ctx, http.MethodPost, roomTasksPath(roomID)+"/"+url.PathEscape(taskID)+"/dispatch", map[string]string{"assigned_to": assignee}, &result)
	return result, err
}
func (c *Client) ReviewRoomTask(ctx context.Context, roomID, taskID string, req apitypes.ReviewRoomTaskRequest) (apitypes.RoomTask, error) {
	var result apitypes.RoomTask
	err := c.DoJSON(ctx, http.MethodPost, roomTasksPath(roomID)+"/"+url.PathEscape(taskID)+"/review", req, &result)
	return result, err
}
func (c *Client) StartRoomTask(ctx context.Context, roomID, taskID string) (apitypes.StartRoomTaskResponse, error) {
	var result apitypes.StartRoomTaskResponse
	err := c.DoJSON(ctx, http.MethodPost, roomTasksPath(roomID)+"/"+url.PathEscape(taskID)+"/start", struct{}{}, &result)
	return result, err
}

func (c *Client) StopRoomTask(ctx context.Context, roomID, taskID string) (apitypes.RoomTask, error) {
	var result apitypes.RoomTask
	err := c.DoJSON(ctx, http.MethodPost, roomTasksPath(roomID)+"/"+url.PathEscape(taskID)+"/stop", struct{}{}, &result)
	return result, err
}
func (c *Client) RecoverRoomTask(ctx context.Context, roomID, taskID string, attempt int, assessment string) (apitypes.RoomTask, error) {
	var result apitypes.RoomTask
	req := struct {
		Attempt    int    `json:"attempt"`
		Assessment string `json:"assessment"`
	}{attempt, assessment}
	err := c.DoJSON(ctx, http.MethodPost, roomTasksPath(roomID)+"/"+url.PathEscape(taskID)+"/recover", req, &result)
	return result, err
}
