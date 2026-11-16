package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/roomtask"
	"csgclaw/internal/taskcore"
	"csgclaw/internal/taskmeta"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type autonomousTaskResponse struct {
	roomtask.AutonomousDecision
	Tasks        []apitypes.RoomTask `json:"tasks"`
	ThreadRootID string              `json:"thread_root_id,omitempty"`
}

func (h *Handler) withAutonomousRoomMutation(roomID string, change func() error) error {
	if h.autonomousTasks != nil && h.im != nil {
		if room, ok := h.im.Room(roomID); ok && room.IsAutonomous() {
			return h.autonomousTasks.WithRoomMutation(change)
		}
	}
	return change()
}

func (h *Handler) registerAutonomousRoomTaskTools(agentID string) {
	type operation struct {
		name, description string
		fields            map[string]any
		required          []string
	}
	part := taskToolFields("title", "scope")
	split := map[string]any{"parts": map[string]any{
		"type": "array", "minItems": 2,
		"items": map[string]any{"type": "object", "properties": part, "required": []string{"title", "scope"}, "additionalProperties": false},
	}}
	update := taskToolFields("next_owner")
	update["status"] = map[string]any{"type": "string", "enum": []string{"completed", "blocked", "failed", "released", "handed_off"}}
	for _, op := range []operation{
		{"autonomous_tasks_list", "Read existing tracked work in this autonomous room conversation.", nil, nil},
		{"autonomous_task_claim", "For explicitly requested tracking, record ownership of a human request by source_message_id or of an existing part by task_id. The accepted field reports whether ownership was recorded. On conflict, the existing record and parts are returned.", taskToolFields("source_message_id", "task_id"), nil},
		{"autonomous_task_split", "For explicitly requested tracking, record an agreed division of the human request into verifiable parts. Each scope describes the deliverable and boundaries. A whole-request record and a split cannot both be accepted.", split, []string{"source_message_id", "parts"}},
		{"autonomous_task_update", "Record the result, blocker, failure or release for a task you own. Use handed_off with next_owner (a current Agent participant ID) for an agreed transfer. When coordinating a split, mark its root completed after the parts are delivered and you have verified the integrated result.", update, []string{"task_id", "status", "summary"}},
	} {
		fields := taskToolFields("room_id", "thread_root_id")
		for _, field := range op.required {
			fields[field] = map[string]any{"type": "string"}
		}
		for key, field := range op.fields {
			fields[key] = field
		}
		h.addAppPlatformTool(agentID, op.name, op.description, fields, op.required, op.name == "autonomous_tasks_list", func(_ context.Context, _ *mcp.CallToolRequest, args map[string]any) (any, error) {
			return h.runAutonomousTaskTool(agentID, op.name, args)
		})
	}
}

func (h *Handler) runAutonomousTaskTool(agentID, name string, args map[string]any) (any, error) {
	if h.autonomousTasks == nil || h.im == nil {
		return nil, fmt.Errorf("autonomous task service is unavailable")
	}
	roomID, threadID := platformText(args, "room_id"), platformText(args, "thread_root_id")
	if name == "autonomous_tasks_list" {
		if _, err := h.autonomousTaskRoom(agentID, roomID); err != nil {
			return nil, err
		}
		return h.autonomousRoomTaskViews(roomID, &threadID), nil
	}
	var response autonomousTaskResponse
	err := h.autonomousTasks.WithRoomMutation(func() error {
		room, err := h.autonomousTaskRoom(agentID, roomID)
		if err != nil {
			return err
		}
		actor := h.participantBridgeTargetForRoomMember(h.agentPlatformParticipant(agentID)).bridgeID
		taskID, sourceID := platformText(args, "task_id"), platformText(args, "source_message_id")
		rootID := taskID
		if name == "autonomous_task_split" || name == "autonomous_task_claim" && taskID == "" {
			source, sourceErr := h.autonomousTaskSource(room, threadID, sourceID)
			if sourceErr != nil {
				return sourceErr
			}
			var parts []roomtask.AutonomousPart
			if name == "autonomous_task_split" {
				raw, marshalErr := json.Marshal(args["parts"])
				if marshalErr != nil || json.Unmarshal(raw, &parts) != nil || parts == nil {
					return fmt.Errorf("invalid task parts")
				}
			}
			rootID, response.AutonomousDecision, err = h.autonomousTasks.Decide(source, actor, threadID, parts)
		} else {
			if sourceID != "" {
				return fmt.Errorf("use either the original message ID or a confirmed task ID")
			}
			task, found := h.autonomousTasks.Get(taskID)
			if !found || task.RoomID != roomID || !h.autonomousTaskInThread(task, threadID) {
				return taskcore.ErrTaskNotFound
			}
			if task.ParentID != "" {
				rootID = task.ParentID
			}
			if name == "autonomous_task_claim" {
				response.AutonomousDecision, err = h.autonomousTasks.Claim(task, actor, threadID)
			} else {
				nextOwner, ownerErr := h.autonomousHandoffOwner(room, platformText(args, "next_owner"))
				if ownerErr != nil {
					return ownerErr
				}
				response.AutonomousDecision, err = h.autonomousTasks.Update(task, actor, threadID, platformText(args, "status"), platformText(args, "summary"), nextOwner)
			}
		}
		if err != nil {
			return err
		}
		if root, found := h.autonomousTasks.Get(rootID); found {
			response.ThreadRootID = autonomousTaskTopic(root)
		}
		for _, task := range h.autonomousRoomTaskViews(roomID, &threadID) {
			if task.ID == rootID || task.ParentID == rootID {
				response.Tasks = append(response.Tasks, task)
			}
		}
		return nil
	})
	return response, err
}

func (h *Handler) autonomousHandoffOwner(room im.Room, id string) (string, error) {
	if id == "" {
		return "", nil
	}
	target := h.participantBridgeTargetForRoomMember(id)
	for _, member := range room.Members {
		if target.matches(member) {
			user, ok := h.im.User(member)
			if ok && (user.Role == "agent" || user.Role == "worker" || user.Role == "manager") {
				if h.agentLifecycleState(h.runtimeAgentIDForBridgeID(target.bridgeID)) != "" {
					break
				}
				return target.bridgeID, nil
			}
		}
	}
	return "", fmt.Errorf("next_owner must be a current Agent in this room")
}

func (h *Handler) autonomousTaskRoom(agentID, roomID string) (im.Room, error) {
	room, ok := h.im.Room(roomID)
	if !ok || !room.IsAutonomous() || !h.agentPlatformRoom(agentID, roomID) {
		return im.Room{}, fmt.Errorf("this tool requires membership in the current autonomous room")
	}
	if h.agentLifecycleState(agentID) != "" {
		return im.Room{}, fmt.Errorf("agent is archived or deleted")
	}
	return room, nil
}

func (h *Handler) autonomousTaskSource(room im.Room, threadID, sourceID string) (taskcore.Task, error) {
	message, user, err := h.im.AutonomousTaskSource(room.ID, threadID, sourceID)
	if err != nil {
		return taskcore.Task{}, err
	}
	title := strings.Join(strings.Fields(message.Content), " ")
	if r := []rune(title); len(r) > 100 {
		title = string(r[:99]) + "…"
	}
	if title == "" {
		title = "Attachment request"
		if strings.HasPrefix(room.Locale, "zh") {
			title = "附件处理"
		}
	}
	if message.ID == threadID {
		threadID = ""
	}
	return taskcore.Task{RoomID: room.ID, ThreadRootID: threadID, SourceMessageID: message.ID, CreatedBy: user.ID, Title: title, Body: message.Content}, nil
}

func (h *Handler) autonomousRoomTaskViews(roomID string, threadID *string) []apitypes.RoomTask {
	items := make([]apitypes.RoomTask, 0)
	for _, task := range h.autonomousTasks.List(roomID) {
		if threadID == nil || h.autonomousTaskInThread(task, *threadID) {
			items = append(items, h.apiRoomTask(task))
		}
	}
	return items
}

func (h *Handler) autonomousRoomTaskRead(w http.ResponseWriter, r *http.Request, events bool) bool {
	if h.autonomousTasks == nil || h.im == nil {
		return false
	}
	room, ok := h.im.Room(pathValue(r, "id"))
	if !ok || !room.IsAutonomous() {
		return false
	}
	task, found := h.autonomousTasks.Get(pathValue(r, "task_id"))
	if !found || task.RoomID != room.ID {
		http.NotFound(w, r)
		return true
	}
	if events {
		writeJSON(w, http.StatusOK, h.autonomousTasks.Events(task))
	} else {
		writeJSON(w, http.StatusOK, h.apiRoomTask(task))
	}
	return true
}

func (h *Handler) autonomousTaskInThread(task taskcore.Task, threadID string) bool {
	if task.ThreadRootID == threadID {
		return true
	}
	if threadID == "" || task.ThreadRootID != "" {
		return false
	}
	if task.ParentID != "" {
		root, ok := h.autonomousTasks.Get(task.ParentID)
		return ok && root.SourceMessageID == threadID
	}
	return task.SourceMessageID == threadID
}

func (h *Handler) publishAutonomousTaskNotice(task taskcore.Task, event taskcore.TaskEvent) error {
	room, ok := h.im.Room(task.RoomID)
	if !ok || !room.IsAutonomous() {
		return im.ErrRoomNotFound
	}
	action := map[string][2]string{
		taskcore.EventTaskClaimed:   {"认领了", "claimed"},
		taskcore.EventTaskAssigned:  {"交接了", "handed off"},
		roomtask.EventPlanned:       {"确认了分工并负责整合", "confirmed the split and will integrate"},
		taskcore.EventTaskBlocked:   {"遇到阻塞", "reported a blocker on"},
		taskcore.EventTaskCompleted: {"交付了", "delivered"},
		taskcore.EventTaskFailed:    {"报告失败", "reported a failure on"},
		"task.released":             {"释放了", "released"},
	}[event.Type]
	lang := 1
	if strings.HasPrefix(room.Locale, "zh") {
		lang = 0
	}
	content := fmt.Sprintf("%s %s · %s", action[lang], task.ID, task.Title)
	if event.Type == taskcore.EventTaskAssigned {
		if user, ok := h.im.User(h.resolveCSGClawParticipantUserID(event.TargetID)); ok {
			content += " → " + user.Name
		}
	}
	if event.Summary != task.Title {
		content += "\n" + event.Summary
	}
	sender := h.resolveCSGClawParticipantUserID(event.ActorID)
	id := fmt.Sprintf("autonomous-task:%s:event:%d", task.ID, event.Seq)
	root := task
	if task.ParentID != "" {
		parent, found := h.autonomousTasks.Get(task.ParentID)
		if !found {
			return taskcore.ErrTaskNotFound
		}
		root = parent
	}
	message, created, err := h.im.CreateMessageOnce(im.CreateMessageRequest{
		RoomID: task.RoomID, SenderID: sender, ClientMessageID: id, Content: content,
		RelatesTo: autonomousTaskThread(autonomousTaskTopic(root)),
		Metadata:  taskmeta.Set(map[string]any{"csgclaw": map[string]any{"delivery_kind": "autonomous_task_update"}}, task.ID, 0),
	})
	if err != nil {
		slog.Warn("publish autonomous task update", "task_id", task.ID, "event_seq", event.Seq, "error", err)
		return err
	}
	if created {
		h.publishMessageCreated(task.RoomID, message.SenderID, message)
		h.publishThreadUpdated(task.RoomID, message)
	}
	return nil
}

func autonomousTaskTopic(task taskcore.Task) string {
	if task.ThreadRootID != "" {
		return task.ThreadRootID
	}
	return task.SourceMessageID
}

func autonomousTaskThread(rootID string) *im.MessageRelation {
	if rootID == "" {
		return nil
	}
	return &im.MessageRelation{RelType: im.RelationTypeThread, EventID: rootID}
}
