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
			if errors.Is(err, relay.ErrRequestTooLarge) {
				code = "relay_task_input_too_large"
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
	var en, zh string
	switch code {
	case "relay_task_input_too_large":
		en = "This message exceeds the cloud task request size limit. Shorten the message and send it again."
		zh = "这条消息超过了云端任务的请求大小限制，请缩短消息后再发送。"
	case "relay_task_result_unavailable":
		en = "Relay could not confirm the result of this message. Check whether it completed before sending it again."
		zh = "Relay 无法确认这条消息的执行结果。请确认消息是否已执行，再决定是否重发。"
	case "model_protocol_unsupported":
		en = "The published agent's model does not support the Responses tool protocol required by its runtime. Select a compatible provider/model in the source agent's settings, then send again."
		zh = "发布端智能体的模型不支持运行时所需的 Responses 工具协议。请在本地源智能体的设置中选择支持该协议的模型服务，再重新发送。"
	default:
		en = "Cloud task could not complete. Check Relay and agent availability before sending again."
		zh = "云端任务未能完成，请检查 Relay 和智能体状态后重新发送。"
	}
	if locale == "en" {
		return en
	}
	return zh
}

var errCloudTaskPersistence = errors.New("cloud task recovery state could not be saved")

const (
	cloudTaskUnsubmittedKey = "relay_task_unsubmitted"
	cloudTaskResponseKey    = "relay_pending_response_id"
	cloudTaskRecoveryKey    = "relay_context_recovery_input"
)

// Build a text checkpoint from successful turns in this room. Replies can be
// appended after the next user message when several messages are queued.
func cloudTaskRecoveryInput(room im.Room, message im.Message) string {
	replies := make(map[string]string)
	for _, reply := range room.Messages {
		if id, _ := reply.Metadata["relay_response_id"].(string); reply.SenderID == room.CloudAgent.UserID && id != "" {
			replies[reply.ClientMessageID] = reply.Content
		}
	}
	budget := (16 << 10) - len("[]")
	var turns []string
	for i := slices.IndexFunc(room.Messages, func(m im.Message) bool { return m.ID == message.ID }) - 1; i >= 0; i-- {
		prior := room.Messages[i]
		reply, ok := replies["relay-reply:"+prior.ID]
		if prior.SenderID != room.CreatedBy || prior.Kind == im.MessageKindEvent || !ok {
			continue
		}
		if len(prior.Content)+len(reply) >= budget {
			continue
		}
		turn, _ := json.Marshal(struct {
			User  string `json:"user"`
			Agent string `json:"agent"`
		}{prior.Content, reply})
		if len(turn)+1 > budget {
			continue
		}
		budget -= len(turn) + 1
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

func cloudTaskCanRestoreContext(result relay.Response, remote *relay.HTTPError, knownUnsubmitted bool) bool {
	if result.Status == "failed" && result.Error != nil && result.Error.Code == "relay_branch_unsupported" {
		return true
	}
	// The same 404 can follow the loss of an earlier submission record.
	// Only allow recovery while every earlier attempt was ruled out.
	return knownUnsubmitted && remote != nil && remote.Status == http.StatusNotFound &&
		remote.Detail.Code == "response_not_found" && remote.Detail.ResponseID == ""
}

func cloudTaskRejectedBeforeExecution(result relay.Response) bool {
	if result.Status != "failed" || result.Error == nil || result.ID == "" {
		return false
	}
	switch result.Error.Code {
	case "admission_failed", "agent_busy", "agent_offline", "agent_unavailable":
		return true
	default:
		return false
	}
}

// Poll known requests by ID and reuse the current key until Relay confirms
// that the attempt did not execute.
func (h *Handler) awaitCloudTask(ctx context.Context, room im.Room, message im.Message, previous string) (relay.Response, error) {
	metadata := maps.Clone(message.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	responseID, _ := metadata[cloudTaskResponseKey].(string)
	recoveryInput, _ := metadata[cloudTaskRecoveryKey].(string)
	startedAtKey, retryAfterKey := "relay_task_started_at", "relay_task_retry_after"
	if recoveryInput != "" {
		startedAtKey, retryAfterKey = "relay_context_recovery_started_at", "relay_context_recovery_after"
	}
	startedAt, _ := metadata[startedAtKey].(string)
	unsubmitted, _ := metadata[cloudTaskUnsubmittedKey].(bool)
	submissionStartedAt, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		submissionStartedAt = message.CreatedAt
		if unsubmitted && recoveryInput == "" && responseID == "" {
			submissionStartedAt = time.Now().UTC()
		}
	}
	// Reconfirm the checkpoint before the first POST after a worker restart.
	needsCheckpoint := true
	checkpoint := func() error {
		needsCheckpoint = false
		_, err := h.im.UpdateDeliveredMessageMetadata(room.ID, message.ID, message.SenderID, metadata)
		return err
	}
	for {
		if ctx.Err() != nil {
			return relay.Response{}, ctx.Err()
		}
		responseID, _ := metadata[cloudTaskResponseKey].(string)
		retryAfter, _ := metadata[retryAfterKey].(string)
		recoveryInput, _ := metadata[cloudTaskRecoveryKey].(string)
		if responseID == "" && time.Since(submissionStartedAt) > 10*time.Minute {
			return relay.Response{}, fmt.Errorf("request recovery window expired")
		}
		unsubmitted, _ := metadata[cloudTaskUnsubmittedKey].(bool)
		knownUnsubmitted := unsubmitted && recoveryInput == "" && responseID == ""
		if knownUnsubmitted {
			// Consume this marker before the POST so a restart cannot mistake
			// an already attempted submission for a new one.
			delete(metadata, cloudTaskUnsubmittedKey)
			metadata[startedAtKey] = submissionStartedAt.Format(time.RFC3339Nano)
			needsCheckpoint = true
		}
		if responseID == "" && needsCheckpoint && checkpoint() != nil {
			return relay.Response{}, errCloudTaskPersistence
		}
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		var result relay.Response
		var err error
		var remote *relay.HTTPError
		if responseID != "" {
			result, err = h.relayManager.ChatResponse(callCtx, room.CloudAgent.AgentID, room.CloudAgent.ConnectionID, responseID)
		} else {
			key, input, parent := "task:"+room.ID+":"+message.ID, message.Content, previous
			if recoveryInput != "" {
				key, input, parent = "task-recovery:"+room.ID+":"+message.ID, recoveryInput, ""
			}
			if retryAfter != "" {
				key = fmt.Sprintf("task-recovery:%x", sha256.Sum256([]byte(key+"\x00"+retryAfter)))
			}
			result, err = h.relayManager.Chat(callCtx, room.CloudAgent.AgentID, room.CloudAgent.ConnectionID, key, input, parent)
			if errors.As(err, &remote) {
				if id := remote.PendingResponseID(); id != "" {
					result.ID, result.Status, err = id, "in_progress", nil
					remote = nil
				}
			}
		}
		cancel()
		restore := recoveryInput == "" && previous != "" && cloudTaskCanRestoreContext(result, remote, knownUnsubmitted)
		retryRejected := err == nil && result.ID != retryAfter && cloudTaskRejectedBeforeExecution(result)
		metadataChanged := true
		switch {
		case restore:
			startedAtKey, retryAfterKey = "relay_context_recovery_started_at", "relay_context_recovery_after"
			submissionStartedAt = time.Now().UTC()
			metadata[cloudTaskRecoveryKey] = cloudTaskRecoveryInput(room, message)
			metadata[startedAtKey] = submissionStartedAt.Format(time.RFC3339Nano)
			delete(metadata, cloudTaskResponseKey)
			delete(metadata, retryAfterKey)
		case retryRejected:
			// Relay and the executor reject these requests before execution.
			// A recorded rejection is terminal for its key, so persist the
			// identity of the next attempt before sending it.
			metadata[retryAfterKey] = result.ID
			if recoveryInput == "" {
				metadata[cloudTaskUnsubmittedKey] = true
				metadata[startedAtKey] = submissionStartedAt.Format(time.RFC3339Nano)
			}
			delete(metadata, cloudTaskResponseKey)
		case result.ID != "" && result.ID != responseID:
			delete(metadata, cloudTaskUnsubmittedKey)
			metadata[cloudTaskResponseKey] = result.ID
		case knownUnsubmitted && remote != nil && remote.Detail.ResponseID == "" &&
			((remote.Status == http.StatusServiceUnavailable && (remote.Detail.Code == "agent_offline" || remote.Detail.Code == "agent_unhealthy")) ||
				(remote.Status == http.StatusConflict && remote.Detail.Code == "agent_busy")):
			// Relay rejected the POST before assigning a response ID. Restore
			// eligibility only if no earlier attempt had an unknown outcome.
			metadata[cloudTaskUnsubmittedKey] = true
		default:
			metadataChanged = false
		}
		if metadataChanged && checkpoint() != nil {
			return result, errCloudTaskPersistence
		}
		if restore {
			continue
		}
		if err == nil && !retryRejected && result.Status != "in_progress" && result.Status != "queued" {
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
	if errors.Is(err, relay.ErrChatReference) || errors.Is(err, relay.ErrRequestTooLarge) {
		return false
	}
	var remote *relay.HTTPError
	if errors.As(err, &remote) {
		if remote.Status == http.StatusConflict && remote.Detail.Code == "agent_busy" {
			return true
		}
		return remote.Status >= http.StatusInternalServerError || remote.Status == http.StatusRequestTimeout || remote.Status == http.StatusTooManyRequests
	}
	return true
}
