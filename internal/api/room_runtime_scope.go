package api

import (
	"fmt"
	"net/http"
	"strings"

	"context"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/roomtask"
)

type activeRoomReader interface {
	ActiveRooms(participantID string) []string
}

// Runtime CLI calls inherit their caller identity. Scope is resolved from the
// server's active work lease, not a model-selected room or a process-wide room
// variable. Browser/human CLI calls have no runtime identity and remain global.
// This is workflow isolation; an unsandboxed process with the server credential
// is still privileged and must not be considered a hostile-code security boundary.
func (h *Handler) roomRuntimeScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller := strings.TrimSpace(r.Header.Get("X-CSGClaw-Caller-Agent"))
		if caller == "" {
			next.ServeHTTP(w, r)
			return
		}
		reader, ok := h.participantWork.(activeRoomReader)
		if !ok || h.im == nil {
			// No built-in IM execution context (for example Feishu or a detached
			// maintenance command): do not change those existing workflows.
			next.ServeHTTP(w, r)
			return
		}
		participantID := h.participantBridgeTargetForRoomMember(caller).bridgeID
		rooms := reader.ActiveRooms(participantID)
		if len(rooms) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		// Message payloads carry their room and sender. Validate those after
		// parsing in handleCreateMessage, rather than rejecting ordinary @ input.
		if r.Method == http.MethodPost && (r.URL.Path == "/api/v1/channels/csgclaw/messages" || r.URL.Path == "/api/v1/messages") {
			next.ServeHTTP(w, r)
			return
		}
		selectedRoom := ""
		// Independent task sessions can be active in different rooms. An explicit
		// room path or globally unique task id may narrow to one active lease,
		// never to an inactive room.
		for _, id := range rooms {
			base := "/api/v1/rooms/" + id
			if r.URL.Path == base+"/task-context" || r.URL.Path == base+"/tasks" || strings.HasPrefix(r.URL.Path, base+"/tasks/") {
				selectedRoom = id
			}
		}
		resolvedTaskID := ""
		if strings.HasPrefix(r.URL.Path, "/api/v1/tasks/") && h.roomTaskSvc != nil {
			resolvedTaskID = strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/"), "/")
			if !strings.Contains(resolvedTaskID, "/") {
				if task, found := h.roomTaskSvc.Resolve(resolvedTaskID); found {
					for _, id := range rooms {
						if task.RoomID == id {
							selectedRoom = id
						}
					}
				}
			}
		}
		if selectedRoom == "" && len(rooms) == 1 {
			selectedRoom = rooms[0]
		}
		if selectedRoom == "" {
			http.Error(w, "runtime execution scope missing or ambiguous; retry in the active task turn", http.StatusConflict)
			return
		}
		room, found := h.im.Room(selectedRoom)
		if !found {
			http.Error(w, "active room unavailable", http.StatusConflict)
			return
		}
		if !room.IsOnDemand() {
			next.ServeHTTP(w, r)
			return
		}
		meta, valid := h.roomSchedulingContext(room.ID)
		if !valid {
			http.Error(w, "collaboration roster unavailable", http.StatusConflict)
			return
		}
		manager := h.participantBridgeTargetForRoomMember(participantID).matches(meta.ManagerID)
		member := manager
		for _, id := range meta.WorkerIDs {
			member = member || h.participantBridgeTargetForRoomMember(participantID).matches(id)
		}
		if !member {
			http.Error(w, "caller is no longer a room member", http.StatusForbidden)
			return
		}
		path := strings.TrimRight(r.URL.Path, "/")
		base := "/api/v1/rooms/" + room.ID
		allowed := r.Method == http.MethodGet && (path == base+"/task-context" || path == base+"/tasks")
		if r.Method == http.MethodGet && resolvedTaskID != "" && h.roomTaskSvc != nil {
			if task, found := h.roomTaskSvc.Get(room.ID, resolvedTaskID); found {
				allowed = manager || (task.ParentID != "" && h.participantBridgeTargetForRoomMember(task.AssignedTo).matches(participantID))
			}
		}
		if manager && (path == base+"/tasks" || strings.HasPrefix(path, base+"/tasks/")) {
			allowed = true
		}
		if !manager && strings.HasPrefix(path, base+"/tasks/") && h.roomTaskSvc != nil {
			parts := strings.Split(strings.TrimPrefix(path, base+"/tasks/"), "/")
			task, found := h.roomTaskSvc.Get(room.ID, parts[0])
			if found && task.ParentID != "" && h.participantBridgeTargetForRoomMember(task.AssignedTo).matches(participantID) {
				allowed = (r.Method == http.MethodGet && len(parts) == 1) || (r.Method == http.MethodPatch && len(parts) == 1) || (r.Method == http.MethodPost && len(parts) == 2 && (parts[1] == "claim" || parts[1] == "messages"))
			}
		}
		if r.Method == http.MethodGet {
			allowed = allowed || path == "/api/v1/channels/csgclaw/rooms/"+room.ID+"/members"
			allowed = allowed || (path == "/api/v1/channels/csgclaw/messages" && r.URL.Query().Get("room_id") == room.ID)
		}
		if !allowed {
			http.Error(w, fmt.Sprintf("on-demand room %s is the execution boundary: use the supplied room context and task operations in this room; global Agent/Team/direct-task APIs and other rooms are unavailable", room.ID), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) handleRoomTaskContext(w http.ResponseWriter, r *http.Request) {
	meta, ok := h.roomSchedulingContext(pathValue(r, "id"))
	if !ok {
		http.Error(w, "collaboration room unavailable", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, h.roomContextFacts(meta))
}

func (h *Handler) roomContextFacts(meta roomtask.Roster) map[string]any {
	room, _ := h.im.Room(meta.RoomID)
	type member struct {
		ID          string `json:"participant_id"`
		Name        string `json:"name"`
		Role        string `json:"role"`
		Description string `json:"description,omitempty"`
	}
	members := make([]member, 0, len(room.Members))
	for _, id := range room.Members {
		user, found := h.im.User(id)
		if !found {
			continue
		}
		participantID := h.participantBridgeTargetForRoomMember(id).bridgeID
		item := member{ID: participantID, Name: user.Name, Role: user.Role}
		item.Description = user.Description
		if h.agentEngine != nil {
			if profile, err := h.agentEngine.Agents().Get(context.Background(), h.runtimeAgentIDForBridgeID(participantID), agentengine.AgentGetOptions{}); err == nil {
				item.Description = profile.Spec.Description
			}
		}
		members = append(members, item)
	}
	return map[string]any{"room_id": room.ID, "manager_id": meta.ManagerID, "members": members, "assignable_worker_ids": meta.WorkerIDs, "task_policy": "manager-created parent tasks queue per room; one-level worker children; explicit manager dispatch and review; dependencies require acceptance; same room only; QA defects require repair and regression children appended to the original parent with plan --append --request-id, not a new parent; accepting a defect report is not product success; close only after required work is accepted"}
}
