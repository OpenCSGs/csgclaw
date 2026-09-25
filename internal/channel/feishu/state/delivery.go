// Package state contains the small, process-local state needed by one Feishu
// binding. Nothing in this package is written to disk or recovered on restart.
package state

import (
	"fmt"
	"strings"
	"sync"
	"time"

	channeltypes "csgclaw/internal/channel"
)

const (
	maxTurnRecords     = 1024
	maxDeliveryRecords = 4096
)

// Store keeps transient turn correlation and delivery dependencies in memory.
// Agent Engine remains the authority for active conversations and Turn IDs.
type Store struct {
	cotFailures   map[string]bool
	pinned        map[string]bool
	mu            sync.RWMutex
	turns         map[string]channeltypes.TurnRecord
	turnOrder     []string
	deliveries    map[string]channeltypes.DeliveryIntent
	deliveryOrder []string
}

func NewStore() *Store {
	return &Store{
		turns:       make(map[string]channeltypes.TurnRecord),
		pinned:      make(map[string]bool),
		cotFailures: make(map[string]bool),
		deliveries:  make(map[string]channeltypes.DeliveryIntent),
	}
}

func (s *Store) Put(record channeltypes.TurnRecord) error {
	if s == nil || strings.TrimSpace(record.TurnID) == "" {
		return fmt.Errorf("feishu memory state: turn ID is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.turns[record.TurnID]
	s.turns[record.TurnID] = record
	if !exists {
		s.turnOrder = append(s.turnOrder, record.TurnID)
	}
	s.pruneTurnsLocked()
	return nil
}

func (s *Store) Get(turnID string) (channeltypes.TurnRecord, bool) {
	if s == nil {
		return channeltypes.TurnRecord{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.turns[strings.TrimSpace(turnID)]
	return record, ok
}

func (s *Store) BeginTurn(record channeltypes.TurnRecord) error {
	return s.Put(record)
}

func (s *Store) AppendTurnDeliveries(turnID string, sequence uint64, intents ...channeltypes.DeliveryIntent) error {
	if s == nil {
		return fmt.Errorf("feishu memory state is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.turns[strings.TrimSpace(turnID)]
	if sequence > record.LastSequence {
		record.LastSequence = sequence
	}
	if record.TurnID != "" {
		s.turns[record.TurnID] = record
	}
	for _, intent := range intents {
		if err := s.enqueueLocked(intent); err != nil {
			return err
		}
	}
	s.pruneDeliveriesLocked()
	return nil
}

func (s *Store) FinishTurn(turnID string, status channeltypes.TurnStatus, intents ...channeltypes.DeliveryIntent) error {
	if s == nil {
		return fmt.Errorf("feishu memory state is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, exists := s.turns[strings.TrimSpace(turnID)]
	record.TurnID = strings.TrimSpace(turnID)
	record.Status = status
	s.turns[record.TurnID] = record
	if !exists {
		s.turnOrder = append(s.turnOrder, record.TurnID)
	}
	for _, intent := range intents {
		if err := s.enqueueLocked(intent); err != nil {
			return err
		}
	}
	s.pruneTurnsLocked()
	s.pruneDeliveriesLocked()
	return nil
}

func (s *Store) Enqueue(intent channeltypes.DeliveryIntent) error {
	if s == nil {
		return fmt.Errorf("feishu memory state is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.enqueueLocked(intent); err != nil {
		return err
	}
	s.pruneDeliveriesLocked()
	return nil
}

func (s *Store) enqueueLocked(intent channeltypes.DeliveryIntent) error {
	intent.ID = strings.TrimSpace(intent.ID)
	if intent.ID == "" || strings.TrimSpace(intent.BindingID) == "" || strings.TrimSpace(intent.TurnID) == "" {
		return fmt.Errorf("feishu memory delivery: ID, binding ID, and turn ID are required")
	}
	if intent.Kind == channeltypes.DeliveryCOTComplete && len(intent.Events) != 0 {
		return fmt.Errorf("COT completion cannot contain append events")
	}
	if _, exists := s.deliveries[intent.ID]; exists {
		return nil
	}
	now := time.Now().UTC()
	intent.Status = channeltypes.DeliveryPending
	intent.CreatedAt = now
	s.deliveries[intent.ID] = cloneIntent(intent)
	if intent.Kind == channeltypes.DeliveryCOTCreate {
		s.pinned[intent.ID] = true
	}
	s.deliveryOrder = append(s.deliveryOrder, intent.ID)
	return nil
}

func (s *Store) Delivery(id string) (channeltypes.DeliveryIntent, bool) {
	if s == nil {
		return channeltypes.DeliveryIntent{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	intent, ok := s.deliveries[strings.TrimSpace(id)]
	return cloneIntent(intent), ok
}

func (s *Store) Pending() []channeltypes.DeliveryIntent {
	return s.byStatus(channeltypes.DeliveryPending, channeltypes.DeliveryDispatching)
}

func (s *Store) byStatus(statuses ...channeltypes.DeliveryStatus) []channeltypes.DeliveryIntent {
	if s == nil {
		return nil
	}
	wanted := make(map[channeltypes.DeliveryStatus]struct{}, len(statuses))
	for _, status := range statuses {
		wanted[status] = struct{}{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []channeltypes.DeliveryIntent
	for _, id := range s.deliveryOrder {
		intent := s.deliveries[id]
		if _, ok := wanted[intent.Status]; ok {
			out = append(out, cloneIntent(intent))
		}
	}
	return out
}

func (s *Store) BeginDelivery(id string) error {
	return s.updateDelivery(id, func(intent *channeltypes.DeliveryIntent) {
		intent.Status = channeltypes.DeliveryDispatching
	})
}

func (s *Store) MarkDelivered(intent channeltypes.DeliveryIntent) error {
	return s.updateDelivery(intent.ID, func(stored *channeltypes.DeliveryIntent) {
		stored.Status = channeltypes.DeliveryDelivered
		stored.MessageID = strings.TrimSpace(intent.MessageID)
		stored.COTID = strings.TrimSpace(intent.COTID)
		stored.ReactionID = strings.TrimSpace(intent.ReactionID)
		stored.NextAttemptAt = nil
		stored.LastError = ""
	})
}

func (s *Store) MarkFailed(id string, cause error) error {
	return s.updateDelivery(id, func(intent *channeltypes.DeliveryIntent) {
		intent.Status = channeltypes.DeliveryFailed
		if intent.Kind == channeltypes.DeliveryCOTCreate || intent.Kind == channeltypes.DeliveryCOTUpdate {
			s.cotFailures[intent.TurnID] = true
		}
		intent.Attempts++
		intent.NextAttemptAt = nil
		if cause != nil {
			intent.LastError = cause.Error()
		}
	})
}

func (s *Store) MarkRetryable(id string, cause error, next time.Time) error {
	return s.updateDelivery(id, func(intent *channeltypes.DeliveryIntent) {
		intent.Status = channeltypes.DeliveryPending
		intent.Attempts++
		intent.NextAttemptAt = &next
		if cause != nil {
			intent.LastError = cause.Error()
		}
	})
}

func (s *Store) updateDelivery(id string, update func(*channeltypes.DeliveryIntent)) error {
	if s == nil {
		return fmt.Errorf("feishu memory state is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	intent, ok := s.deliveries[id]
	if !ok {
		return fmt.Errorf("feishu delivery %q was not found", id)
	}
	update(&intent)
	s.deliveries[id] = intent
	s.pruneDeliveriesLocked()
	return nil
}

func (s *Store) pruneTurnsLocked() {
	if len(s.turns) <= maxTurnRecords {
		return
	}
	kept := s.turnOrder[:0]
	for _, id := range s.turnOrder {
		record, exists := s.turns[id]
		if !exists {
			continue
		}
		if len(s.turns) > maxTurnRecords && terminalTurn(record.Status) && !s.turnRequiredLocked(id) {
			delete(s.turns, id)
			continue
		}
		kept = append(kept, id)
	}
	s.turnOrder = kept
}

func (s *Store) pruneDeliveriesLocked() {
	if len(s.deliveries) <= maxDeliveryRecords {
		return
	}
	protected := make(map[string]struct{})
	for id := range s.pinned {
		protected[id] = struct{}{}
	}
	queue := make([]string, 0)
	for id, intent := range s.deliveries {
		if terminalDelivery(intent.Status) {
			continue
		}
		protected[id] = struct{}{}
		queue = append(queue, id)
	}
	for len(queue) > 0 {
		id := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		relatedID := strings.TrimSpace(s.deliveries[id].RelatedID)
		if relatedID == "" {
			continue
		}
		if _, exists := protected[relatedID]; exists {
			continue
		}
		if _, exists := s.deliveries[relatedID]; exists {
			protected[relatedID] = struct{}{}
			queue = append(queue, relatedID)
		}
	}

	kept := s.deliveryOrder[:0]
	for _, id := range s.deliveryOrder {
		intent, exists := s.deliveries[id]
		if !exists {
			continue
		}
		_, required := protected[id]
		turn := s.turns[intent.TurnID]
		if len(s.deliveries) > maxDeliveryRecords && terminalDelivery(intent.Status) &&
			!required && terminalTurn(turn.Status) {
			delete(s.deliveries, id)
			if intent.Kind == channeltypes.DeliveryCOTCreate {
				delete(s.cotFailures, intent.TurnID)
			}
			continue
		}
		kept = append(kept, id)
	}
	s.deliveryOrder = kept
}

func terminalTurn(status channeltypes.TurnStatus) bool {
	switch status {
	case channeltypes.TurnSucceeded, channeltypes.TurnFailed, channeltypes.TurnCanceled, "":
		return true
	default:
		return false
	}
}

func terminalDelivery(status channeltypes.DeliveryStatus) bool {
	return status == channeltypes.DeliveryDelivered || status == channeltypes.DeliveryFailed
}

func cloneIntent(intent channeltypes.DeliveryIntent) channeltypes.DeliveryIntent {
	intent.Events = append([]channeltypes.COTEvent(nil), intent.Events...)
	if intent.Card != nil {
		card := make(map[string]any, len(intent.Card))
		for key, value := range intent.Card {
			card[key] = value
		}
		intent.Card = card
	}
	return intent
}

func (s *Store) Intent(id string) (channeltypes.DeliveryIntent, bool) { return s.Delivery(id) }
func (s *Store) Begin(id string) error                                { return s.BeginDelivery(id) }

// COTFailed prevents sending a suffix after an earlier append had an ambiguous outcome.
func (s *Store) COTFailed(turnID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cotFailures[turnID]
}

func (s *Store) Pin(id string, pin bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pin {
		s.pinned[id] = true
	} else {
		delete(s.pinned, id)
	}
}
func (s *Store) turnRequiredLocked(id string) bool {
	for key, item := range s.deliveries {
		if item.TurnID == id && (s.pinned[key] || !terminalDelivery(item.Status)) {
			return true
		}
	}
	return false
}

// LatestCard returns the last queued snapshot, including updates awaiting delivery.
func (s *Store) LatestCard(createID string) (channeltypes.DeliveryIntent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.deliveryOrder) - 1; i >= 0; i-- {
		item := s.deliveries[s.deliveryOrder[i]]
		if (item.Kind == channeltypes.DeliveryCardUpdate && item.RelatedID == createID) || item.ID == createID {
			return cloneIntent(item), true
		}
	}
	return channeltypes.DeliveryIntent{}, false
}

// RetryCOTCompletion reopens a failed completion using only locally recorded
// identifiers. Repeated clicks leave pending and completed requests unchanged.
func (s *Store) RetryCOTCompletion(createID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	create, ok := s.deliveries[createID]
	if !ok || create.Kind != channeltypes.DeliveryCOTCreate || create.Status != channeltypes.DeliveryDelivered || create.COTID == "" {
		return fmt.Errorf("COT create record is unavailable")
	}
	id := create.TurnID + ":cot:complete"
	intent, ok := s.deliveries[id]
	if !ok {
		return fmt.Errorf("COT completion is not ready")
	}
	if intent.Status != channeltypes.DeliveryFailed {
		return nil
	}
	intent.Status = channeltypes.DeliveryPending
	intent.Events = nil
	intent.Attempts = 0
	intent.NextAttemptAt = nil
	intent.LastError = ""
	s.deliveries[id] = intent
	s.pinned[createID] = true
	return nil
}
