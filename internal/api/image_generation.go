package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/delivery"
	"csgclaw/internal/im"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) retryImageGeneration(w http.ResponseWriter, r *http.Request) {
	if h.im == nil || h.participant == nil || h.agentEngine == nil {
		http.Error(w, "image delivery unavailable", http.StatusServiceUnavailable)
		return
	}

	var input struct {
		RoomID string `json:"room_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	messages, err := h.im.ListMessagesWithOptions(input.RoomID, im.ListMessagesOptions{IncludeThreadReplies: true})
	if err != nil {
		http.Error(w, "room not found", 404)
		return
	}
	var task contract.ImageGenerationTask
	var turn channel.TurnContext
	found := false
	for _, message := range messages {
		if message.ID != chi.URLParam(r, "message_id") {
			continue
		}
		body, _ := json.Marshal(message.Metadata["image_generation"])
		route, _ := json.Marshal(message.Metadata["image_generation_context"])
		if json.Unmarshal(body, &task) != nil || json.Unmarshal(route, &turn) != nil || task.ID == "" || turn.RoomID != input.RoomID {
			break
		}
		participant, ok := h.participant.Get("csgclaw", turn.ParticipantID)
		if !ok || participant.ChannelUserRef != message.SenderID || participant.AgentID != turn.AgentID {
			break
		}
		found = true
		break
	}
	if caller := strings.TrimSpace(r.Header.Get("X-CSGClaw-Caller-Agent")); caller != "" && agent.CanonicalID(caller) != turn.AgentID {
		http.Error(w, "image task belongs to another agent", http.StatusForbidden)
		return
	}
	if !found {
		http.Error(w, "image task not found", 404)
		return
	}
	if task.State != "failed" && task.State != "delivery_failed" {
		http.Error(w, "image task cannot be retried", 409)
		return
	}
	selected, err := h.agentEngine.Agents().Get(r.Context(), turn.AgentID, agentengine.AgentGetOptions{})
	if err != nil {
		http.Error(w, "agent unavailable", http.StatusNotFound)
		return
	}
	if selected.Spec.Runtime.Options["execution_mode"] == "read_only" {
		http.Error(w, "image generation is unavailable in read-only mode", http.StatusForbidden)
		return
	}
	digest := sha256.Sum256([]byte(string(turn.TurnID) + "\x00" + task.ID))
	turn.TurnID = agentengine.TurnID("image-retry-" + hex.EncodeToString(digest[:16]))
	store, err := delivery.NewIMTranscriptStore(h.im, h.participant, h.agentEngine)
	if err != nil {
		http.Error(w, "image delivery unavailable", 503)
		return
	}
	renderer := delivery.NewTranscriptRenderer(store)
	result := h.agentEngine.Conversations(turn.AgentID).Run(r.Context(), agentengine.TurnRequest{
		ID: turn.TurnID, ConversationKey: turn.ConversationKey, Input: []agentengine.InputPart{{Kind: agentengine.InputPartText, Text: task.Prompt}},
		Admission: agentengine.AdmissionRejectIfBusy, ImageGeneration: &task,
	}, contract.ImageGenerationSink{EventSink: agentengine.EventSinkFunc(func(ctx context.Context, event agentengine.TurnEvent) error { return renderer.Emit(ctx, turn, event) })})
	writeJSON(w, http.StatusOK, result)
}
