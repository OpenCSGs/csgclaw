package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentsession"
)

type agentSessionInteraction struct {
	InteractionID string                               `json:"interaction_id"`
	Status        activity.UserInputStatus             `json:"status"`
	Questions     []activity.UserInputQuestionSnapshot `json:"questions"`
	Detached      bool                                 `json:"detached"`
	RequestedAt   time.Time                            `json:"requested_at"`
	AutoResolveAt *time.Time                           `json:"auto_resolve_at,omitempty"`
}

func sessionUserInput(request agentengine.InteractionRequest) (agentSessionInteraction, bool) {
	snapshot, ok := request.Payload.(activity.UserInputSnapshot)
	if !ok || request.Kind != agentengine.InteractionUserInput {
		return agentSessionInteraction{}, false
	}
	return agentSessionInteraction{
		InteractionID: request.ID,
		Status:        snapshot.Status,
		Questions:     snapshot.Questions,
		Detached:      request.Detached,
		RequestedAt:   snapshot.RequestedAt,
		AutoResolveAt: snapshot.AutoResolveAt,
	}, true
}

func (h *Handler) existingInteractionSession(w http.ResponseWriter, r *http.Request) (agentengine.Agent, agentsession.Binding, bool) {
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
		writeSessionInputError(w, errSessionAuthorization)
		return agentengine.Agent{}, agentsession.Binding{}, false
	}
	sessionID := pathValue(r, "session_id")
	if !agentSessionIDPattern.MatchString(sessionID) {
		writeAgentSessionError(w, http.StatusBadRequest, "invalid_session_id", "session_id must be 1-128 path-safe ASCII characters", "session_id")
		return agentengine.Agent{}, agentsession.Binding{}, false
	}
	if h.agentEngine == nil || h.sessionBindings == nil {
		writeAgentSessionError(w, http.StatusServiceUnavailable, "session_service_unavailable", "session conversation service is not configured", nil)
		return agentengine.Agent{}, agentsession.Binding{}, false
	}
	selected, ok := h.sessionRequestAgent(w, r)
	if !ok {
		return agentengine.Agent{}, agentsession.Binding{}, false
	}
	binding, ok := h.sessionBindings.Lookup(selected.ID, sessionID)
	if !ok {
		writeAgentSessionError(w, http.StatusNotFound, "session_not_found", "session was not found", "session_id")
		return agentengine.Agent{}, agentsession.Binding{}, false
	}
	return selected, binding, true
}

func (h *Handler) listAgentSessionInteractions(w http.ResponseWriter, r *http.Request) {
	selected, binding, ok := h.existingInteractionSession(w, r)
	if !ok {
		return
	}
	requests, err := h.agentEngine.Conversations(selected.ID).PendingInteractions(r.Context(), agentengine.ConversationKey(binding.ConversationKey))
	if err != nil {
		writeSessionInteractionError(w, err)
		return
	}
	pending := make([]agentSessionInteraction, 0, len(requests))
	now := time.Now()
	for _, request := range requests {
		item, ok := sessionUserInput(request)
		if !ok {
			continue
		}
		if item.Status == activity.UserInputStatusSubmitting || (item.Status == activity.UserInputStatusPending && (item.AutoResolveAt == nil || now.Before(*item.AutoResolveAt))) {
			pending = append(pending, item)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"interactions": pending})
}

func (h *Handler) respondAgentSessionInteraction(w http.ResponseWriter, r *http.Request) {
	selected, binding, ok := h.existingInteractionSession(w, r)
	if !ok {
		return
	}
	conversation := h.agentEngine.Conversations(selected.ID)
	key := agentengine.ConversationKey(binding.ConversationKey)
	id := pathValue(r, "interaction_id")
	request, err := conversation.GetInteraction(r.Context(), key, id)
	if err != nil {
		writeSessionInteractionError(w, err)
		return
	}
	if _, ok := sessionUserInput(request); !ok {
		writeAgentSessionError(w, http.StatusNotFound, "interaction_not_found", "user input request was not found", "interaction_id")
		return
	}
	var input struct {
		activity.RequestUserInputResponse
		Stream bool `json:"stream,omitempty"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, agentSessionResponseBodyLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeSessionInteractionInputError(w, err)
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		writeSessionInteractionInputError(w, err)
		return
	}
	if input.Answers == nil {
		writeAgentSessionError(w, http.StatusBadRequest, "invalid_request", "answers is required", "answers")
		return
	}
	resolution := agentengine.InteractionResolution{
		ConversationKey: key,
		InteractionID:   id,
		ResponderID:     "session-api",
		Answers:         make(map[string]agentengine.InteractionAnswer, len(input.Answers)),
	}
	for questionID, answer := range input.Answers {
		if answer.Answers == nil {
			writeAgentSessionError(w, http.StatusBadRequest, "invalid_request", "each question answer must include an answers array", "answers")
			return
		}
		resolution.Answers[questionID] = agentengine.InteractionAnswer{Values: answer.Answers, Skipped: len(answer.Answers) == 0}
	}
	var writeResult func()
	if request.Detached {
		resolution.ContinueDetached = func(ctx context.Context, answer agentengine.InteractionRequest) bool {
			prompt, err := activity.UserInputContinuationPrompt(answer.Payload.(activity.UserInputSnapshot))
			if err != nil {
				writeResult = func() {
					writeAgentSessionError(w, http.StatusInternalServerError, "invalid_interaction", "user input continuation could not be prepared", nil)
				}
				return false
			}
			continuation := agentSessionInput{Text: prompt, userInput: true, respondingTo: id}
			var delivered bool
			delivered, writeResult = h.runSessionResponse(ctx, w, r, selected, binding, continuation, input.Stream, nil, agentengine.AdmissionWait)
			return delivered
		}
	}
	err = conversation.Resolve(r.Context(), resolution)
	if writeResult != nil {
		writeResult()
		return
	}
	if err != nil {
		writeSessionInteractionError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeSessionInteractionInputError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeAgentSessionError(w, http.StatusRequestEntityTooLarge, "invalid_request", "answer submission exceeds the request body limit", "answers")
		return
	}
	// Decoder errors can include a submitted value used as a JSON field name.
	writeAgentSessionError(w, http.StatusBadRequest, "invalid_request", "invalid answer submission JSON", "answers")
}

func writeSessionInteractionError(w http.ResponseWriter, err error) {
	code := agentengine.ErrorCodeOf(err)
	switch code {
	case agentengine.ErrorInvalidRequest:
		writeAgentSessionError(w, http.StatusBadRequest, string(code), err.Error(), "answers")
	case agentengine.ErrorInteractionNotFound:
		writeAgentSessionError(w, http.StatusNotFound, string(code), err.Error(), "interaction_id")
	case agentengine.ErrorInteractionAlreadyResolved:
		writeAgentSessionError(w, http.StatusConflict, string(code), err.Error(), "interaction_id")
	case agentengine.ErrorInteractionGone:
		writeAgentSessionError(w, http.StatusGone, string(code), err.Error(), "interaction_id")
	default:
		writeAgentSessionError(w, http.StatusServiceUnavailable, "interaction_unavailable", "user input request is unavailable", nil)
	}
}
