package api

import (
	"context"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/conv"
	"csgclaw/internal/modelcap"
	"csgclaw/internal/participant"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"
)

func (h *Handler) compactRoomContext(w http.ResponseWriter, r *http.Request) {
	if h.im == nil || h.agentEngine == nil || h.participant == nil {
		writeCodedAPIError(w, 503, "context_compaction_unavailable", "Context compaction is unavailable")
		return
	}
	var req struct {
		AgentIDs  []string `json:"agent_ids"`
		RequestID string   `json:"request_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&req) != nil || len(req.AgentIDs) == 0 || len(req.AgentIDs) > 100 || strings.TrimSpace(req.RequestID) == "" || len(req.RequestID) > 128 {
		writeCodedAPIError(w, 400, "invalid_request", "Agent IDs and request ID are required")
		return
	}
	h.compactionMu.Lock()
	defer h.compactionMu.Unlock()
	room, ok := h.im.Room(pathValue(r, "id"))
	if !ok {
		writeCodedAPIError(w, 404, "room_not_found", "Room not found")
		return
	}
	if room.CloudAgent != nil || !slices.Contains(room.Members, h.im.CurrentUserID()) {
		writeCodedAPIError(w, 403, "context_compaction_forbidden", "Room is not accessible")
		return
	}
	op := modelcap.CompactionOperation{ID: req.RequestID, Epoch: room.ContextUsageEpoch}
	keys := map[string]agentengine.ConversationKey{}
	seen := map[string]bool{}
	for _, id := range req.AgentIDs {
		if seen[id] {
			writeCodedAPIError(w, 400, "invalid_request", "Duplicate agent ID")
			return
		}
		seen[id] = true
		selected, err := h.agentEngine.Agents().Get(r.Context(), id, agentengine.AgentGetOptions{})
		if err != nil || selected.Spec.Runtime.Sandboxed || (selected.Spec.Runtime.Adapter != "codex" && selected.Spec.Runtime.Adapter != "dsh") {
			writeCodedAPIError(w, 400, "invalid_request", "Agent does not support context compaction")
			return
		}
		userID, binding := "", ""
		for _, p := range h.participant.List(participant.ListOptions{Channel: participant.ChannelCSGClaw}) {
			if p.AgentID == id {
				user, exists := h.im.User(p.ChannelUserRef)
				if exists && slices.Contains(room.Members, user.ID) {
					userID = user.ID
					binding = p.ID
					break
				}
			}
		}
		if userID == "" {
			writeCodedAPIError(w, 403, "context_compaction_forbidden", "Agent is not a room member")
			return
		}
		key, _ := conv.ConversationKey(channel.Binding{ID: binding}, channel.Event{RoomID: room.ID})
		keys[id] = key
		usage, exists := room.ContextUsage[userID]
		target := modelcap.CompactionTarget{AgentID: id, UserID: userID, Name: selected.Spec.Name, Status: "pending"}
		if exists {
			target.Before = modelcap.CloneUsage(&usage)
		}
		if !exists || usage.UsedTokens == nil {
			target.Status = "unknown"
		} else if !modelcap.ManualCompactionEligible(&usage) {
			target.Status = "unnecessary"
		}
		op.Targets = append(op.Targets, target)
	}
	op, created, err := h.im.StartContextCompaction(room.ID, op)
	if err != nil {
		writeCodedAPIError(w, 409, "context_compaction_conflict", "Compaction state changed; refresh the conversation")
		return
	}
	payload, _ := json.Marshal(op)
	if created {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Minute)
		go func() {
			if h.eventStreamShutdown != nil {
				go func() {
					select {
					case <-h.eventStreamShutdown:
						cancel()
					case <-ctx.Done():
					}
				}()
			}
			defer cancel()
			h.runRoomCompaction(ctx, room.ID, keys, op)
		}()
	}
	w.Header().Set("Content-Type", "application/json")
	if created {
		w.WriteHeader(http.StatusAccepted)
	}
	_, _ = w.Write(payload)
}
func (h *Handler) runRoomCompaction(ctx context.Context, roomID string, keys map[string]agentengine.ConversationKey, op modelcap.CompactionOperation) {
	for i := range op.Targets {
		target := &op.Targets[i]
		if target.Status != "pending" {
			continue
		}
		room, ok := h.im.Room(roomID)
		if !ok || room.ContextUsageEpoch != op.Epoch {
			return
		}
		if !slices.Contains(room.Members, target.UserID) {
			target.Status = "unavailable"
			target.Reason = modelcap.CompactionMemberRemoved
			continue
		}
		currentUsage, exists := room.ContextUsage[target.UserID]
		if !exists || currentUsage.UsedTokens == nil {
			target.Status = "unknown"
			continue
		}
		target.Before = modelcap.CloneUsage(&currentUsage)
		if !modelcap.ManualCompactionEligible(target.Before) {
			target.Status = "unnecessary"
			continue
		}
		target.Status = "running"
		if h.im.UpdateContextCompaction(roomID, op) != nil {
			return
		}
		compactor, ok := h.agentEngine.Conversations(target.AgentID).(contract.ConversationCompactor)
		if !ok {
			target.Status = "unsupported"
			continue
		}
		request := contract.CompactRequest{ConversationKey: keys[target.AgentID], Usage: *target.Before}
		if room.Configuration != nil {
			request.RoomConfiguration = &contract.RoomConfiguration{RoomID: room.ID, Configuration: *room.Configuration}
		}
		result, err := compactor.Compact(ctx, request)
		if err != nil {
			target.Status = "failed"
			if nativeErr, ok := err.(*contract.TurnError); ok {
				switch nativeErr.Code {
				case contract.ErrorAgentUnavailable:
					target.Status = "unavailable"
					target.Reason = modelcap.CompactionAgentUnavailable
				case contract.ErrorConversationNotResumable:
					target.Status = "unavailable"
					target.Reason = modelcap.CompactionRestoreFailed
				case contract.ErrorConversationBusy:
					target.Status = "busy"
				}
			}
			if ctx.Err() != nil {
				target.Status = "interrupted"
			}
		} else {
			target.Status = result.Status
			target.Reason = result.Reason
			target.After = modelcap.CloneUsage(result.Usage)
		}
		// Recheck epoch after the asynchronous boundary before saving any counters.
		if current, exists := h.im.Room(roomID); !exists || current.ContextUsageEpoch != op.Epoch {
			return
		}
		if target.After != nil {
			if _, err := h.im.RecordCompactedContextUsage(roomID, target.UserID, *target.After, op.Epoch); err != nil {
				target.Status = "uncertain"
			}
		}
		if h.im.UpdateContextCompaction(roomID, op) != nil {
			return
		}
	}
	op.Running = false
	op.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_ = h.im.UpdateContextCompaction(roomID, op)
}
