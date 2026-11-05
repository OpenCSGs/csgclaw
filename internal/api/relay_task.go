package api

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"time"
	"unicode"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/relay"
)

func (h *Handler) prepareCloudTask(ctx context.Context, req *apitypes.CreateRoomRequest) error {
	if h.relayManager == nil || h.im == nil {
		return relay.ErrChatReference
	}
	entry, err := h.relayManager.DiscoverAgent(ctx, req.CloudAgent.AgentID, req.CloudAgent.ConnectionID)
	if err != nil {
		return err
	}
	if entry.Agent.Status != "online" {
		return fmt.Errorf("cloud agent is offline")
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(entry.ConnectionID+":"+entry.Agent.ID)))[:16]
	display := []rune{}
	for _, r := range entry.Agent.Name {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || (len(display) > 0 && (r == '-' || r == '_' || r == '.')) {
			display = append(display, r)
			if len(display) == 16 {
				break
			}
		}
	}
	if len(display) == 0 {
		display = []rune("agent")
	}
	user, _, err := h.im.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "relay-" + digest, Name: string(display) + "-cloud-" + digest[:8], Description: entry.Agent.Description, Avatar: entry.Avatar, Role: "cloud", SkipDirectRoom: true})
	if err != nil {
		return err
	}
	h.publishUserEvent(im.EventTypeUserCreated, user)
	req.CloudAgent = &apitypes.CloudAgentReference{AgentID: entry.Agent.ID, ConnectionID: entry.ConnectionID, UserID: user.ID}
	req.MemberIDs = []string{user.ID}
	req.Type, req.IsDirect, req.TaskSession = apitypes.RoomTypeFree, true, true
	req.Configuration, req.ManagerID, req.SourceTeamID, req.CollaborationGuide = nil, "", "", ""
	return nil
}

// The persisted user message is the job. A deterministic reply ID and Relay key
// make duplicate sends and restarts safe without a second queue or credential copy.
func (h *Handler) startCloudTask(roomID string) {
	h.cloudTaskMu.Lock()
	defer h.cloudTaskMu.Unlock()
	h.startCloudTaskLocked(roomID)
}

// Caller serializes this with cloud room management and message persistence.
func (h *Handler) startCloudTaskLocked(roomID string) {
	if h.cloudTasks == nil {
		h.cloudTasks = map[string]bool{}
	}
	if h.cloudTasks[roomID] || h.relayManager == nil || (h.relayContext != nil && h.relayContext.Err() != nil) {
		return
	}
	h.cloudTasks[roomID] = true
	h.cloudTaskWG.Add(1)
	go func() { defer h.cloudTaskWG.Done(); h.runCloudTask(roomID) }()
}

func cloudTaskPending(room im.Room) (im.Message, string, bool) {
	previous := ""
	replied := map[string]bool{}
	for _, message := range room.Messages {
		if message.SenderID == room.CloudAgent.UserID {
			replied[message.ClientMessageID] = true
			if id, ok := message.Metadata["relay_response_id"].(string); ok && id != "" {
				previous = id
			}
		}
	}
	for _, message := range room.Messages {
		if message.SenderID == room.CreatedBy && message.Kind != im.MessageKindEvent && !replied["relay-reply:"+message.ID] {
			return message, previous, true
		}
	}
	return im.Message{}, previous, false
}

func (h *Handler) runCloudTask(roomID string) {
	for {
		h.cloudTaskMu.Lock()
		room, found := h.im.Room(roomID)
		var message im.Message
		var previous string
		pending := false
		if found && room.CloudAgent != nil {
			message, previous, pending = cloudTaskPending(room)
		}
		if !pending {
			delete(h.cloudTasks, roomID)
			h.cloudTaskMu.Unlock()
			return
		}
		h.cloudTaskMu.Unlock()

		parent := h.relayContext
		if parent == nil {
			parent = context.Background()
		}
		result, err := h.awaitCloudTask(parent, room, message, previous)
		if parent.Err() != nil || errors.Is(err, errCloudTaskPersistence) {
			h.cloudTaskMu.Lock()
			delete(h.cloudTasks, roomID)
			h.cloudTaskMu.Unlock()
			return
		}

		content := result.Text()
		metadata := map[string]any{}
		if err != nil || result.Status != "completed" {
			code := "execution_failed"
			if result.Error != nil && result.Error.Code == "model_protocol_unsupported" {
				code = result.Error.Code
			}
			var remote *relay.HTTPError
			if previous != "" && errors.As(err, &remote) && remote.Detail.Code == "response_not_found" {
				code = "relay_task_context_expired"
			}
			content = cloudTaskFailureText(room.Locale, code)
			metadata["relay_error_code"] = code
			metadata["relay_task_failed"] = true
		} else {
			metadata["relay_response_id"] = result.ID
			if content == "" {
				content = "(empty response)"
			}
		}
		reply, created, saveErr := h.im.CreateMessageOnce(im.CreateMessageRequest{RoomID: roomID, SenderID: room.CloudAgent.UserID, Content: content, ClientMessageID: "relay-reply:" + message.ID, Metadata: metadata, Locale: room.Locale})
		if saveErr != nil {
			h.cloudTaskMu.Lock()
			delete(h.cloudTasks, roomID)
			h.cloudTaskMu.Unlock()
			return
		}
		if created {
			h.publishMessageCreated(roomID, reply.SenderID, reply)
		}
	}
}

// Use trusted local text instead of exposing arbitrary upstream diagnostics.
func cloudTaskFailureText(locale, code string) string {
	if code == "relay_task_context_expired" {
		if locale == "en" {
			return "This conversation can no longer continue. Start a new cloud task to chat again."
		}
		return "当前对话已无法继续，请点击“新建任务”重新与该云端智能体聊天。"
	}

	if code == "model_protocol_unsupported" {
		if locale == "en" {
			return "The published agent's model does not support the Responses tool protocol required by its runtime. Select a compatible provider/model in the source agent's settings, then send again."
		}
		return "发布端智能体的模型不支持运行时所需的 Responses 工具协议。请在本地源智能体的设置中选择支持该协议的模型服务，再重新发送。"
	}
	if locale == "en" {
		return "Cloud task could not complete. Check Relay and agent availability before sending again."
	}
	return "云端任务未能完成，请检查 Relay 和智能体状态后重新发送。"
}

var errCloudTaskPersistence = errors.New("cloud task recovery state could not be saved")

// Poll known requests through temporary outages. Submission retries always keep
// the original key; never turn a transport failure into a completed job.
func (h *Handler) awaitCloudTask(ctx context.Context, room im.Room, message im.Message, previous string) (relay.Response, error) {
	responseID, _ := message.Metadata["relay_pending_response_id"].(string)
	for {
		if ctx.Err() != nil {
			return relay.Response{}, ctx.Err()
		}
		if responseID == "" && time.Since(message.CreatedAt) > 10*time.Minute {
			return relay.Response{}, fmt.Errorf("request recovery window expired")
		}
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		var result relay.Response
		var err error
		if responseID != "" {
			result, err = h.relayManager.ChatResponse(callCtx, room.CloudAgent.AgentID, room.CloudAgent.ConnectionID, responseID)
		} else {
			result, err = h.relayManager.Chat(callCtx, room.CloudAgent.AgentID, room.CloudAgent.ConnectionID, "task:"+room.ID+":"+message.ID, message.Content, previous)
			var remote *relay.HTTPError
			if errors.As(err, &remote) && remote.Detail.ResponseID != "" && (remote.Detail.Code == "response_in_progress" || remote.Detail.Code == "delivery_confirmation_timeout" || remote.Detail.Code == "relay_unavailable") {
				result.ID, result.Status, err = remote.Detail.ResponseID, "in_progress", nil
			}
		}
		cancel()
		if result.ID != "" && result.ID != responseID {
			metadata := maps.Clone(message.Metadata)
			if metadata == nil {
				metadata = map[string]any{}
			}
			metadata["relay_pending_response_id"] = result.ID
			if _, saveErr := h.im.UpdateDeliveredMessageMetadata(room.ID, message.ID, message.SenderID, metadata); saveErr != nil {
				return result, errCloudTaskPersistence
			}
			responseID = result.ID
		}
		if err == nil && result.Status != "in_progress" && result.Status != "queued" {
			return result, nil
		}
		if err != nil && !cloudTaskRetryable(err) {
			return result, err
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func cloudTaskRetryable(err error) bool {
	if errors.Is(err, relay.ErrChatReference) {
		return false
	}
	var remote *relay.HTTPError
	if errors.As(err, &remote) {
		return remote.Status >= http.StatusInternalServerError || remote.Status == http.StatusRequestTimeout || remote.Status == http.StatusTooManyRequests
	}
	return true
}
