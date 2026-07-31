package worklease

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	agentruntime "csgclaw/internal/runtime"
)

const ControlEventTypeParticipantWorkStopRequested = "participant.work.stop_requested"

type ControlEvent struct {
	RegistryEpoch string    `json:"registry_epoch"`
	ParticipantID string    `json:"participant_id"`
	RoomID        string    `json:"room_id"`
	LeaseID       string    `json:"lease_id"`
	RequestID     string    `json:"request_id"`
	RequestedAt   time.Time `json:"requested_at"`
}

// ControlBus is a bounded, best-effort path for participant-directed runtime
// controls. The Registry remains the source of truth when a live event is lost.
type ControlBus struct {
	mu          sync.Mutex
	nextID      int
	subscribers map[string]map[int]chan ControlEvent
}

type registeredTurnController struct {
	id         uint64
	controller agentruntime.TurnController
}

// TurnControlDispatcher routes a participant turn to its runtime-specific
// controller. A fallback controller is used for runtimes, such as OpenClaw,
// whose control plane is the participant SSE connection.
type TurnControlDispatcher struct {
	mu          sync.RWMutex
	nextID      uint64
	controllers map[string]map[uint64]agentruntime.TurnController
	fallback    agentruntime.TurnController
}

func NewTurnControlDispatcher(fallback agentruntime.TurnController) *TurnControlDispatcher {
	return &TurnControlDispatcher{
		controllers: make(map[string]map[uint64]agentruntime.TurnController),
		fallback:    fallback,
	}
}

func (d *TurnControlDispatcher) RegisterTurnController(
	participantID string,
	controller agentruntime.TurnController,
) func() {
	participantID = strings.TrimSpace(participantID)
	if d == nil || participantID == "" || controller == nil {
		return func() {}
	}
	d.mu.Lock()
	d.nextID++
	registered := registeredTurnController{id: d.nextID, controller: controller}
	byID := d.controllers[participantID]
	if byID == nil {
		byID = make(map[uint64]agentruntime.TurnController)
		d.controllers[participantID] = byID
	}
	byID[registered.id] = registered.controller
	d.mu.Unlock()
	return func() {
		d.mu.Lock()
		if byID := d.controllers[participantID]; byID != nil {
			delete(byID, registered.id)
			if len(byID) == 0 {
				delete(d.controllers, participantID)
			}
		}
		d.mu.Unlock()
	}
}

func (d *TurnControlDispatcher) StopTurn(ctx context.Context, ref agentruntime.TurnRef) error {
	if d == nil {
		return agentruntime.ErrTurnControlUnavailable
	}
	participantID := strings.TrimSpace(ref.ParticipantID)
	d.mu.RLock()
	byID := d.controllers[participantID]
	registered := make([]registeredTurnController, 0, len(byID))
	for id, controller := range byID {
		registered = append(registered, registeredTurnController{id: id, controller: controller})
	}
	fallback := d.fallback
	d.mu.RUnlock()
	sort.Slice(registered, func(i, j int) bool { return registered[i].id > registered[j].id })
	for _, candidate := range registered {
		err := candidate.controller.StopTurn(ctx, ref)
		if errors.Is(err, agentruntime.ErrTurnNotFound) {
			continue
		}
		return err
	}
	if len(registered) > 0 {
		return agentruntime.ErrTurnNotFound
	}
	if fallback == nil {
		return agentruntime.ErrTurnControlUnavailable
	}
	return fallback.StopTurn(ctx, ref)
}

func NewControlBus() *ControlBus {
	return &ControlBus{subscribers: make(map[string]map[int]chan ControlEvent)}
}

func (b *ControlBus) Subscribe(participantID string) (<-chan ControlEvent, func()) {
	ch := make(chan ControlEvent, 16)
	participantID = strings.TrimSpace(participantID)
	if b == nil || participantID == "" {
		close(ch)
		return ch, func() {}
	}

	b.mu.Lock()
	id := b.nextID
	b.nextID++
	byID := b.subscribers[participantID]
	if byID == nil {
		byID = make(map[int]chan ControlEvent)
		b.subscribers[participantID] = byID
	}
	byID[id] = ch
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		if byID := b.subscribers[participantID]; byID != nil {
			delete(byID, id)
			if len(byID) == 0 {
				delete(b.subscribers, participantID)
			}
		}
		b.mu.Unlock()
	}
}

func (b *ControlBus) Publish(event ControlEvent) {
	if b == nil {
		return
	}
	b.mu.Lock()
	targets := make([]chan ControlEvent, 0, len(b.subscribers[event.ParticipantID]))
	for _, subscriber := range b.subscribers[event.ParticipantID] {
		targets = append(targets, subscriber)
	}
	b.mu.Unlock()

	for _, subscriber := range targets {
		select {
		case subscriber <- event:
		default:
		}
	}
}

func (b *ControlBus) StopTurn(ctx context.Context, ref agentruntime.TurnRef) error {
	if b == nil {
		return agentruntime.ErrTurnControlUnavailable
	}
	event := ControlEvent{
		RegistryEpoch: strings.TrimSpace(ref.RegistryEpoch),
		ParticipantID: strings.TrimSpace(ref.ParticipantID),
		RoomID:        strings.TrimSpace(ref.RoomID),
		LeaseID:       strings.TrimSpace(ref.LeaseID),
		RequestID:     strings.TrimSpace(ref.RequestID),
		RequestedAt:   ref.RequestedAt.UTC(),
	}
	if event.RequestedAt.IsZero() {
		event.RequestedAt = time.Now().UTC()
	}
	b.mu.Lock()
	targets := make([]chan ControlEvent, 0, len(b.subscribers[event.ParticipantID]))
	for _, subscriber := range b.subscribers[event.ParticipantID] {
		targets = append(targets, subscriber)
	}
	b.mu.Unlock()

	delivered := false
	for _, subscriber := range targets {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case subscriber <- event:
			delivered = true
		default:
		}
	}
	if !delivered {
		return agentruntime.ErrTurnControlUnavailable
	}
	return nil
}
