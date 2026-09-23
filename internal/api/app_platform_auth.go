package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	agent "csgclaw/internal/agentengine/agents"
)

type appAgentContextKey struct{}

// ValidateAgentAccessToken authenticates a runtime at the desktop listener.
// It grants no route permissions; the API still checks Agent and resource scope.
func (h *Handler) ValidateAgentAccessToken(token string) bool {
	_, ok := h.agentForAuthorization("Bearer " + token)
	return ok
}

func (h *Handler) agentForAuthorization(header string) (string, bool) {
	owner, ok := h.svc.(interface{ AgentIDForAccessToken(string) (string, bool) })
	if !ok || !strings.HasPrefix(header, "Bearer ") {
		return "", false
	}
	return owner.AgentIDForAccessToken(strings.TrimPrefix(header, "Bearer "))
}

func (h *Handler) authorizesAgentToken(agentID, header string) bool {
	owner, ok := h.svc.(interface{ AuthorizesAgentAccessToken(string, string) bool })
	return ok && strings.HasPrefix(header, "Bearer ") && owner.AuthorizesAgentAccessToken(agentID, strings.TrimPrefix(header, "Bearer "))
}

func (h *Handler) authorizeAppPlatformRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.apps == nil {
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if agentID, ok := h.agentForAuthorization(r.Header.Get("Authorization")); ok {
			if !h.authorizeAgentPlatformRoute(r, agentID) {
				writeCodedAPIError(w, http.StatusForbidden, "agent_access_denied", "Agent cannot access this resource")
				return
			}
			a, _ := h.svc.Agent(agentID)
			r.Header.Set("X-CSGClaw-Caller-Agent", agent.ParticipantIDForAgent(a.Name, a.ID))
			r = r.WithContext(context.WithValue(r.Context(), appAgentContextKey{}, agentID))
			next.ServeHTTP(w, r)
			return
		}
		// Apps use the existing personal-service access policy. Agent credentials
		// are handled above and never gain management permissions from NoAuth.
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer agent.") {
			writeCodedAPIError(w, http.StatusUnauthorized, "unauthorized", "Invalid Agent credential")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func appRequestAgentID(r *http.Request) string {
	id, _ := r.Context().Value(appAgentContextKey{}).(string)
	return id
}

func (h *Handler) agentPlatformParticipant(agentID string) string {
	a, ok := h.svc.Agent(agentID)
	if !ok {
		return ""
	}
	return agent.ParticipantIDForAgent(a.Name, a.ID)
}

func (h *Handler) agentPlatformManager(agentID string) bool {
	a, ok := h.svc.Agent(agentID)
	return ok && a.Role == agent.RoleManager
}

func (h *Handler) agentPlatformRoom(agentID, roomID string) bool {
	if h.im == nil {
		return false
	}
	room, ok := h.im.Room(roomID)
	if !ok {
		return false
	}
	actor := h.participantBridgeTargetForRoomMember(h.agentPlatformParticipant(agentID))
	for _, member := range room.Members {
		if actor.matches(member) {
			return true
		}
	}
	return false
}

// Only legacy runtime routes with an explicit, checkable resource scope remain
// available. User administration follows the existing service access policy.
func (h *Handler) authorizeAgentPlatformRoute(r *http.Request, agentID string) bool {
	p := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/")
	parts := strings.Split(p, "/")
	if h.agentPlatformManager(agentID) && managerSkillRoute(r) {
		return true
	}
	if parts[0] == "agent-tasks" || parts[0] == "teams" {
		return h.authorizeAgentTaskRoute(r, agentID, parts)
	}
	if len(parts) >= 3 && parts[0] == "agents" {
		if agent.CanonicalID(parts[1]) != agent.CanonicalID(agentID) {
			return false
		}
		return parts[2] == "llm" || (len(parts) == 3 && parts[2] == "mcp") || (r.Method == http.MethodGet && len(parts) == 3 && parts[2] == "connectors")
	}
	if p == "connectors/catalog" && r.Method == http.MethodGet {
		return true
	}
	if (len(parts) == 3 || len(parts) == 4) && parts[0] == "rooms" && parts[2] == "attachments" && (r.Method == http.MethodGet || (len(parts) == 4 && r.Method == http.MethodDelete)) {
		return h.agentPlatformRoom(agentID, parts[1])
	}
	if len(parts) >= 3 && parts[0] == "rooms" && parts[2] == "tasks" {
		return h.agentPlatformRoom(agentID, parts[1])
	}
	if len(parts) == 3 && parts[0] == "rooms" && parts[2] == "task-context" && r.Method == http.MethodGet {
		return h.agentPlatformRoom(agentID, parts[1])
	}
	if p == "messages" {
		if r.Method == http.MethodGet {
			return h.agentPlatformRoom(agentID, r.URL.Query().Get("room_id"))
		}
		if r.Method == http.MethodPost {
			payload, ok := platformRequestBody(r)
			if !ok {
				return false
			}
			roomID, _ := payload["room_id"].(string)
			sender, _ := payload["sender_id"].(string)
			actor := h.participantBridgeTargetForRoomMember(h.agentPlatformParticipant(agentID))
			if sender != "" && !actor.matches(sender) {
				return false
			}
			payload["sender_id"] = actor.bridgeID
			setPlatformRequestBody(r, payload)
			return h.agentPlatformRoom(agentID, roomID)
		}
	}
	if h.agentPlatformManager(agentID) && r.Method == http.MethodGet {
		return p == "agents" || p == "hub/templates" || p == "channels/csgclaw/participants" || p == "channels/feishu/participants"
	}
	return false
}

// managerSkillRoute preserves the existing Agent Creator and Feishu Skills'
// CLI workflows without granting App administration or another Agent's MCP.
func managerSkillRoute(r *http.Request) bool {
	method := r.Method
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.EscapedPath(), "/api/v1/"), "/"), "/")
	if len(parts) == 3 && parts[0] == "hub" && parts[1] == "templates" {
		return method == http.MethodGet
	}
	if len(parts) == 3 && parts[0] == "agents" {
		return method == http.MethodPost && (parts[2] == "recreate" || parts[2] == "bindings:apply")
	}
	if len(parts) < 3 || parts[0] != "channels" || (parts[1] != "csgclaw" && parts[1] != "feishu") {
		return false
	}
	switch parts[2] {
	case "participants":
		return (len(parts) == 3 && (method == http.MethodGet || method == http.MethodPost)) ||
			(len(parts) == 4 && (method == http.MethodGet || method == http.MethodPatch))
	case "rooms":
		return (len(parts) == 3 && (method == http.MethodGet || method == http.MethodPost)) ||
			(len(parts) == 5 && parts[4] == "members" && method == http.MethodPost)
	}
	return false
}

func platformRequestBody(r *http.Request) (map[string]any, bool) {
	if r.Body == nil {
		return map[string]any{}, true
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1024*1024+1))
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil || len(raw) > 1024*1024 {
		return nil, false
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, true
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil || payload == nil {
		return nil, false
	}
	return payload, true
}

func setPlatformRequestBody(r *http.Request, payload map[string]any) {
	raw, _ := json.Marshal(payload)
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.ContentLength = int64(len(raw))
}
