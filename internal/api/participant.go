package api

import (
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
		writeJSON(w, http.StatusOK, items)
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
		writeJSON(w, http.StatusCreated, created)
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
		writeJSON(w, http.StatusOK, item)
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
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		_, ok, err := h.participant.Delete(r.Context(), channelName, id, participant.DeleteOptions{
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
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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
		h.handleFeishuParticipantEvents(w, r, h.resolveFeishuParticipantTargetID(id))
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
			return participantChannelUserOrID(item)
		}
		if strings.EqualFold(channelName, participant.ChannelCSGClaw) {
			for _, item := range h.participant.List(participant.ListOptions{Channel: channelName}) {
				if !isCSGClawAgentParticipant(item) || !participantMatchesIdentity(item, id) {
					continue
				}
				return participantChannelUserOrID(item)
			}
		}
	}
	if id == agent.ManagerUserID {
		return agent.ManagerParticipantID
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
