package api

import (
	"context"
	"slices"
	"strings"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
)

func (h *Handler) localChatParticipants() []apitypes.Participant {
	if h.participant == nil {
		return nil
	}
	return h.participant.List(participant.ListOptions{Channel: participant.ChannelCSGClaw})
}

// Bindings are authoritative, including bindings whose chat user is missing.
// Never turn an unrelated same-suffix user into the agent's chat identity.
func (h *Handler) agentChatUserIDs(agentID string) []string {
	agentID = agent.CanonicalID(agentID)
	var ids []string
	bound := false
	for _, p := range h.localChatParticipants() {
		if agent.CanonicalID(p.AgentID) != agentID || p.Type != participant.TypeAgent {
			continue
		}
		bound = true
		if p.ChannelUserKind == participant.ChannelUserKindLocalUserID && p.ChannelUserRef != "" {
			ids = append(ids, localUserIDFromAny(p.ChannelUserRef))
		}
	}
	if !bound {
		if id := h.legacyAgentChatUserID(agentID); id != "" {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

func (h *Handler) legacyAgentChatUserID(agentID string) string {
	if h.im == nil {
		return ""
	}
	id := localUserIDFromAny(agentID)
	user, ok := h.im.User(id)
	if ok && user.ID == id && h.legacyChatUserMatchesAgent(user, agentID) {
		return id
	}
	return ""
}

// Older installations can have no participant record. Require both the legacy
// ID convention and matching agent profile, with neither side claimed by a
// participant. This function must not call IM: it also runs under the IM lock.
func (h *Handler) legacyChatUserMatchesAgent(user im.User, agentID string) bool {
	if h.agentEngine == nil || user.AgentState == "deleted" {
		return false
	}
	agentID = agent.CanonicalID(agentID)
	if user.ID != localUserIDFromAny(agentID) || (user.AgentID != "" && agent.CanonicalID(user.AgentID) != agentID) {
		return false
	}
	for _, p := range h.localChatParticipants() {
		if localUserIDFromAny(p.ChannelUserRef) == user.ID || agent.CanonicalID(p.AgentID) == agentID {
			return false
		}
	}
	item, err := h.agentEngine.Agents().Get(context.Background(), agentID, agentengine.AgentGetOptions{})
	return err == nil && (user.Role == "worker" || user.Role == "manager") &&
		user.Role == string(item.Spec.Role) && strings.EqualFold(user.Name, item.Spec.Name)
}

func (h *Handler) chatUserLifecycleState(user im.User) string {
	bound := false
	for _, p := range h.localChatParticipants() {
		if p.ChannelUserKind != participant.ChannelUserKindLocalUserID || localUserIDFromAny(p.ChannelUserRef) != user.ID {
			continue
		}
		bound = true
		// A shared identity remains usable while another binding is active.
		if p.Type != participant.TypeAgent || h.agentLifecycleState(p.AgentID) != "archived" {
			return ""
		}
	}
	if bound {
		return "archived"
	}
	agentID := workerAgentIDFromUserID(user.ID)
	if h.legacyChatUserMatchesAgent(user, agentID) {
		return h.agentLifecycleState(agentID)
	}
	return ""
}
