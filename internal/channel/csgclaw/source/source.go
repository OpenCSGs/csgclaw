// Package source connects built-in IM participant events to Binding-scoped
// ingress workers without using the legacy HTTP/SSE Codex bridge.
package source

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/binding"
	"csgclaw/internal/channelbridge"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	agentruntime "csgclaw/internal/runtime"
)

const (
	defaultReconcileInterval = 30 * time.Second
	defaultRetryInitial      = 50 * time.Millisecond
	defaultRetryMax          = time.Second
)

type participantProvider interface {
	List(participant.ListOptions) []apitypes.Participant
}

type agentProvider interface {
	Get(context.Context, string) (agentengine.Agent, error)
}

type workerManager interface {
	Ensure(channel.Binding) error
	Stop(channel.BindingID)
	Submit(channel.Binding, channelbridge.BotEvent) error
	Close()
}

// Source owns the in-process event subscriptions for built-in IM bindings.
// The subscription lifetime follows the Binding, not the Agent runtime.
type Source struct {
	bridge         *im.ParticipantBridge
	bus            *im.Bus
	participants   participantProvider
	agents         agentProvider
	workers        workerManager
	reconcileEvery time.Duration
	retryInitial   time.Duration
	retryMax       time.Duration

	reconcileMu   sync.Mutex
	mu            sync.Mutex
	cancel        context.CancelFunc
	cancelBus     func()
	subscriptions map[channel.BindingID]*subscription
	wg            sync.WaitGroup
}

type subscription struct {
	binding channel.Binding
	cancel  func()
}

func New(
	bridge *im.ParticipantBridge,
	bus *im.Bus,
	participants participantProvider,
	agents agentProvider,
	workers *binding.Manager,
) (*Source, error) {
	return newSource(bridge, bus, participants, agents, workers)
}

func newSource(
	bridge *im.ParticipantBridge,
	bus *im.Bus,
	participants participantProvider,
	agents agentProvider,
	workers workerManager,
) (*Source, error) {
	if bridge == nil {
		return nil, fmt.Errorf("participant event bridge is required")
	}
	if participants == nil {
		return nil, fmt.Errorf("participant provider is required")
	}
	if agents == nil {
		return nil, fmt.Errorf("agent provider is required")
	}
	if workers == nil {
		return nil, fmt.Errorf("binding worker manager is required")
	}
	return &Source{
		bridge:         bridge,
		bus:            bus,
		participants:   participants,
		agents:         agents,
		workers:        workers,
		reconcileEvery: defaultReconcileInterval,
		retryInitial:   defaultRetryInitial,
		retryMax:       defaultRetryMax,
		subscriptions:  make(map[channel.BindingID]*subscription),
	}, nil
}

// Start ensures workers and event subscriptions for persisted Codex bindings,
// then follows Participant events and periodically reconciles Agent eligibility.
func (s *Source) Start(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	var lifecycleEvents <-chan im.Event
	if s.bus != nil {
		lifecycleEvents, s.cancelBus = s.bus.Subscribe()
	}
	s.cancel = cancel
	s.mu.Unlock()

	if err := s.Reconcile(runCtx); err != nil {
		s.Close()
		return err
	}
	s.wg.Add(1)
	go s.followDesiredState(runCtx, lifecycleEvents)
	return nil
}

func (s *Source) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	cancelBus := s.cancelBus
	s.cancelBus = nil
	subscriptions := make([]*subscription, 0, len(s.subscriptions))
	for id, sub := range s.subscriptions {
		subscriptions = append(subscriptions, sub)
		delete(s.subscriptions, id)
	}
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if cancelBus != nil {
		cancelBus()
	}
	for _, sub := range subscriptions {
		if sub != nil && sub.cancel != nil {
			sub.cancel()
		}
	}
	s.wg.Wait()
	if s.workers != nil {
		s.workers.Close()
	}
}

func (s *Source) followDesiredState(ctx context.Context, events <-chan im.Event) {
	defer s.wg.Done()
	interval := s.reconcileEvery
	if interval <= 0 {
		interval = defaultReconcileInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Reconcile(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("reconcile built-in IM bindings failed", "error", err)
			}
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if event.Participant == nil || !strings.EqualFold(strings.TrimSpace(event.Participant.Channel), participant.ChannelCSGClaw) {
				continue
			}
			switch event.Type {
			case im.EventTypeParticipantCreated, im.EventTypeParticipantUpdated, im.EventTypeParticipantDeleted:
				if err := s.Reconcile(ctx); err != nil && ctx.Err() == nil {
					slog.Warn("refresh built-in IM bindings failed", "participant_id", event.Participant.ID, "error", err)
				}
			}
		}
	}
}

// Reconcile makes subscriptions and Binding workers match the authoritative
// Participant + Agent snapshot. This repairs missed bus events and Agent
// runtime/profile changes that do not emit Participant events.
func (s *Source) Reconcile(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	desired := make(map[channel.BindingID]channel.Binding)
	for _, item := range s.participants.List(participant.ListOptions{
		Channel: participant.ChannelCSGClaw,
		Type:    participant.TypeAgent,
	}) {
		value, ok := s.binding(ctx, item)
		if !ok {
			continue
		}
		desired[value.StableID()] = value
	}

	var outErr error
	for _, value := range desired {
		if err := s.ensureBinding(ctx, value); err != nil {
			outErr = errors.Join(outErr, err)
		}
	}

	s.mu.Lock()
	stale := make([]channel.BindingID, 0)
	for id := range s.subscriptions {
		if _, ok := desired[id]; !ok {
			stale = append(stale, id)
		}
	}
	s.mu.Unlock()
	for _, id := range stale {
		s.stop(id)
	}
	return outErr
}

func (s *Source) ensureBinding(ctx context.Context, value channel.Binding) error {
	if err := s.workers.Ensure(value); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		s.workers.Stop(value.StableID())
		return err
	}

	bindingID := value.StableID()
	s.mu.Lock()
	if s.cancel == nil {
		s.mu.Unlock()
		s.workers.Stop(bindingID)
		return context.Canceled
	}
	if existing := s.subscriptions[bindingID]; existing != nil {
		existing.binding = value
		s.mu.Unlock()
		return nil
	}
	events, cancel := s.bridge.Subscribe(value.ParticipantID)
	s.subscriptions[bindingID] = &subscription{binding: value, cancel: cancel}
	s.wg.Add(1)
	s.mu.Unlock()
	go s.forward(ctx, bindingID, value.ParticipantID, events)
	return nil
}

func (s *Source) stop(bindingID channel.BindingID) {
	bindingID = channel.BindingID(strings.TrimSpace(string(bindingID)))
	if bindingID == "" {
		return
	}
	s.mu.Lock()
	sub := s.subscriptions[bindingID]
	delete(s.subscriptions, bindingID)
	s.mu.Unlock()
	if sub != nil && sub.cancel != nil {
		sub.cancel()
	}
	s.workers.Stop(bindingID)
}

func (s *Source) forward(ctx context.Context, bindingID channel.BindingID, participantID string, events <-chan im.ParticipantEvent) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if s.submitWithRetry(ctx, bindingID, event) {
				s.bridge.Ack(participantID, event.MessageID)
				continue
			}
			// The source or this subscription stopped while the event was still
			// inflight. Preserve it for the next subscription instead of losing it.
			s.bridge.Requeue(participantID, event)
			return
		}
	}
}

func (s *Source) submitWithRetry(ctx context.Context, bindingID channel.BindingID, event im.ParticipantEvent) bool {
	delay := s.retryInitial
	if delay <= 0 {
		delay = defaultRetryInitial
	}
	maxDelay := s.retryMax
	if maxDelay < delay {
		maxDelay = defaultRetryMax
	}
	for {
		if !s.hasSubscription(bindingID) {
			return false
		}
		value, ok := s.currentBinding(ctx, bindingID)
		if ok {
			if err := s.workers.Submit(value, botEvent(value, event)); err == nil {
				return true
			} else {
				slog.Warn("submit built-in IM event failed; retrying",
					"binding_id", bindingID,
					"message_id", event.MessageID,
					"retry_in", delay,
					"error", err,
				)
			}
		} else {
			slog.Warn("built-in IM binding temporarily unavailable; retrying",
				"binding_id", bindingID,
				"message_id", event.MessageID,
				"retry_in", delay,
			)
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
		if delay < maxDelay {
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
}

func (s *Source) hasSubscription(bindingID channel.BindingID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.subscriptions[bindingID] != nil
}

func (s *Source) currentBinding(ctx context.Context, bindingID channel.BindingID) (channel.Binding, bool) {
	s.mu.Lock()
	sub := s.subscriptions[bindingID]
	var value channel.Binding
	if sub != nil {
		value = sub.binding
	}
	s.mu.Unlock()
	if sub == nil {
		return channel.Binding{}, false
	}
	selected, err := s.agents.Get(ctx, value.AgentID)
	if err != nil || !usesHostCodex(selected) {
		return channel.Binding{}, false
	}
	return value, true
}

func (s *Source) binding(ctx context.Context, item apitypes.Participant) (channel.Binding, bool) {
	if !strings.EqualFold(strings.TrimSpace(item.Channel), participant.ChannelCSGClaw) ||
		!strings.EqualFold(strings.TrimSpace(item.Type), participant.TypeAgent) {
		return channel.Binding{}, false
	}
	participantID := strings.TrimSpace(item.ID)
	agentID := strings.TrimSpace(item.AgentID)
	if participantID == "" || agentID == "" {
		return channel.Binding{}, false
	}
	selected, err := s.agents.Get(ctx, agentID)
	if err != nil || !usesHostCodex(selected) {
		return channel.Binding{}, false
	}
	return channel.Binding{
		ID:            participantID,
		Channel:       channel.ChannelCSGClaw,
		ParticipantID: participantID,
		AgentID:       agentID,
		Enabled:       true,
	}, true
}

func usesHostCodex(selected agentengine.Agent) bool {
	return strings.EqualFold(strings.TrimSpace(selected.Spec.Runtime.Adapter), agentruntime.NameCodex) &&
		!selected.Spec.Runtime.Sandboxed
}

func botEvent(value channel.Binding, event im.ParticipantEvent) channelbridge.BotEvent {
	return channelbridge.BotEvent{
		Channel:       string(channel.ChannelCSGClaw),
		ParticipantID: value.ParticipantID,
		MessageID:     strings.TrimSpace(event.MessageID),
		RoomID:        strings.TrimSpace(event.RoomID),
		Locale:        strings.TrimSpace(event.Locale),
		ChatType:      strings.TrimSpace(event.ChatType),
		Text:          event.Text,
		Attachments:   append([]channelbridge.MessageAttachment(nil), event.Attachments...),
		Mentions:      append([]string(nil), event.Mentions...),
		Mentioned:     event.Mentioned,
		ThreadRootID:  strings.TrimSpace(event.ThreadRootID),
		ThreadContext: botThreadContext(event.ThreadContext),
	}
}

func botThreadContext(value *im.ParticipantThreadContext) *channelbridge.BotThreadContext {
	if value == nil {
		return nil
	}
	out := &channelbridge.BotThreadContext{
		RootMessageID: strings.TrimSpace(value.RootMessageID),
		Summary: channelbridge.BotThreadContextSummary{
			RootExcerpt:  value.Summary.RootExcerpt,
			MessageCount: value.Summary.MessageCount,
			BeforeCount:  value.Summary.BeforeCount,
			AfterCount:   value.Summary.AfterCount,
		},
	}
	for _, message := range value.Context {
		createdAt := ""
		if !message.CreatedAt.IsZero() {
			createdAt = message.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00")
		}
		out.Context = append(out.Context, channelbridge.BotThreadContextMessage{
			ID:          message.ID,
			SenderID:    message.SenderID,
			Content:     message.Content,
			CreatedAt:   createdAt,
			Attachments: append([]channelbridge.MessageAttachment(nil), message.Attachments...),
		})
	}
	return out
}
