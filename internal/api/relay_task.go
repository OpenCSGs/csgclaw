package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
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
			if errors.As(err, &remote) && remote.Detail.Code == "response_not_found" {
				code = "relay_task_result_unavailable"
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
	if code == "relay_task_result_unavailable" {
		if locale == "en" {
			return "Relay could not confirm the result of this message. Check whether it completed before sending it again."
		}
		return "Relay 无法确认这条消息的执行结果。请确认消息是否已执行，再决定是否重发。"
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

// Build a text checkpoint from successful turns in this room. Replies can be
// appended after the next user message when several messages are queued.
func cloudTaskRecoveryInput(room im.Room, message im.Message) string {
	replies := make(map[string]string)
	for _, reply := range room.Messages {
		if id, _ := reply.Metadata["relay_response_id"].(string); reply.SenderID == room.CloudAgent.UserID && id != "" {
			replies[reply.ClientMessageID] = reply.Content
		}
	}
	budget := 16 << 10
	var turns []string
	for i := slices.IndexFunc(room.Messages, func(m im.Message) bool { return m.ID == message.ID }) - 1; i >= 0; i-- {
		prior := room.Messages[i]
		reply, ok := replies["relay-reply:"+prior.ID]
		if prior.SenderID != room.CreatedBy || prior.Kind == im.MessageKindEvent || !ok {
			continue
		}
		if len(prior.Content)+len(reply) >= budget {
			break
		}
		turn, _ := json.Marshal(struct {
			User  string `json:"user"`
			Agent string `json:"agent"`
		}{prior.Content, reply})
		budget -= len(turn) + 1
		if budget < 0 {
			break
		}
		turns = append(turns, string(turn))
	}
	if len(turns) == 0 {
		return message.Content
	}
	slices.Reverse(turns)
	input := "The remote session context was lost. Use the following completed turns as background. Do not repeat prior actions. Respond to the current user message.\n[" +
		strings.Join(turns, ",") + "]\n\nCurrent user message:\n" + message.Content
	// Leave room for the Relay request envelope when the current message
	// alone is already close to the request size limit.
	if encoded, _ := json.Marshal(input); len(encoded) > (1<<20)-1024 {
		return message.Content
	}
	return input
}

func cloudTaskCanRestoreContext(result relay.Response, err error, responseID string) bool {
	if err == nil {
		return result.Status == "failed" && result.Error != nil && result.Error.Code == "relay_branch_unsupported"
	}
	// A missing result for a known request is ambiguous. Only retry a POST
	// that Relay rejected before it assigned the new request a response ID.
	var remote *relay.HTTPError
	return responseID == "" && errors.As(err, &remote) && remote.Status == http.StatusNotFound &&
		remote.Detail.Code == "response_not_found" && remote.Detail.ResponseID == ""
}

// Poll known requests through temporary outages. Submission retries always keep
// the original key; never turn a transport failure into a completed job.
func (h *Handler) awaitCloudTask(ctx context.Context, room im.Room, message im.Message, previous string) (relay.Response, error) {
	metadata := maps.Clone(message.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	responseID, _ := metadata["relay_pending_response_id"].(string)
	recoveryInput, _ := metadata["relay_context_recovery_input"].(string)
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
			key, input, parent := "task:"+room.ID+":"+message.ID, message.Content, previous
			if recoveryInput != "" {
				key, input, parent = "task-recovery:"+room.ID+":"+message.ID, recoveryInput, ""
			}
			result, err = h.relayManager.Chat(callCtx, room.CloudAgent.AgentID, room.CloudAgent.ConnectionID, key, input, parent)
			var remote *relay.HTTPError
			if errors.As(err, &remote) && remote.Detail.ResponseID != "" && (remote.Detail.Code == "response_in_progress" || remote.Detail.Code == "delivery_confirmation_timeout" || remote.Detail.Code == "relay_unavailable") {
				result.ID, result.Status, err = remote.Detail.ResponseID, "in_progress", nil
			}
		}
		cancel()
		if recoveryInput == "" && previous != "" && cloudTaskCanRestoreContext(result, err, responseID) {
			// Save the exact replacement input and switch attempts atomically
			// before submitting work under a different idempotency key.
			recoveryInput = cloudTaskRecoveryInput(room, message)
			metadata["relay_context_recovery_input"] = recoveryInput
			delete(metadata, "relay_pending_response_id")
			if _, saveErr := h.im.UpdateDeliveredMessageMetadata(room.ID, message.ID, message.SenderID, metadata); saveErr != nil {
				return result, errCloudTaskPersistence
			}
			responseID = ""
			continue
		}
		if result.ID != "" && result.ID != responseID {
			metadata["relay_pending_response_id"] = result.ID
			if _, saveErr := h.im.UpdateDeliveredMessageMetadata(room.ID, message.ID, message.SenderID, metadata); saveErr != nil {
				return result, errCloudTaskPersistence
			}
			responseID = result.ID
		}
		if err == nil && result.Status != "in_progress" && result.Status != "queued" {
			return result, nil
		}
		if err != nil && !cloudTaskRetryable(err, recoveryInput != "" && responseID == "") {
			return result, err
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func cloudTaskRetryable(err error, submittingRecovery bool) bool {
	if errors.Is(err, relay.ErrChatReference) {
		return false
	}
	var remote *relay.HTTPError
	if errors.As(err, &remote) {
		// A rejected continuation can be visible before Relay releases the
		// agent. Give the recovery submission time to acquire that slot.
		if submittingRecovery && remote.Status == http.StatusConflict && remote.Detail.Code == "agent_busy" {
			return true
		}
		return remote.Status >= http.StatusInternalServerError || remote.Status == http.StatusRequestTimeout || remote.Status == http.StatusTooManyRequests
	}
	return true
}
