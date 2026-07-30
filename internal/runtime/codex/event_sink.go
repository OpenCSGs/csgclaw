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
	targets := make([]*sessionSubscription, 0, len(s.subscribers))
	for _, sub := range s.subscribers {
		if sub.runtimeID != "" && sub.runtimeID != runtimeID {
			continue
		}
		if sub.sessionID != "" && sub.sessionID != strings.TrimSpace(event.SessionID) {
			continue
		}
		targets = append(targets, sub)
	}
	s.mu.Unlock()

	for _, sub := range targets {
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
	if !activity.RuntimeEventRequiresReliableDelivery(event) {
		_ = trySendSessionEvent(s.ch, event)
		return
	}
	s.sendReliable(event)
}

func (s *sessionSubscription) sendReliable(event SessionEvent) {
	s.reliableMu.Lock()
	s.reliableQueue = append(s.reliableQueue, event)
	s.reliableMu.Unlock()
	select {
	case <-s.done:
	case s.reliableWake <- struct{}{}:
	default:
	}
}

func (s *sessionSubscription) pumpReliable() {
	defer close(s.stopped)
	for {
		if event, ok := s.popReliable(); ok {
			if !sendSessionEventUntilDone(s.ch, s.done, event) {
				return
			}
			continue
		}
		select {
		case <-s.done:
			return
		case <-s.reliableWake:
		}
	}
}

func (s *sessionSubscription) popReliable() (SessionEvent, bool) {
	s.reliableMu.Lock()
	defer s.reliableMu.Unlock()
	if len(s.reliableQueue) == 0 {
		return SessionEvent{}, false
	}
	event := s.reliableQueue[0]
	s.reliableQueue[0] = SessionEvent{}
	s.reliableQueue = s.reliableQueue[1:]
	if len(s.reliableQueue) == 0 {
		s.reliableQueue = nil
	}
	return event, true
}

func trySendSessionEvent(ch chan SessionEvent, event SessionEvent) (sent bool) {
	defer func() {
		if recover() != nil {
			sent = false
		}
	}()
	select {
	case ch <- event:
		return true
	default:
		return false
	}
}

func sendSessionEventUntilDone(ch chan SessionEvent, done <-chan struct{}, event SessionEvent) (sent bool) {
	defer func() {
		if recover() != nil {
			sent = false
		}
	}()
	select {
	case ch <- event:
		return true
	case <-done:
		return false
	}
}
