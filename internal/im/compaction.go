package im

import (
	"csgclaw/internal/modelcap"
	"encoding/json"
	"errors"
	"slices"
	"time"
)

const compactionMetadataKey = "context_compaction"

var ErrCompactionConflict = errors.New("compaction request conflicts with an existing operation")

func MessageCompaction(m Message) *modelcap.CompactionOperation {
	if m.Kind != MessageKindEvent {
		return nil
	}
	raw, ok := m.Metadata[compactionMetadataKey]
	if !ok {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var op modelcap.CompactionOperation
	if json.Unmarshal(data, &op) != nil || op.ID == "" {
		return nil
	}
	return &op
}
func compactionMetadata(op modelcap.CompactionOperation) map[string]any {
	// Snapshot through JSON: callers and live events must never share target slices.
	data, _ := json.Marshal(op)
	var value any
	_ = json.Unmarshal(data, &value)
	return map[string]any{compactionMetadataKey: value}
}
func (s *Service) StartContextCompaction(roomID string, op modelcap.CompactionOperation) (modelcap.CompactionOperation, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[roomID]
	if !ok {
		return op, false, ErrRoomNotFound
	}
	if room.ContextUsageEpoch != op.Epoch {
		return op, false, ErrCompactionConflict
	}
	for _, m := range room.Messages {
		if previous := MessageCompaction(m); previous != nil && previous.ID == op.ID {
			if len(previous.Targets) != len(op.Targets) {
				return op, false, ErrCompactionConflict
			}
			for i, t := range op.Targets {
				if previous.Targets[i].AgentID != t.AgentID {
					return op, false, ErrCompactionConflict
				}
			}
			return *previous, false, nil
		}
	}
	// No-op single clicks stay in the panel. Batches retain all skip outcomes.
	if len(op.Targets) == 1 && op.Targets[0].Status != "pending" {
		return op, false, nil
	}
	op.Revision = 1
	op.Running = true
	op.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	message := s.newMessage("", s.currentUserID, MessageKindEvent, "Context compaction")
	message.MemoryExcluded = true
	message.Metadata = compactionMetadata(op)
	room.Messages = append(room.Messages, message)
	if err := s.saveRoomLocked(*room); err != nil {
		room.Messages = room.Messages[:len(room.Messages)-1]
		return op, false, err
	}
	s.publishMessageCreatedLocked(roomID, s.currentUserID, message)
	return op, true, nil
}
func (s *Service) UpdateContextCompaction(roomID string, op modelcap.CompactionOperation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[roomID]
	if !ok {
		return ErrRoomNotFound
	}
	if room.ContextUsageEpoch != op.Epoch {
		return ErrCompactionConflict
	}
	for i, m := range room.Messages {
		if old := MessageCompaction(m); old != nil && old.ID == op.ID {
			op.Revision = old.Revision + 1
			for j := range op.Targets {
				if !slices.Contains(room.Members, op.Targets[j].UserID) {
					op.Targets[j].Status = "unavailable"
					op.Targets[j].Reason = modelcap.CompactionMemberRemoved
				}
			}
			room.Messages[i].Metadata = compactionMetadata(op)
			if err := s.saveRoomLocked(*room); err != nil {
				room.Messages[i] = m
				return err
			}
			s.publishMessageCreatedLocked(roomID, m.SenderID, room.Messages[i])
			return nil
		}
	}
	return ErrCompactionConflict
}
func interruptSavedCompactions(messages []Message) bool {
	changed := false
	for i, m := range messages {
		if op := MessageCompaction(m); op != nil && op.Running {
			changed = true
			op.Revision++
			op.Running = false
			op.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
			for j := range op.Targets {
				if op.Targets[j].Status == "pending" || op.Targets[j].Status == "running" {
					op.Targets[j].Status = "interrupted"
				}
			}
			messages[i].Metadata = compactionMetadata(*op)
		}
	}
	return changed
}

// Only fill an unknown completion count for the exact original model/session.
func (s *Service) reconcileCompactionUsageLocked(room *Room, user string, usage modelcap.ContextUsage) error {
	if usage.UsedTokens == nil || usage.Compacting {
		return nil
	}
	for i, m := range room.Messages {
		op := MessageCompaction(m)
		if op == nil || op.Epoch != room.ContextUsageEpoch {
			continue
		}
		changed := false
		for j := range op.Targets {
			t := &op.Targets[j]
			if t.UserID == user && t.Status == "completed" && t.Before != nil && t.Before.SessionID == usage.SessionID && t.Before.ModelID == usage.ModelID && t.After != nil && t.After.UsedTokens == nil {
				t.After = modelcap.CloneUsage(&usage)
				changed = true
			}
		}
		if changed {
			op.Revision++
			room.Messages[i].Metadata = compactionMetadata(*op)
			if err := s.saveRoomLocked(*room); err != nil {
				room.Messages[i] = m
				return err
			}
			s.publishMessageCreatedLocked(room.ID, m.SenderID, room.Messages[i])
		}
	}
	return nil
}
