package api

import (
	"context"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/roomtask"
	"encoding/json"
	"fmt"
)

func (h *Handler) roomSceneContext(room im.Room, request roomtask.TurnContextRequest) (roomtask.PrivateTurnContext, error) {
	actor := h.participantBridgeTargetForRoomMember(request.ParticipantID)
	member := false
	for _, id := range room.Members {
		member = member || actor.matches(id)
	}
	if !member {
		return roomtask.PrivateTurnContext{Skip: room.IsAutonomous()}, nil
	}
	var notification *apitypes.UnreadNotification
	if room.IsAutonomous() {
		notice, admitted, err := h.im.AdmitAutonomousNotification(room.ID, request.SourceID, request.ParticipantID)
		if err != nil {
			return roomtask.PrivateTurnContext{}, err
		}
		if !admitted {
			return roomtask.PrivateTurnContext{Skip: true}, nil
		}
		notification = &notice
	}
	role := roomtask.TurnRoleMember
	if room.IsDirect {
		role = roomtask.TurnRoleDirect
	} else if room.IsAutonomous() {
		role = roomtask.TurnRoleAutonomous
	}
	scope := h.roomContextFacts(roomtask.Roster{RoomID: room.ID})
	h.addSceneIdentity(scope, room, request)
	turn := h.sceneEvent(room, request)
	snapshot := map[string]any{}
	if notification != nil {
		snapshot["unread"] = notification
	}

	scopeJSON, err := json.Marshal(scope)
	if err != nil {
		return roomtask.PrivateTurnContext{}, err
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return roomtask.PrivateTurnContext{}, err
	}
	turnJSON, err := json.Marshal(turn)
	if err != nil {
		return roomtask.PrivateTurnContext{}, err
	}
	return roomtask.PrivateTurnContext{Notification: notification, Role: role, PolicyID: roomtask.TurnPolicyID(role), ScopeJSON: string(scopeJSON), SnapshotJSON: string(snapshotJSON), TurnJSON: string(turnJSON), CollaborationGuide: room.CollaborationGuide}, nil
}

func (h *Handler) addSceneIdentity(scope map[string]any, room im.Room, request roomtask.TurnContextRequest) {
	actor := h.participantBridgeTargetForRoomMember(request.ParticipantID)
	self := map[string]any{"agent_id": h.runtimeAgentIDForBridgeID(actor.bridgeID), "participant_id": actor.bridgeID}
	for _, id := range room.Members {
		if actor.matches(id) {
			if user, ok := h.im.User(id); ok {
				self["name"], self["description"], self["role"] = user.Name, user.Description, user.Role
			}
		}
	}
	if h.agentEngine != nil {
		if profile, err := h.agentEngine.Agents().Get(context.Background(), fmt.Sprint(self["agent_id"]), agentengine.AgentGetOptions{}); err == nil {
			self["name"], self["description"], self["role"] = profile.Spec.Name, profile.Spec.Description, profile.Spec.Role
			self["created_by"], self["named_by"] = profile.CreatedBy, profile.NamedBy
			if user, ok := h.im.User(profile.NamedBy); ok {
				self["named_by_name"] = user.Name
			}
		}
	}
	scope["self"], scope["title"], scope["description"] = self, room.Title, room.Description
	scope["room_type"] = apitypes.NormalizeRoomType(room.Type)
	if room.IsDirect {
		scope["room_type"] = "direct"
	}
	scope["reply_target"] = map[string]any{"room_id": room.ID}
	delete(scope, "collaboration_guide")
}

func (h *Handler) sceneEvent(room im.Room, request roomtask.TurnContextRequest) map[string]any {
	target := map[string]any{"room_id": room.ID}
	turn := map[string]any{"source_message_id": request.SourceID, "related_task_id": request.TaskID, "reply_target": target}
	messages, err := h.im.ListMessagesWithOptions(room.ID, im.ListMessagesOptions{IncludeThreadReplies: true})
	if err != nil {
		return turn
	}
	for _, message := range messages {
		if message.ID != request.SourceID {
			continue
		}
		sender := map[string]any{"id": message.SenderID, "type": "unknown"}
		if user, ok := h.im.User(message.SenderID); ok {
			sender["name"], sender["role"] = user.Name, user.Role
			if user.Role == "admin" || user.Role == "user" || user.Role == "human" {
				sender["type"] = "human"
			} else if h.isAgentSender(message.SenderID) {
				sender["type"] = "agent"
			}
		}
		if message.Event != nil {
			sender["type"] = "system"
			turn["event_key"] = message.Event.Key
		}
		turn["sender"], turn["created_at"] = sender, message.CreatedAt
		actor := h.participantBridgeTargetForRoomMember(request.ParticipantID)
		mentions := make([]string, 0, len(message.Mentions))
		addressed := false
		for _, mention := range message.Mentions {
			mentions = append(mentions, mention.ID)
			addressed = addressed || actor.matches(mention.ID)
		}
		turn["mentioned_participant_ids"], turn["addressed_to_you"] = mentions, addressed
		if message.RelatesTo != nil {
			turn["reply_relation"] = message.RelatesTo
			if message.RelatesTo.RelType == im.RelationTypeThread {
				target["thread_root_id"] = message.RelatesTo.EventID
			}
		}
		break
	}
	return turn
}
