package codex

import (
	"strings"
	"sync"

	"csgclaw/internal/activity"
)

const defaultSessionEventBuffer = 64

// EventSink fans out normalized Codex session events to bridge workers.
type EventSink struct {
	mu          sync.Mutex
	nextID      int
	subscribers map[int]*sessionSubscription
}

type sessionSubscription struct {
	runtimeID string
	sessionID string
	ch        chan SessionEvent
	done      chan struct{}
	stopped   chan struct{}

	reliableMu    sync.Mutex
	reliableQueue []SessionEvent
	reliableWake  chan struct{}
}

func NewEventSink() *EventSink {
	return &EventSink{
		subscribers: make(map[int]*sessionSubscription),
	}
}

func (s *EventSink) Publish(event SessionEvent) {
	if s == nil {
		return
	}

	runtimeID := strings.TrimSpace(event.RuntimeID)

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sub := range s.subscribers {
		if sub.runtimeID != "" && sub.runtimeID != runtimeID {
			continue
		}
		if sub.sessionID != "" && sub.sessionID != strings.TrimSpace(event.SessionID) {
			continue
		}
		sub.deliver(event)
	}
}

func (s *EventSink) Subscribe(runtimeID string) (<-chan SessionEvent, func()) {
	return s.subscribe(runtimeID, "")
}

func (s *EventSink) SubscribeSession(runtimeID, sessionID string) (<-chan SessionEvent, func()) {
	return s.subscribe(runtimeID, sessionID)
}

func (s *EventSink) subscribe(runtimeID, sessionID string) (<-chan SessionEvent, func()) {
	ch := make(chan SessionEvent, defaultSessionEventBuffer)
	if s == nil {
		close(ch)
		return ch, func() {}
	}

	s.mu.Lock()
	id := s.nextID
	s.nextID++
	sub := &sessionSubscription{
		runtimeID:    strings.TrimSpace(runtimeID),
		sessionID:    strings.TrimSpace(sessionID),
		ch:           ch,
		done:         make(chan struct{}),
		stopped:      make(chan struct{}),
		reliableWake: make(chan struct{}, 1),
	}
	s.subscribers[id] = sub
	s.mu.Unlock()
	go sub.pumpReliable()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			s.mu.Lock()
			if sub, ok := s.subscribers[id]; ok {
				delete(s.subscribers, id)
				close(sub.done)
				<-sub.stopped
				close(sub.ch)
			}
			s.mu.Unlock()
		})
	}
	return ch, cancel
}

func (s *sessionSubscription) deliver(event SessionEvent) {
	s.reliableMu.Lock()
	defer s.reliableMu.Unlock()
	// Send directly only when no older event is waiting for this subscriber.
	if len(s.reliableQueue) == 0 {
		select {
		case s.ch <- event:
			return
		default:
		}
	}
	if !activity.RuntimeEventRequiresReliableDelivery(event) {
		return
	}
	s.reliableQueue = append(s.reliableQueue, event)
	select {
	case s.reliableWake <- struct{}{}:
	default:
	}
}

func (s *sessionSubscription) pumpReliable() {
	defer close(s.stopped)
	for {
		s.reliableMu.Lock()
		if len(s.reliableQueue) == 0 {
			s.reliableMu.Unlock()
			select {
			case <-s.done:
				return
			case <-s.reliableWake:
			}
			continue
		}
		event := s.reliableQueue[0]
		s.reliableMu.Unlock()

		// Keep the head in the queue until sent so a new event cannot pass it.
		select {
		case s.ch <- event:
		case <-s.done:
			return
		}
		s.reliableMu.Lock()
		s.reliableQueue[0] = SessionEvent{}
		s.reliableQueue = s.reliableQueue[1:]
		if len(s.reliableQueue) == 0 {
			s.reliableQueue = nil
		}
		s.reliableMu.Unlock()
	}
}
