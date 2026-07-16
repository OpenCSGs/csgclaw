package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"csgclaw/internal/activity"
)

type activityDecisionRequest struct {
	OptionID string `json:"option_id"`
}

type ActivityDecider = activity.ActivityDecider

func (h *Handler) handleChannelActivityAction(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimSpace(pathValue(r, "activity_action"))
	switch {
	case strings.HasSuffix(action, ":decide"):
		h.handleChannelActivityDecision(w, r)
	case strings.HasSuffix(action, ":respond"):
		h.handleChannelUserInputResponse(w, r)
	default:
		http.NotFound(w, r)
	}
}

func channelActivityID(r *http.Request, actionSuffix string) string {
	if activityID := strings.TrimSpace(pathValue(r, "activity_id")); activityID != "" {
		return activityID
	}
	action := strings.TrimSpace(pathValue(r, "activity_action"))
	return strings.TrimSpace(strings.TrimSuffix(action, actionSuffix))
}

func (h *Handler) handleChannelActivityDecision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.activityDecider == nil {
		http.Error(w, "activity decisions are not configured", http.StatusServiceUnavailable)
		return
	}

	channel := strings.TrimSpace(pathValue(r, "channel"))
	if channel == "" {
		http.Error(w, "channel is required", http.StatusBadRequest)
		return
	}
	activityID := channelActivityID(r, ":decide")
	if activityID == "" {
		http.Error(w, "activity id is required", http.StatusBadRequest)
		return
	}

	var req activityDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	snapshot, err := h.activityDecider.Decide(r.Context(), activity.ActivityDecisionRequest{
		Channel:    channel,
		ActivityID: activityID,
		OptionID:   strings.TrimSpace(req.OptionID),
	})
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, snapshot)
	case errors.Is(err, activity.ErrActionInvalidOption):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, activity.ErrActionNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, activity.ErrActionAlreadyDecided):
		writeJSON(w, http.StatusConflict, snapshot)
	case errors.Is(err, activity.ErrActionGone):
		writeJSON(w, http.StatusGone, snapshot)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
