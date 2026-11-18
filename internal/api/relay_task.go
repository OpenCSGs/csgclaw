package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
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
		var attachments []im.MessageAttachmentUpload
		var releaseAttachments func()
		if err != nil || result.Status != "completed" {
			code := "execution_failed"
			if result.Error != nil {
				switch result.Error.Code {
				case "model_protocol_unsupported", "input_file_unavailable", "file_delivery_failed", "file_delivery_invalid":
					code = result.Error.Code
				case "resource_not_attached", "resource_disabled", "resource_unavailable", "invalid_resource_reference":
					code = result.Error.Code
				case "execution_interrupted", "execution_timeout", "cancellation_unconfirmed", "relay_invocation_identity_mismatch":
					code = "relay_task_result_unavailable"
				}
			}
			var remote *relay.HTTPError
			if errors.Is(err, errCloudTaskResultUnavailable) ||
				(errors.As(err, &remote) && remote.Detail.Code == "response_not_found") {
				code = "relay_task_result_unavailable"
			}
			if errors.Is(err, errCloudTaskAttachment) {
				code = "input_file_unavailable"
			}
			if errors.Is(err, relay.ErrRequestTooLarge) {
				code = "relay_task_input_too_large"
			}
			content = cloudTaskFailureText(room.Locale, code)
			metadata["relay_error_code"] = code
			metadata["relay_task_failed"] = true
		} else {
			metadata["relay_response_id"] = result.ID
			var files []relay.File
			for _, output := range result.Output {
				for _, part := range output.Content {
					if part.Type == "output_file" && part.File != nil {
						files = append(files, *part.File)
					}
				}
			}
			if len(files) > 0 {
				attachments, releaseAttachments, err = h.cloudOutputAttachments(parent, room, files)
				if err != nil && parent.Err() == nil {
					code := "file_delivery_failed"
					if len(attachments) > 0 {
						code = "file_delivery_partial"
					}
					metadata["relay_error_code"] = code
					note := cloudTaskFailureText(room.Locale, code)
					content = strings.TrimSpace(content + "\n\n" + note)
				}
			}
			if content == "" && len(attachments) == 0 {
				content = "(empty response)"
			}
		}
		var reply im.Message
		var created bool
		saveErr := retryCloudTaskWrite(parent, func() error {
			var err error
			reply, created, err = h.im.CreateMessageOnce(im.CreateMessageRequest{RoomID: roomID, SenderID: room.CloudAgent.UserID, Content: content, ClientMessageID: "relay-reply:" + message.ID, Metadata: metadata, Locale: room.Locale, Attachments: attachments})
			return err
		})
		if releaseAttachments != nil {
			releaseAttachments()
		}
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

// Keep the current attempt in memory while storage is unavailable. Retrying a
// write must not repeat a Relay request or regenerate its checkpoint.
func retryCloudTaskWrite(ctx context.Context, write func() error) error {
	for delay := time.Second; ; delay = min(2*delay, 30*time.Second) {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := write()
		var pathErr *os.PathError
		var linkErr *os.LinkError
		if !errors.As(err, &pathErr) && !errors.As(err, &linkErr) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// Use trusted local text instead of exposing arbitrary upstream diagnostics.
func cloudTaskFailureText(locale, code string) string {
	var en, zh string
	switch code {
	case "resource_not_attached", "resource_disabled", "resource_unavailable", "invalid_resource_reference":
		en = "A selected resource is no longer available. Remove or reselect the resource before sending the message again."
		zh = "所选资源已不可用，请移除或重新选择资源后再发送消息。"
	case "input_file_unavailable":
		en = "The cloud agent could not receive the input attachments."
		zh = "云端智能体未能收到输入附件。"
	case "file_delivery_partial":
		en = "Some of the cloud agent's output files could not be delivered."
		zh = "云端智能体的部分输出文件未能交付。"
	case "file_delivery_failed", "file_delivery_invalid":
		en = "The cloud agent's output files could not be delivered."
		zh = "云端智能体的输出文件未能交付。"
	case "relay_task_input_too_large":
		en = "This message exceeds the cloud task request size limit. Shorten the message and send it again."
		zh = "这条消息超过了云端任务的请求大小限制，请缩短消息后再发送。"
	case "relay_task_result_unavailable":
		en = "The result of this message could not be confirmed. Check whether it completed before sending it again."
		zh = "暂时无法确认这条消息的执行结果。请确认消息是否已执行，再决定是否重发。"
	case "model_protocol_unsupported":
		en = "The published agent's model does not support the Responses tool protocol required by its runtime. Select a compatible provider/model in the source agent's settings, then send again."
		zh = "发布端智能体的模型不支持运行时所需的 Responses 工具协议。请在本地源智能体的设置中选择支持该协议的模型服务，再重新发送。"
	default:
		en = "The cloud task could not complete. Check the cloud agent status and try again later."
		zh = "云端任务未能完成，请检查云端智能体状态后重试。"
	}
	if locale == "en" {
		return en
	}
	return zh
}

var errCloudTaskAttachment = errors.New("cloud task input attachment unavailable")
var errCloudTaskPersistence = errors.New("cloud task recovery state could not be saved")
var errCloudTaskResultUnavailable = errors.New("cloud task execution result could not be confirmed")

const (
	cloudTaskMaxInvalidPolls  = 3
	cloudTaskUnsubmittedKey   = "relay_task_unsubmitted"
	cloudTaskResponseKey      = "relay_pending_response_id"
	cloudTaskInputFilesKey    = "relay_input_files"
	cloudTaskRecoveryKey      = "relay_context_recovery_input"
	cloudTaskStartedAtKey     = "relay_task_started_at"
	cloudTaskRetryAfterKey    = "relay_task_retry_after"
	cloudTaskRecoveryStartKey = "relay_context_recovery_started_at"
	cloudTaskRecoveryRetryKey = "relay_context_recovery_after"
)

// Build a text checkpoint from successful turns in this room. Replies can be
// appended after the next user message when several messages are queued.
func cloudTaskRecoveryInput(room im.Room, message im.Message) string {
	const unavailableFiles = "Attachments from earlier turns are unavailable in this restored session. If the current request needs one of those files, ask the user to attach it again. Do not infer its contents from the conversation."
	replies := make(map[string]im.Message)
	for _, reply := range room.Messages {
		if id, _ := reply.Metadata["relay_response_id"].(string); reply.SenderID == room.CloudAgent.UserID && id != "" {
			replies[reply.ClientMessageID] = reply
		}
	}
	budget := (16 << 10) - len("[]")
	var turns []string
	hadAttachments := false
	for i := slices.IndexFunc(room.Messages, func(m im.Message) bool { return m.ID == message.ID }) - 1; i >= 0; i-- {
		prior := room.Messages[i]
		reply, ok := replies["relay-reply:"+prior.ID]
		if prior.SenderID != room.CreatedBy || prior.Kind == im.MessageKindEvent {
			continue
		}
		hadAttachments = hadAttachments || len(prior.Attachments) > 0 || len(reply.Attachments) > 0
		if !ok || len(prior.Content)+len(reply.Content) >= budget {
			continue
		}
		turn, _ := json.Marshal(struct {
			User      string                      `json:"user"`
			Agent     string                      `json:"agent"`
			Resources *apitypes.ResourceSelection `json:"resource_selection,omitempty"`
		}{prior.Content, reply.Content, apitypes.ResourceSelectionFromMetadata(prior.Metadata)})
		if len(turn)+1 > budget {
			continue
		}
		budget -= len(turn) + 1
		turns = append(turns, string(turn))
	}
	fallback := message.Content
	currentInput := "Current user message:\n" + message.Content
	if hadAttachments {
		currentInput = unavailableFiles + "\n\n" + currentInput
		fallback = currentInput
	}
	if len(turns) == 0 {
		return fallback
	}
	slices.Reverse(turns)
	input := "The remote session context was lost. Use the following completed turns as background. Historical resource selections describe those turns only; do not apply them to the current turn unless selected again. Do not repeat prior actions. Respond to the current user message.\n[" +
		strings.Join(turns, ",") + "]\n\n" + currentInput
	// Leave room for the Relay request envelope when the current message
	// alone is already close to the request size limit.
	if encoded, _ := json.Marshal(input); len(encoded) > (1<<20)-1024 {
		return fallback
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
	_, recovering := metadata[cloudTaskRecoveryKey].(string)
	startedAtKey, retryAfterKey := cloudTaskStartedAtKey, cloudTaskRetryAfterKey
	if recovering {
		startedAtKey, retryAfterKey = cloudTaskRecoveryStartKey, cloudTaskRecoveryRetryKey
	}
	startedAt, _ := metadata[startedAtKey].(string)
	unsubmitted, _ := metadata[cloudTaskUnsubmittedKey].(bool)
	submissionStartedAt, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		submissionStartedAt = message.CreatedAt
		if unsubmitted && !recovering && responseID == "" {
			submissionStartedAt = time.Now().UTC()
		}
	}
	// Reconfirm the checkpoint before the first POST after a worker restart.
	needsCheckpoint := true
	checkpoint := func() error {
		needsCheckpoint = false
		return retryCloudTaskWrite(ctx, func() error {
			_, err := h.im.UpdateDeliveredMessageMetadata(room.ID, message.ID, message.SenderID, metadata)
			return err
		})
	}
	var relayFiles []relay.File
	if saved, ok := metadata[cloudTaskInputFilesKey]; ok {
		data, err := json.Marshal(saved)
		if err != nil || json.Unmarshal(data, &relayFiles) != nil || len(relayFiles) != len(message.Attachments) {
			return relay.Response{}, errCloudTaskResultUnavailable
		}
	}
	invalidPolls := 0
	for {
		if ctx.Err() != nil {
			return relay.Response{}, ctx.Err()
		}
		responseID, _ := metadata[cloudTaskResponseKey].(string)
		retryAfter, _ := metadata[retryAfterKey].(string)
		recoveryInput, recovering := metadata[cloudTaskRecoveryKey].(string)
		unsubmitted, _ := metadata[cloudTaskUnsubmittedKey].(bool)
		knownUnsubmitted := unsubmitted && !recovering && responseID == ""
		freshSubmission := knownUnsubmitted && retryAfter == ""
		submissionDeadline := submissionStartedAt.Add(10 * time.Minute)
		if responseID == "" && len(message.Attachments) > 0 && relayFiles == nil &&
			(freshSubmission || time.Now().Before(submissionDeadline)) {
			uploadDeadline := submissionDeadline
			if freshSubmission {
				uploadDeadline = time.Now().Add(10 * time.Minute)
			}
			uploadCtx, cancel := context.WithDeadline(ctx, uploadDeadline)
			files, err := h.uploadCloudTaskAttachments(uploadCtx, room, message)
			cancel()
			if err != nil {
				return relay.Response{}, err
			}
			relayFiles = files
			metadata[cloudTaskInputFilesKey] = files
			needsCheckpoint = true
			if freshSubmission {
				submissionStartedAt = time.Now().UTC()
				submissionDeadline = submissionStartedAt.Add(10 * time.Minute)
			}
		}
		if knownUnsubmitted {
			// Consume this marker before the POST so a restart cannot mistake
			// an already attempted submission for a new one.
			delete(metadata, cloudTaskUnsubmittedKey)
			metadata[startedAtKey] = submissionStartedAt.Format(time.RFC3339Nano)
			needsCheckpoint = true
		}
		if responseID == "" && needsCheckpoint && time.Now().Before(submissionDeadline) && checkpoint() != nil {
			return relay.Response{}, errCloudTaskPersistence
		}
		if responseID == "" && !time.Now().Before(submissionDeadline) {
			if !unsubmitted {
				return relay.Response{}, errCloudTaskResultUnavailable
			}
			return relay.Response{}, fmt.Errorf("request recovery window expired")
		}
		callDeadline := time.Now().Add(30 * time.Second)
		if responseID == "" && submissionDeadline.Before(callDeadline) {
			callDeadline = submissionDeadline
		}
		callCtx, cancel := context.WithDeadline(ctx, callDeadline)
		var result relay.Response
		var err error
		var remote *relay.HTTPError
		if responseID != "" {
			result, err = h.relayManager.ChatResponse(callCtx, room.CloudAgent.AgentID, room.CloudAgent.ConnectionID, responseID)
		} else {
			key, input, parent := "task:"+room.ID+":"+message.ID, message.Content, previous
			if recovering {
				key, input, parent = "task-recovery:"+room.ID+":"+message.ID, recoveryInput, ""
			}
			if retryAfter != "" {
				key = fmt.Sprintf("task-recovery:%x", sha256.Sum256([]byte(key+"\x00"+retryAfter)))
			}
			var refs []apitypes.ResourceReference
			if selection := apitypes.ResourceSelectionFromMetadata(message.Metadata); selection != nil {
				for _, ref := range selection.Refs {
					refs = append(refs, ref.ResourceReference)
				}
			}
			result, err = h.relayManager.ChatWithResources(callCtx, room.CloudAgent.AgentID, room.CloudAgent.ConnectionID, key, input, parent, refs, relayFiles...)
			if errors.As(err, &remote) {
				if id := remote.PendingResponseID(); id != "" {
					result.ID, result.Status, err = id, "in_progress", nil
					remote = nil
				}
			}
		}
		cancel()
		if errors.Is(err, relay.ErrInvalidChatResponse) {
			invalidPolls++
			if invalidPolls >= cloudTaskMaxInvalidPolls {
				return relay.Response{}, errCloudTaskResultUnavailable
			}
		} else if err == nil {
			invalidPolls = 0
		}
		restore := !recovering && previous != "" && cloudTaskCanRestoreContext(result, remote, knownUnsubmitted)
		retryRejected := err == nil && result.ID != retryAfter && cloudTaskRejectedBeforeExecution(result)
		metadataChanged := true
		switch {
		case restore:
			startedAtKey, retryAfterKey = cloudTaskRecoveryStartKey, cloudTaskRecoveryRetryKey
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
			if !recovering {
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
			// A later rejection cannot establish the outcome of an earlier
			// submission. The local size limit rejects the same body before
			// any POST, so keep the specific size error.
			if !knownUnsubmitted && !errors.Is(err, relay.ErrRequestTooLarge) {
				return result, errCloudTaskResultUnavailable
			}
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

func (h *Handler) uploadCloudTaskAttachments(ctx context.Context, room im.Room, message im.Message) ([]relay.File, error) {
	var relayFiles []relay.File
	for _, attachment := range message.Attachments {
		source, err := h.im.RoomAttachmentFile(room.ID, attachment.ID)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errCloudTaskAttachment, err)
		}
		file, err := h.relayManager.UploadAttachment(ctx, room.CloudAgent.AgentID, room.CloudAgent.ConnectionID, message.ID, attachment, func() (io.ReadCloser, error) {
			return os.Open(source.Path)
		})
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errCloudTaskAttachment, err)
		}
		relayFiles = append(relayFiles, file)
	}
	return relayFiles, nil
}

func (h *Handler) cloudOutputAttachments(ctx context.Context, room im.Room, files []relay.File) ([]im.MessageAttachmentUpload, func(), error) {
	paths := make([]string, len(files))
	uploads := make([]im.MessageAttachmentUpload, len(files))
	for i, file := range files {
		uploads[i] = im.MessageAttachmentUpload{Name: file.Name, MediaType: file.MediaType, SizeBytes: file.Size, Open: func() (io.ReadCloser, error) {
			return os.Open(paths[i])
		}}
	}
	cleanup := func() {
		for _, path := range paths {
			if path != "" {
				_ = os.Remove(path)
			}
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	var delivered []im.MessageAttachmentUpload
	var transferErr error
	var totalBytes int64
	for i, file := range files {
		if err := im.ValidateMessageAttachmentUploads(uploads[i : i+1]); err != nil {
			transferErr = err
			continue
		}
		if len(delivered) >= im.MaxAttachmentsPerMessage || file.Size > im.MaxAttachmentMessageBytes-totalBytes {
			transferErr = fmt.Errorf("output attachments exceed conversation limits")
			continue
		}
		path, err := h.stageCloudOutputAttachment(ctx, room, file)
		paths[i] = path
		if err != nil {
			transferErr = err
			if ctx.Err() != nil {
				break
			}
			continue
		}
		delivered = append(delivered, uploads[i])
		totalBytes += file.Size
	}
	return delivered, cleanup, transferErr
}

func (h *Handler) stageCloudOutputAttachment(ctx context.Context, room im.Room, file relay.File) (string, error) {
	target, err := os.CreateTemp("", "csgclaw-relay-output-*")
	if err != nil {
		return "", err
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err = target.Truncate(0); err != nil {
			break
		}
		if _, err = target.Seek(0, io.SeekStart); err != nil {
			break
		}
		err = h.relayManager.DownloadAttachment(ctx, room.CloudAgent.AgentID, room.CloudAgent.ConnectionID, file, target)
		var remote *relay.HTTPError
		if err == nil || ctx.Err() != nil || (errors.As(err, &remote) && remote.Status >= 400 && remote.Status < 500) {
			break
		}
	}
	return target.Name(), errors.Join(err, target.Close())
}
