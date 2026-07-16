package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"csgclaw/internal/activity"
)

type UserInputResponder = activity.UserInputResponder

type userInputResponseRequest struct {
	RoomID      string                              `json:"room_id"`
	ResponderID string                              `json:"responder_id"`
	Answers     map[string]activity.UserInputAnswer `json:"answers,omitempty"`
	SkipAll     bool                                `json:"skip_all,omitempty"`
}

func (h *Handler) handleChannelUserInputResponse(w http.ResponseWriter, r *http.Request) {
	if h.userInputResponder == nil {
		http.Error(w, "user input responses are not configured", http.StatusServiceUnavailable)
		return
	}
	channel := strings.TrimSpace(pathValue(r, "channel"))
	activityID := channelActivityID(r, ":respond")
	if channel == "" || activityID == "" {
		http.Error(w, "channel and activity id are required", http.StatusBadRequest)
		return
	}
	var req userInputResponseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	req.RoomID = strings.TrimSpace(req.RoomID)
	req.ResponderID = strings.TrimSpace(req.ResponderID)
	if req.RoomID == "" || req.ResponderID == "" {
		http.Error(w, "room_id and responder_id are required", http.StatusBadRequest)
		return
	}
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}
	room, ok := h.im.Room(req.RoomID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if _, ok := h.im.User(req.ResponderID); !ok || !slices.Contains(room.Members, req.ResponderID) {
		http.NotFound(w, r)
		return
	}

	snapshot, err := h.userInputResponder.Respond(r.Context(), activity.UserInputResponseRequest{
		Channel:     channel,
		ActivityID:  activityID,
		RoomID:      req.RoomID,
		ResponderID: req.ResponderID,
		Answers:     req.Answers,
		SkipAll:     req.SkipAll,
	})
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, snapshot)
	case errors.Is(err, activity.ErrUserInputInvalidResponse):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, activity.ErrUserInputNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, activity.ErrUserInputAlreadyResolved):
		writeJSON(w, http.StatusConflict, snapshot)
	case errors.Is(err, activity.ErrUserInputGone):
		writeJSON(w, http.StatusGone, snapshot)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
