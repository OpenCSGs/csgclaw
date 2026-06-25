package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/participant"
)

func (h *Handler) handleParticipants(w http.ResponseWriter, r *http.Request) {
	if h.participant == nil {
		http.Error(w, "participant service is not configured", http.StatusServiceUnavailable)
		return
	}
	channelName := pathValue(r, "channel")
	if channelName == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		items := h.participant.List(participant.ListOptions{
			Channel: channelName,
			Type:    r.URL.Query().Get("type"),
			AgentID: r.URL.Query().Get("agent_id"),
		})
		writeJSON(w, http.StatusOK, h.presentParticipants(items))
	case http.MethodPost:
		var req participant.CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		req.Channel = channelName
		created, err := h.participant.Create(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, h.presentParticipant(created))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleParticipantByIDPath(w http.ResponseWriter, r *http.Request) {
	if h.participant == nil {
		http.Error(w, "participant service is not configured", http.StatusServiceUnavailable)
		return
	}
	channelName := pathValue(r, "channel")
	id := pathValue(r, "id")
	if channelName == "" || id == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		item, ok := h.participant.Get(channelName, id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, h.presentParticipant(item))
	case http.MethodPatch:
		var req participant.UpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		updated, ok, err := h.participant.Update(r.Context(), channelName, id, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, h.presentParticipant(updated))
	case http.MethodDelete:
		deleted, ok, err := h.participant.Delete(r.Context(), channelName, id, participant.DeleteOptions{
			DeleteAgent: r.URL.Query().Get("delete_agent"),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		if err := h.recreateFeishuAgentAfterDisconnect(r.Context(), deleted, r.URL.Query().Get("delete_agent")); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) recreateFeishuAgentAfterDisconnect(ctx context.Context, deleted apitypes.Participant, deleteAgentMode string) error {
	if !strings.EqualFold(strings.TrimSpace(deleted.Channel), participant.ChannelFeishu) {
		return nil
	}
	if strings.TrimSpace(deleteAgentMode) != "" {
		return nil
	}
	if strings.TrimSpace(deleted.Type) != participant.TypeAgent {
		return nil
	}
	agentID := strings.TrimSpace(deleted.AgentID)
	if agentID == "" {
		return nil
	}
	if h == nil || h.svc == nil {
		return fmt.Errorf("agent service is required to disconnect feishu participant %q", deleted.ID)
	}
	if h.participant != nil {
		for _, item := range h.participant.List(participant.ListOptions{
			Channel: participant.ChannelFeishu,
			Type:    participant.TypeAgent,
			AgentID: agentID,
		}) {
			if strings.TrimSpace(item.ID) == strings.TrimSpace(deleted.ID) {
				continue
			}
			if strings.TrimSpace(item.ChannelUserKind) != participant.ChannelUserKindAppID {
				continue
			}
			if _, _, err := h.participant.Delete(ctx, participant.ChannelFeishu, item.ID, participant.DeleteOptions{}); err != nil {
				return fmt.Errorf("delete feishu participant %q for agent %q: %w", item.ID, agentID, err)
			}
		}
	}
	if _, err := h.svc.Recreate(ctx, agentID); err != nil {
		return fmt.Errorf("recreate agent %q after disconnecting feishu participant %q: %w", agentID, deleted.ID, err)
	}
	return nil
}

func (h *Handler) handleParticipantEvents(w http.ResponseWriter, r *http.Request) {
	channelName := participantChannelName(pathValue(r, "channel"))
	id := pathValue(r, "id")
	if channelName == "" || id == "" {
		http.NotFound(w, r)
		return
	}

	switch channelName {
	case "csgclaw":
		participantID, ok := h.requireParticipantBridgeID(w, r, h.resolveParticipantBridgeID(channelName, id))
		if !ok {
			return
		}
		h.handleParticipantEventsStream(w, r, participantID)
	case "feishu":
		h.handleFeishuParticipantEvents(w, r, id, h.resolveFeishuParticipantTargetID(id))
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleParticipantMessage(w http.ResponseWriter, r *http.Request) {
	channelName := participantChannelName(pathValue(r, "channel"))
	id := pathValue(r, "id")
	if channelName == "" || id == "" {
		http.NotFound(w, r)
		return
	}
	if channelName != "csgclaw" {
		http.NotFound(w, r)
		return
	}
	participantID := h.resolveParticipantChannelUserID(channelName, id)
	participantID, ok := h.requireParticipantBridgeID(w, r, participantID)
	if !ok {
		return
	}
	h.handleParticipantSendMessage(w, r, participantID)
}

func (h *Handler) resolveParticipantChannelUserID(channelName, id string) string {
	id = strings.TrimSpace(id)
	if h != nil && h.participant != nil {
		if item, ok := h.participant.Get(channelName, id); ok {
			return participantChannelLocalIdentity(item)
		}
		if strings.EqualFold(channelName, participant.ChannelCSGClaw) {
			for _, item := range h.participant.List(participant.ListOptions{Channel: channelName}) {
				if !isCSGClawAgentParticipant(item) || !participantMatchesIdentity(item, id) {
					continue
				}
				return participantChannelLocalIdentity(item)
			}
		}
	}
	if strings.EqualFold(channelName, participant.ChannelCSGClaw) {
		return csgclawParticipantIDFromAny(id)
	}
	return id
}

func (h *Handler) resolveParticipantBridgeID(channelName, id string) string {
	id = strings.TrimSpace(id)
	if h != nil && h.participant != nil {
		if item, ok := h.participant.Get(channelName, id); ok && isCSGClawAgentParticipant(item) {
			return strings.TrimSpace(item.ID)
		}
		if strings.EqualFold(channelName, participant.ChannelCSGClaw) {
			for _, item := range h.participant.List(participant.ListOptions{Channel: channelName}) {
				if !isCSGClawAgentParticipant(item) || !participantMatchesIdentity(item, id) {
					continue
				}
				return strings.TrimSpace(item.ID)
			}
		}
	}
	if id == agent.ManagerUserID {
		return agent.ManagerParticipantID
	}
	if strings.EqualFold(channelName, participant.ChannelCSGClaw) {
		return csgclawParticipantIDFromAny(id)
	}
	return id
}

func (h *Handler) resolveFeishuParticipantTargetID(id string) string {
	id = strings.TrimSpace(id)
	if h != nil && h.participant != nil {
		if item, ok := h.participant.Get(participant.ChannelFeishu, id); ok {
			return participantChannelUserOrID(item)
		}
		for _, item := range h.participant.List(participant.ListOptions{Channel: participant.ChannelFeishu}) {
			if !participantMatchesIdentity(item, id) {
				continue
			}
			return participantChannelUserOrID(item)
		}
	}
	return id
}

func participantChannelUserOrID(item apitypes.Participant) string {
	if ref := strings.TrimSpace(item.ChannelUserRef); ref != "" {
		return ref
	}
	return strings.TrimSpace(item.ID)
}

func participantChannelLocalIdentity(item apitypes.Participant) string {
	if strings.EqualFold(strings.TrimSpace(item.Channel), participant.ChannelCSGClaw) {
		if id := strings.TrimSpace(item.ID); id != "" {
			return id
		}
	}
	return participantChannelUserOrID(item)
}

func presentParticipants(items []apitypes.Participant) []apitypes.Participant {
	out := make([]apitypes.Participant, 0, len(items))
	for _, item := range items {
		out = append(out, presentParticipant(item))
	}
	return out
}

func presentParticipant(item apitypes.Participant) apitypes.Participant {
	if len(item.ChannelAppConfig) == 0 {
		return item
	}
	item.ChannelAppConfig = participant.RedactChannelAppConfig(item.ChannelAppConfig)
	return item
}

func (h *Handler) presentParticipants(items []apitypes.Participant) []apitypes.Participant {
	out := make([]apitypes.Participant, 0, len(items))
	for _, item := range items {
		out = append(out, h.presentParticipant(item))
	}
	return out
}

func (h *Handler) presentParticipant(item apitypes.Participant) apitypes.Participant {
	item = presentParticipant(item)
	if h == nil {
		return item
	}
	if strings.TrimSpace(item.AgentID) != "" {
		item.AgentID = agent.CanonicalID(item.AgentID)
	}
	if h.svc != nil {
		if name, ok := h.svc.AgentDisplayName(item.AgentID); ok {
			item.AgentName = name
		}
	}
	if item.UserID == "" {
		item.UserID = strings.TrimSpace(item.ChannelUserRef)
	}
	if h.im != nil && strings.TrimSpace(item.UserID) != "" {
		if user, ok := h.im.User(item.UserID); ok {
			item.UserName = strings.TrimSpace(user.Name)
		}
	}
	return item
}

func (h *Handler) requireParticipantBridgeID(w http.ResponseWriter, r *http.Request, id string) (string, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		http.NotFound(w, r)
		return "", false
	}
	if h.participantBridge == nil {
		http.Error(w, "picoclaw integration is not configured", http.StatusServiceUnavailable)
		return "", false
	}
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	return id, true
}

func participantChannelName(channel string) string {
	return strings.TrimSpace(channel)
}
