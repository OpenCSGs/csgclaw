package state

import (
	"strings"

	channel "csgclaw/internal/channel"
)

type ControlQuery struct {
	BindingID, AgentID, MessageID, ChatID, ThreadID, RequesterID string
}

type ControlTarget struct {
	Intent channel.DeliveryIntent
	Turn   channel.TurnRecord
}

// ResolveControlTarget reads the delivered message and its turn atomically.
// Action values never supply routing identities.
func (s *Store) ResolveControlTarget(q ControlQuery) (ControlTarget, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if q.MessageID == "" || q.ChatID == "" {
		return ControlTarget{}, false
	}
	for index := len(s.deliveryOrder) - 1; index >= 0; index-- {
		item := s.deliveries[s.deliveryOrder[index]]
		if item.Kind != channel.DeliveryCard && item.Kind != channel.DeliveryCOTCreate {
			continue
		}
		if item.Status != channel.DeliveryDelivered || item.BindingID != q.BindingID || item.MessageID != q.MessageID {
			continue
		}
		if item.ChatID != q.ChatID || (q.ThreadID != "" && item.ThreadID != q.ThreadID) || (item.RequesterID != "" && item.RequesterID != q.RequesterID) {
			return ControlTarget{}, false
		}
		record, found := s.turns[item.TurnID]
		if !found || record.BindingID != q.BindingID || record.AgentID != q.AgentID || strings.TrimSpace(record.ConversationKey) == "" {
			return ControlTarget{}, false
		}
		return ControlTarget{Intent: cloneIntent(item), Turn: record}, true
	}
	return ControlTarget{}, false
}

func (s *Store) MarkCanceling(turnID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.turns[turnID]
	if ok && (record.Status == channel.TurnAccepted || record.Status == channel.TurnRunning) {
		record.Status = channel.TurnCanceling
		s.turns[turnID] = record
	}
}
