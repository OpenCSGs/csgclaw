package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel/feishu"
)

func (h *Handler) handleFeishuParticipantEvents(w http.ResponseWriter, r *http.Request, participantID, targetID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	targetIDs := []string{participantID, targetID}
	targetIDs = append(targetIDs, h.resolveFeishuParticipantEventOpenIDs(r.Context(), participantID, targetID)...)
	h.streamFeishuEvents(w, r, feishuEventTarget{IDs: targetIDs})
}

type feishuEventTarget struct {
	IDs []string
}

func (h *Handler) resolveFeishuParticipantEventOpenIDs(ctx context.Context, ids ...string) []string {
	if h == nil || h.feishu == nil {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		openID, _, err := h.feishu.ResolveBotOpenID(ctx, id)
		if err != nil {
			continue
		}
		openID = strings.TrimSpace(openID)
		if openID == "" || openID == id {
			continue
		}
		out = append(out, openID)
	}
	return out
}

func (h *Handler) streamFeishuEvents(w http.ResponseWriter, r *http.Request, target feishuEventTarget) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h.feishu == nil || h.feishu.MessageBus() == nil {
		http.Error(w, "feishu events are not configured", http.StatusServiceUnavailable)
		return
	}
	target.IDs = normalizedFeishuEventTargetIDs(target.IDs...)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	events, cancel := h.feishu.MessageBus().Subscribe()
	defer cancel()

	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()

	ticker := time.NewTicker(sseHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-h.eventStreamShutdown:
			return
		case <-ticker.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case evt, ok := <-events:
			if !ok {
				return
			}
			if !feishuEventMentions(evt, target) {
				continue
			}
			data, err := json.Marshal(evt)
			if err != nil {
				return
			}
			if _, err := io.WriteString(w, "data: "); err != nil {
				return
			}
			if _, err := w.Write(data); err != nil {
				return
			}
			if _, err := io.WriteString(w, "\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func feishuEventMentions(evt feishu.MessageEvent, target feishuEventTarget) bool {
	targetIDs := normalizedFeishuEventTargetIDs(target.IDs...)
	if len(targetIDs) == 0 {
		return false
	}
	if feishuEventTargetMatches(strings.TrimSpace(evt.MentionBotID), targetIDs) {
		return true
	}
	if evt.Message == nil {
		return false
	}
	for _, mention := range evt.Message.Mentions {
		if feishuEventTargetMatches(strings.TrimSpace(mention.ID), targetIDs) ||
			feishuEventTargetMatches(strings.TrimSpace(mention.Name), targetIDs) {
			return true
		}
	}
	return false
}

func normalizedFeishuEventTargetIDs(ids ...string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func feishuEventTargetMatches(value string, targetIDs []string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, id := range targetIDs {
		if value == id {
			return true
		}
	}
	return false
}

func (h *Handler) handleFeishuUsers(w http.ResponseWriter, r *http.Request) {
	if h.feishu == nil {
		http.Error(w, "feishu channel is not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, h.feishu.ListUsers())
	case http.MethodPost:
		var req feishu.CreateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		user, err := h.feishu.CreateUser(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, user)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleFeishuUserByID(w http.ResponseWriter, r *http.Request) {
	if h.feishu == nil {
		http.Error(w, "feishu channel is not configured", http.StatusServiceUnavailable)
		return
	}

	userID := pathValue(r, "id")
	if userID == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := h.feishu.DeleteUser(userID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeAPIError(w, err, http.StatusNotFound)
				return
			}
			writeAPIError(w, err, http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleFeishuRooms(w http.ResponseWriter, r *http.Request) {
	if h.feishu == nil {
		http.Error(w, "feishu channel is not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		rooms, err := h.feishu.ListRooms()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, rooms)
	case http.MethodPost:
		var req apitypes.CreateRoomRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		room, err := h.feishu.CreateRoom(req)
		if err != nil {
			writeAPIError(w, err, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, room)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleFeishuMessages(w http.ResponseWriter, r *http.Request) {
	if h.feishu == nil {
		http.Error(w, "feishu channel is not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		roomID, err := roomIDFromQuery(r)
		if err != nil {
			writeAPIError(w, err, http.StatusBadRequest)
			return
		}
		messages, err := h.feishu.ListRoomMessages(roomID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeAPIError(w, err, http.StatusNotFound)
				return
			}
			writeAPIError(w, err, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, messages)
	case http.MethodPost:
		var req apitypes.CreateMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		message, err := h.feishu.SendMessage(req)
		if err != nil {
			writeAPIError(w, err, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, message)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleFeishuRoomByID(w http.ResponseWriter, r *http.Request) {
	if h.feishu == nil {
		http.Error(w, "feishu channel is not configured", http.StatusServiceUnavailable)
		return
	}

	roomID := pathValue(r, "id")
	if roomID == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := h.feishu.DeleteRoom(roomID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeCodedAPIError(w, http.StatusNotFound, "room_not_found", err.Error())
				return
			}
			writeAPIError(w, err, http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleFeishuRoomMembersByID(w http.ResponseWriter, r *http.Request) {
	if h.feishu == nil {
		http.Error(w, "feishu channel is not configured", http.StatusServiceUnavailable)
		return
	}

	roomID := pathValue(r, "id")
	if roomID == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		members, err := h.feishu.ListRoomMembers(roomID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeCodedAPIError(w, http.StatusNotFound, "room_not_found", err.Error())
				return
			}
			writeAPIError(w, err, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, members)
	case http.MethodPost:
		var req apitypes.AddRoomMembersRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		req.RoomID = roomID
		room, err := h.feishu.AddRoomMembers(req)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeCodedAPIError(w, http.StatusNotFound, "room_not_found", err.Error())
				return
			}
			writeAPIError(w, err, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, room)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
