package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	"csgclaw/internal/participant"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	endpoint string
	token    string
	client   HTTPClient
}

func DefaultAPIBaseURL() string {
	return config.DefaultAPIBaseURL()
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

func (c *Client) ListParticipants(ctx context.Context, channel, typ, agentID string) ([]apitypes.Participant, error) {
	var participants []apitypes.Participant
	values := url.Values{}
	if strings.TrimSpace(typ) != "" {
		values.Set("type", strings.TrimSpace(typ))
	}
	if strings.TrimSpace(agentID) != "" {
		values.Set("agent_id", strings.TrimSpace(agentID))
	}
	path, err := participantCollectionPath(channel)
	if err != nil {
		return nil, err
	}
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	if err := c.GetJSON(ctx, path, &participants); err != nil {
		return nil, err
	}
	return participants, nil
}

func (c *Client) ListAgents(ctx context.Context) ([]apitypes.Agent, error) {
	var agents []apitypes.Agent
	if err := c.GetJSON(ctx, "/api/v1/agents", &agents); err != nil {
		return nil, err
	}
	return agents, nil
}

func (c *Client) GetAgent(ctx context.Context, id string) (apitypes.Agent, error) {
	var got apitypes.Agent
	id = strings.TrimSpace(id)
	if id == "" {
		return apitypes.Agent{}, fmt.Errorf("agent id is required")
	}
	if err := c.GetJSON(ctx, "/api/v1/agents/"+url.PathEscape(id), &got); err != nil {
		return apitypes.Agent{}, err
	}
	return got, nil
}

func (c *Client) CreateParticipant(ctx context.Context, req participant.CreateRequest) (apitypes.Participant, error) {
	var created apitypes.Participant
	path, err := participantCollectionPath(req.Channel)
	if err != nil {
		return apitypes.Participant{}, err
	}
	if err := c.DoJSON(ctx, http.MethodPost, path, req, &created); err != nil {
		return apitypes.Participant{}, err
	}
	return created, nil
}

func (c *Client) UpdateParticipant(ctx context.Context, channel, id string, req participant.UpdateRequest) (apitypes.Participant, error) {
	var updated apitypes.Participant
	path, err := participantItemPath(channel, id)
	if err != nil {
		return apitypes.Participant{}, err
	}
	if err := c.DoJSON(ctx, http.MethodPatch, path, req, &updated); err != nil {
		return apitypes.Participant{}, err
	}
	return updated, nil
}

func (c *Client) DeleteParticipant(ctx context.Context, channel, id, deleteAgent string) error {
	path, err := participantItemPath(channel, id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(deleteAgent) != "" {
		values := url.Values{}
		values.Set("delete_agent", strings.TrimSpace(deleteAgent))
		path += "?" + values.Encode()
	}
	return c.DoNoContent(ctx, http.MethodDelete, path)
}

func (c *Client) RecreateAgent(ctx context.Context, id string) (apitypes.Agent, error) {
	var recreated apitypes.Agent
	id = strings.TrimSpace(id)
	if id == "" {
		return apitypes.Agent{}, fmt.Errorf("agent id is required")
	}
	if err := c.DoJSON(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(id)+"/recreate", nil, &recreated); err != nil {
		return apitypes.Agent{}, err
	}
	return recreated, nil
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

func (c *Client) ClearRoomMessages(ctx context.Context, roomID string) (apitypes.Room, error) {
	var room apitypes.Room
	path, err := roomClearMessagesPath(roomID)
	if err != nil {
		return apitypes.Room{}, err
	}
	if err := c.DoJSON(ctx, http.MethodPost, path, nil, &room); err != nil {
		return apitypes.Room{}, err
	}
	return room, nil
}

func (c *Client) ListUsers(ctx context.Context) ([]apitypes.User, error) {
	return c.ListUsersByChannel(ctx, "csgclaw")
}

func (c *Client) ListUsersByChannel(ctx context.Context, channel string) ([]apitypes.User, error) {
	var users []apitypes.User
	if strings.TrimSpace(channel) == "" {
		channel = "csgclaw"
	}
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
	if strings.TrimSpace(channel) == "" {
		channel = "csgclaw"
	}
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

func (c *Client) ListTeams(ctx context.Context) ([]apitypes.Team, error) {
	var teams []apitypes.Team
	if err := c.GetJSON(ctx, "/api/v1/teams", &teams); err != nil {
		return nil, err
	}
	return teams, nil
}

func (c *Client) CreateTeam(ctx context.Context, req apitypes.CreateTeamRequest) (apitypes.Team, error) {
	var created apitypes.Team
	if err := c.DoJSON(ctx, http.MethodPost, "/api/v1/teams", req, &created); err != nil {
		return apitypes.Team{}, err
	}
	return created, nil
}

func (c *Client) UpdateTeam(ctx context.Context, teamID string, req apitypes.PatchTeamRequest) (apitypes.Team, error) {
	var updated apitypes.Team
	path, err := teamBasePath(teamID)
	if err != nil {
		return apitypes.Team{}, err
	}
	if err := c.DoJSON(ctx, http.MethodPatch, path, req, &updated); err != nil {
		return apitypes.Team{}, err
	}
	return updated, nil
}

func (c *Client) DeleteTeam(ctx context.Context, teamID string) error {
	path, err := teamBasePath(teamID)
	if err != nil {
		return err
	}
	return c.DoNoContent(ctx, http.MethodDelete, path)
}

func (c *Client) ListGlobalTasks(ctx context.Context) ([]apitypes.GlobalTask, error) {
	var tasks []apitypes.GlobalTask
	if err := c.GetJSON(ctx, "/api/v1/tasks", &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (c *Client) ListTeamTasks(ctx context.Context, teamID string) ([]apitypes.TeamTask, error) {
	var tasks []apitypes.TeamTask
	path, err := teamTasksPath(teamID)
	if err != nil {
		return nil, err
	}
	if err := c.GetJSON(ctx, path, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (c *Client) CreateTeamTasksBatch(ctx context.Context, teamID string, req apitypes.CreateTeamTasksBatchRequest) (apitypes.CreateTeamTasksBatchResponse, error) {
	var created apitypes.CreateTeamTasksBatchResponse
	path, err := teamTasksBatchPath(teamID)
	if err != nil {
		return apitypes.CreateTeamTasksBatchResponse{}, err
	}
	if err := c.DoJSON(ctx, http.MethodPost, path, req, &created); err != nil {
		return apitypes.CreateTeamTasksBatchResponse{}, err
	}
	return created, nil
}

func (c *Client) PlanTeamTask(ctx context.Context, teamID, taskID, actorID string, autoStart bool) (apitypes.PlanTeamTaskResponse, error) {
	var planned apitypes.PlanTeamTaskResponse
	path, err := teamTaskPlanPath(teamID, taskID)
	if err != nil {
		return apitypes.PlanTeamTaskResponse{}, err
	}
	req := apitypes.PlanTeamTaskRequest{
		ActorID:   strings.TrimSpace(actorID),
		AutoStart: autoStart,
	}
	if err := c.DoJSON(ctx, http.MethodPost, path, req, &planned); err != nil {
		return apitypes.PlanTeamTaskResponse{}, err
	}
	return planned, nil
}

func (c *Client) ClaimNextTeamTask(ctx context.Context, req apitypes.ClaimNextTeamTaskRequest) (apitypes.TeamTask, error) {
	var task apitypes.TeamTask
	path, err := teamClaimNextPath(req.TeamID)
	if err != nil {
		return apitypes.TeamTask{}, err
	}
	if err := c.DoJSON(ctx, http.MethodPost, path, req, &task); err != nil {
		return apitypes.TeamTask{}, err
	}
	return task, nil
}

func (c *Client) ClaimTeamTask(ctx context.Context, teamID, taskID, participantID string) (apitypes.TeamTask, error) {
	var task apitypes.TeamTask
	path, err := teamTaskClaimPath(teamID, taskID)
	if err != nil {
		return apitypes.TeamTask{}, err
	}
	if err := c.DoJSON(ctx, http.MethodPost, path, apitypes.ClaimTeamTaskRequest{ParticipantID: strings.TrimSpace(participantID)}, &task); err != nil {
		return apitypes.TeamTask{}, err
	}
	return task, nil
}

func (c *Client) UpdateTeamTask(ctx context.Context, teamID, taskID, actorID string, req apitypes.PatchTeamTaskRequest) (apitypes.TeamTask, error) {
	var updated apitypes.TeamTask
	path, err := teamTaskPath(teamID, taskID)
	if err != nil {
		return apitypes.TeamTask{}, err
	}
	body := struct {
		apitypes.PatchTeamTaskRequest
		ActorID string `json:"actor_id"`
	}{
		PatchTeamTaskRequest: req,
		ActorID:              strings.TrimSpace(actorID),
	}
	if err := c.DoJSON(ctx, http.MethodPatch, path, body, &updated); err != nil {
		return apitypes.TeamTask{}, err
	}
	return updated, nil
}

func (c *Client) AssignTeamTask(ctx context.Context, teamID, taskID, actorID, participantID string) (apitypes.TeamTask, error) {
	var updated apitypes.TeamTask
	path, err := teamTaskAssignPath(teamID, taskID)
	if err != nil {
		return apitypes.TeamTask{}, err
	}
	body := struct {
		apitypes.AssignTeamTaskRequest
		ActorID string `json:"actor_id"`
	}{
		AssignTeamTaskRequest: apitypes.AssignTeamTaskRequest{ParticipantID: strings.TrimSpace(participantID)},
		ActorID:               strings.TrimSpace(actorID),
	}
	if err := c.DoJSON(ctx, http.MethodPost, path, body, &updated); err != nil {
		return apitypes.TeamTask{}, err
	}
	return updated, nil
}

func (c *Client) StartTeamTask(ctx context.Context, teamID, taskID, actorID string) (apitypes.StartTeamTaskResponse, error) {
	var started apitypes.StartTeamTaskResponse
	path, err := teamTaskStartPath(teamID, taskID)
	if err != nil {
		return apitypes.StartTeamTaskResponse{}, err
	}
	req := apitypes.StartTeamTaskRequest{ActorID: strings.TrimSpace(actorID)}
	if err := c.DoJSON(ctx, http.MethodPost, path, req, &started); err != nil {
		return apitypes.StartTeamTaskResponse{}, err
	}
	return started, nil
}

func (c *Client) ListTeamApprovals(ctx context.Context, teamID string) ([]apitypes.TeamApproval, error) {
	var approvals []apitypes.TeamApproval
	path, err := teamApprovalsPath(teamID)
	if err != nil {
		return nil, err
	}
	if err := c.GetJSON(ctx, path, &approvals); err != nil {
		return nil, err
	}
	return approvals, nil
}

func (c *Client) CreateTeamApproval(ctx context.Context, teamID string, req apitypes.CreateTeamApprovalRequest) (apitypes.TeamApproval, error) {
	var created apitypes.TeamApproval
	path, err := teamApprovalsPath(teamID)
	if err != nil {
		return apitypes.TeamApproval{}, err
	}
	if err := c.DoJSON(ctx, http.MethodPost, path, req, &created); err != nil {
		return apitypes.TeamApproval{}, err
	}
	return created, nil
}

func (c *Client) ResolveTeamApproval(ctx context.Context, teamID, approvalID string, req apitypes.ResolveTeamApprovalRequest) (apitypes.TeamApproval, error) {
	var resolved apitypes.TeamApproval
	path, err := teamApprovalResolvePath(teamID, approvalID)
	if err != nil {
		return apitypes.TeamApproval{}, err
	}
	if err := c.DoJSON(ctx, http.MethodPost, path, req, &resolved); err != nil {
		return apitypes.TeamApproval{}, err
	}
	return resolved, nil
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

func participantCollectionPath(channelName string) (string, error) {
	channelName = strings.ToLower(strings.TrimSpace(channelName))
	if channelName == "" {
		channelName = "csgclaw"
	}
	switch channelName {
	case "csgclaw", "feishu":
		return "/api/v1/channels/" + channelName + "/participants", nil
	default:
		return "", fmt.Errorf("unsupported channel %q", channelName)
	}
}

func participantItemPath(channelName, participantID string) (string, error) {
	path, err := participantCollectionPath(channelName)
	if err != nil {
		return "", err
	}
	participantID = strings.TrimSpace(participantID)
	if participantID == "" {
		return "", fmt.Errorf("participant id is required")
	}
	return path + "/" + url.PathEscape(participantID), nil
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

func roomClearMessagesPath(roomID string) (string, error) {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return "", fmt.Errorf("room_id is required")
	}
	return "/api/v1/rooms/" + url.PathEscape(roomID) + ":clearMessages", nil
}

func userDeletePath(channelName, userID string) (string, error) {
	channelName = strings.ToLower(strings.TrimSpace(channelName))
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", fmt.Errorf("user id is required")
	}
	switch channelName {
	case "":
		return "/api/v1/channels/csgclaw/users/" + url.PathEscape(userID), nil
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

func teamBasePath(teamID string) (string, error) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return "", fmt.Errorf("team_id is required")
	}
	return "/api/v1/teams/" + url.PathEscape(teamID), nil
}

func teamTasksBatchPath(teamID string) (string, error) {
	path, err := teamTasksPath(teamID)
	if err != nil {
		return "", err
	}
	return path + "/batch", nil
}

func teamTasksPath(teamID string) (string, error) {
	path, err := teamBasePath(teamID)
	if err != nil {
		return "", err
	}
	return path + "/tasks", nil
}

func teamClaimNextPath(teamID string) (string, error) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return "/api/v1/teams/tasks/claim-next", nil
	}
	path, err := teamBasePath(teamID)
	if err != nil {
		return "", err
	}
	return path + "/tasks/claim-next", nil
}

func teamTaskClaimPath(teamID, taskID string) (string, error) {
	path, err := teamTaskPath(teamID, taskID)
	if err != nil {
		return "", err
	}
	return path + "/claim", nil
}

func teamTaskPlanPath(teamID, taskID string) (string, error) {
	path, err := teamTaskPath(teamID, taskID)
	if err != nil {
		return "", err
	}
	return path + "/plan", nil
}

func teamTaskPath(teamID, taskID string) (string, error) {
	path, err := teamBasePath(teamID)
	if err != nil {
		return "", err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}
	return path + "/tasks/" + url.PathEscape(taskID), nil
}

func teamTaskAssignPath(teamID, taskID string) (string, error) {
	path, err := teamTaskPath(teamID, taskID)
	if err != nil {
		return "", err
	}
	return path + "/assign", nil
}

func teamTaskStartPath(teamID, taskID string) (string, error) {
	path, err := teamTaskPath(teamID, taskID)
	if err != nil {
		return "", err
	}
	return path + "/start", nil
}

func teamApprovalsPath(teamID string) (string, error) {
	path, err := teamBasePath(teamID)
	if err != nil {
		return "", err
	}
	return path + "/approvals", nil
}

func teamApprovalResolvePath(teamID, approvalID string) (string, error) {
	path, err := teamApprovalsPath(teamID)
	if err != nil {
		return "", err
	}
	approvalID = strings.TrimSpace(approvalID)
	if approvalID == "" {
		return "", fmt.Errorf("approval_id is required")
	}
	return path + "/" + url.PathEscape(approvalID) + "/resolve", nil
}
