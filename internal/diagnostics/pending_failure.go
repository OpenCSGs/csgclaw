package diagnostics

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"
)

type pendingFailure struct {
	created time.Time
	entries []ContextEntry
	bytes   int
	dropped bool
}

const maxPendingFailureBytes = 16 << 20

func nativeFailureKey(agent, session, turn string) string {
	return agent + "\x00" + session + "\x00" + turn
}

// Only failure-ingress evidence needs this short-lived bridge. Large model
// bodies already have their own recorder. Keys always contain exact native IDs.
func (s *Store) deferNativeFailure(agent, session, turn, kind string, value any) bool {
	if agent == "" || session == "" || turn == "" {
		return false
	}
	raw := Protect(value)
	now := time.Now().UTC()
	key := nativeFailureKey(agent, session, turn)
	s.mu.Lock()
	if record := s.nativeFailureRecordLocked(agent, session, turn); record != nil {
		s.mu.Unlock()
		record.Capture(kind, raw)
		return true
	}
	if s.pendingFailures == nil {
		s.pendingFailures = map[string]*pendingFailure{}
	}
	for id, item := range s.pendingFailures {
		if time.Since(item.created) > time.Minute {
			s.pendingFailureBytes -= item.bytes
			delete(s.pendingFailures, id)
		}
	}
	item := s.pendingFailures[key]
	if item == nil {
		if len(s.pendingFailures) >= 128 {
			s.mu.Unlock()
			return false
		}
		item = &pendingFailure{created: now}
		s.pendingFailures[key] = item
		current := item
		time.AfterFunc(time.Minute, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.pendingFailures[key] == current {
				s.pendingFailureBytes -= current.bytes
				delete(s.pendingFailures, key)
			}
		})
	}
	if s.pendingFailureBytes+len(raw) > maxPendingFailureBytes || len(item.entries) >= 64 {
		item.dropped = true
		s.mu.Unlock()
		return true
	}
	item.entries = append(item.entries, ContextEntry{At: now, Kind: kind, Data: json.RawMessage(raw)})
	item.bytes += len(raw)
	s.pendingFailureBytes += len(raw)
	s.mu.Unlock()
	return true
}
func (s *Store) nativeFailureRecordLocked(agent, session, turn string) *Record {
	var found *Record
	for _, record := range s.records {
		record.mu.Lock()
		matches := record.data.AgentID == agent && record.data.RuntimeSessionID == session && record.data.RuntimeTurnID == turn
		record.mu.Unlock()
		if matches {
			if found != nil {
				return nil
			}
			found = record
		}
	}
	return found
}
func (s *Store) flushNativeFailures(record *Record, agent, session, turn string) {
	if session == "" || turn == "" {
		return
	}
	key := nativeFailureKey(agent, session, turn)
	s.mu.Lock()
	if s.nativeFailureRecordLocked(agent, session, turn) != record {
		s.mu.Unlock()
		return
	}
	item := s.pendingFailures[key]
	delete(s.pendingFailures, key)
	if item != nil {
		s.pendingFailureBytes -= item.bytes
	}
	s.mu.Unlock()
	if item == nil {
		return
	}
	if item.dropped {
		record.Issue("early_native_failure_evidence_dropped")
	}
	for _, entry := range item.entries {
		record.captureAt(entry.Kind, entry.Data, entry.At)
		if entry.Kind == "execution.failure" {
			slog.WarnContext(WithRecord(context.Background(), record), "turn operation failed after native binding", "occurred_at", entry.At, "details", entry.Data)
		}
	}
}
func deferrableFailureKind(kind string) bool {
	return strings.HasPrefix(kind, "llm.ingress") || kind == "execution.failure" || kind == "llm.failed_request"
}
