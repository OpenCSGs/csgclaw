package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"csgclaw/internal/activity"
	"csgclaw/internal/im"
)

type UserInputResponder = activity.UserInputResponder

const (
	userInputTranscriptMetadataKey = "csgclaw"
	userInputTranscriptRequestKey  = "request_user_input"
	userInputTranscriptAnswerKind  = "answer"
)

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
	var req activity.RequestUserInputResponse
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(w, "decode request: expected one JSON object", http.StatusBadRequest)
		return
	}
	if req.Answers == nil {
		http.Error(w, "answers is required", http.StatusBadRequest)
		return
	}
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}
	pending, ok := h.userInputResponder.Get(activityID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if pending.Channel != "" && pending.Channel != channel {
		http.NotFound(w, r)
		return
	}
	roomID := strings.TrimSpace(pending.RoomID)
	room, ok := h.im.Room(roomID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	responderID := strings.TrimSpace(h.im.Bootstrap().CurrentUserID)
	if _, ok := h.im.User(responderID); !ok || !slices.Contains(room.Members, responderID) {
		http.NotFound(w, r)
		return
	}

	snapshot, err := h.userInputResponder.Respond(r.Context(), activity.UserInputResponseRequest{
		Channel:          channel,
		ActivityID:       activityID,
		RoomID:           roomID,
		ResponderID:      responderID,
		Response:         req,
		RecordTranscript: h.recordUserInputTranscript,
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

func (h *Handler) recordUserInputTranscript(ctx context.Context, snapshot activity.UserInputSnapshot) error {
	if h == nil || h.im == nil {
		return fmt.Errorf("IM service is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	content := activity.UserInputAnswerMarkdown(snapshot)
	if content == "" {
		return nil
	}
	_, err := h.im.DeliverMessage(im.DeliverMessageRequest{
		RoomID:       snapshot.RoomID,
		SenderID:     snapshot.ResponderID,
		Content:      content,
		MessageID:    "answer-" + snapshot.ID,
		ThreadRootID: snapshot.ThreadRootID,
		Metadata: map[string]any{
			userInputTranscriptMetadataKey: map[string]any{
				userInputTranscriptRequestKey: map[string]any{
					"kind":       userInputTranscriptAnswerKind,
					"request_id": snapshot.ID,
				},
			},
		},
	})
	return err
}

func isUserInputAnswerTranscript(message *im.Message) bool {
	if message == nil {
		return false
	}
	namespace, ok := message.Metadata[userInputTranscriptMetadataKey].(map[string]any)
	if !ok {
		return false
	}
	request, ok := namespace[userInputTranscriptRequestKey].(map[string]any)
	if !ok {
		return false
	}
	kind, _ := request["kind"].(string)
	return strings.TrimSpace(kind) == userInputTranscriptAnswerKind
}
