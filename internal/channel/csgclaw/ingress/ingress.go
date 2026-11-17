package ingress

import (
	"context"
	"csgclaw/internal/diagnostics"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/conv"
	"csgclaw/internal/channel/csgclaw/execution"
	"csgclaw/internal/channel/csgclaw/state"
)

const (
	defaultQueueSize        = 32
	defaultSeenWindow       = 256
	defaultConcurrentScopes = 8
)

// Worker is a Binding-scoped source buffer. It dedupes source events, preserves
// source order within one Conversation, and lets independent Conversations run
// concurrently. Agent Engine remains the final owner of Turn admission.
type Worker struct {
	adapter *execution.Adapter
	binding channel.Binding
	queue   chan queuedEvent
	cancel  context.CancelFunc
	done    chan struct{}

	mu            sync.Mutex
	stopped       bool
	queued        map[string]struct{}
	notifications map[agentengine.ConversationKey]string
	latest        map[string]string
	seen          *state.SeenWindow
	processing    map[string]struct{}
	active        map[agentengine.ConversationKey]map[string]activeEvent
	generation    map[agentengine.ConversationKey]uint64
}

type activeEvent struct {
	cancel context.CancelFunc
	unread chan struct{}
	event  channel.Event
}

type queuedEvent struct {
	diagnostic      *diagnostics.Record
	event           channel.Event
	dedupeKey       string
	conversationKey agentengine.ConversationKey
	generation      uint64
}

func NewWorker(adapter *execution.Adapter, binding channel.Binding) (*Worker, error) {
	if adapter == nil {
		return nil, fmt.Errorf("built-in IM adapter is required")
	}
	if binding.StableID() == "" || strings.TrimSpace(binding.AgentID) == "" {
		return nil, fmt.Errorf("binding id and agent id are required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &Worker{
		adapter:       adapter,
		binding:       binding,
		queue:         make(chan queuedEvent, defaultQueueSize),
		queued:        make(map[string]struct{}),
		notifications: make(map[agentengine.ConversationKey]string),
		latest:        make(map[string]string),
		seen:          state.NewSeenWindow(defaultSeenWindow),
		processing:    make(map[string]struct{}),
		active:        make(map[agentengine.ConversationKey]map[string]activeEvent),
		generation:    make(map[agentengine.ConversationKey]uint64),
		cancel:        cancel,
		done:          make(chan struct{}),
	}
	go w.run(ctx)
	return w, nil
}

func (w *Worker) SameBinding(binding channel.Binding) bool {
	if w == nil {
		return false
	}
	return w.binding.AgentID == strings.TrimSpace(binding.AgentID) &&
		w.binding.ParticipantID == strings.TrimSpace(binding.ParticipantID)
}

func (w *Worker) Submit(event channel.Event) error {
	if w == nil {
		return fmt.Errorf("binding worker is not running")
	}
	item, accepted, err := w.acceptAndEnqueue(event)
	if err != nil {
		return err
	}
	if !accepted {
		return nil
	}
	// A reset must interrupt the current context before it can reach the
	// conversation-aware ingress scheduler. Waiting for handle to return would
	// otherwise leave /new stuck behind the turn that it is supposed to stop.
	if execution.Classify(event) == "reset" {
		w.cancelConversation(item.conversationKey)
	}
	return nil
}

func (w *Worker) Close(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.mu.Lock()
	if !w.stopped {
		w.stopped = true
		w.cancel()
		for _, turns := range w.active {
			for _, turn := range turns {
				turn.cancel()
			}
		}
		w.active = make(map[agentengine.ConversationKey]map[string]activeEvent)
	}
	w.mu.Unlock()
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsCurrent reports whether sourceMessageID is still the newest accepted
// source for one Conversation. Detached interactions use this to avoid
// resuming an older workflow after a newer message has already been queued.
func (w *Worker) IsCurrent(key agentengine.ConversationKey, sourceMessageID string) bool {
	if w == nil {
		return false
	}
	scope := strings.TrimSpace(string(key))
	sourceMessageID = strings.TrimSpace(sourceMessageID)
	if scope == "" || sourceMessageID == "" {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.TrimSpace(w.latest[scope]) == sourceMessageID
}

func (w *Worker) run(ctx context.Context) {
	defer close(w.done)
	pending := make([]queuedEvent, 0, defaultQueueSize)
	activeScopes := make(map[agentengine.ConversationKey]queuedEvent)
	completed := make(chan agentengine.ConversationKey, defaultConcurrentScopes)
	var handlers sync.WaitGroup
	defer handlers.Wait()

	dispatch := func() {
		for index := 0; index < len(pending); {
			item := pending[index]
			if w.isCurrentGeneration(item) {
				index++
				continue
			}
			pending = append(pending[:index], pending[index+1:]...)
			item.diagnostic.Finish("canceled")
			w.discardQueued(item.dedupeKey)
		}
		for len(activeScopes) < defaultConcurrentScopes {
			index := nextDispatchable(pending, activeScopes)
			if index < 0 {
				return
			}
			item := pending[index]
			pending = append(pending[:index], pending[index+1:]...)
			activeScopes[item.conversationKey] = item
			w.beginProcessing(item.dedupeKey)
			handlers.Add(1)
			go func() {
				defer handlers.Done()
				w.handle(ctx, item)
				w.finishProcessing(item.dedupeKey)
				completed <- item.conversationKey
			}()
		}
	}

	for {
		dispatch()
		select {
		case <-ctx.Done():
			for _, item := range pending {
				item.diagnostic.Finish("interrupted")
			}
			for {
				select {
				case item := <-w.queue:
					item.diagnostic.Finish("interrupted")
				default:
					return
				}
			}
		case item := <-w.queue:
			item.diagnostic.Capture("performance.queue", map[string]any{"pending_scopes": len(pending), "active_scopes": len(activeScopes), "binding_queue_depth": len(w.queue)})
			pending = append(pending, item)
		case scope := <-completed:
			delete(activeScopes, scope)
		}
	}
}

func (w *Worker) handle(ctx context.Context, item queuedEvent) {
	if ctx.Err() != nil {
		item.diagnostic.Finish("interrupted")
		return
	}
	turnCtx, cancel := context.WithCancel(diagnostics.WithRecord(ctx, item.diagnostic))
	turn := activeEvent{cancel: cancel}
	if item.event.NotificationOnly {
		turn.unread = make(chan struct{}, 1)
	}
	if !w.setActive(item, turn) {
		item.diagnostic.Finish("canceled")
		cancel()
		return
	}
	defer func() {
		cancel()
		w.clearActive(item.conversationKey, item.dedupeKey)
	}()
	start := time.Now()
	attrs := []any{"phase", "agent_turn", "binding_id", w.binding.StableID(), "participant_id", w.binding.ParticipantID, "agent_id", w.binding.AgentID, "room_id", item.event.RoomID, "task_id", item.event.TaskID, "attempt", item.event.TaskAttempt, "message_id", item.event.MessageID, "conversation_key", item.conversationKey}
	if item.event.RoomManager {
		attrs = append(attrs, "room_role", "manager")
	} else {
		attrs = append(attrs, "room_role", "worker")
	}
	if item.event.TaskID != "" {
		slog.InfoContext(turnCtx, "room task Agent turn starting", attrs...)
	}
	turnCtx = w.adapter.WithUnreadSignal(turnCtx, w.binding, item.event, turn.unread)
	outcome, err := w.adapter.Handle(turnCtx, w.binding, item.event)
	attrs = append(attrs, "turn_id", outcome.Turn.TurnID, "duration", time.Since(start), "turn_status", outcome.Result.Status)
	if item.event.TaskID != "" {
		if err != nil {
			slog.ErrorContext(turnCtx, "room task Agent turn failed", append(attrs, "error", diagnostics.Redact(err.Error()))...)
		} else if outcome.Result.Error != nil {
			slog.ErrorContext(turnCtx, "room task Agent runtime failed", append(attrs, "error_code", outcome.Result.Error.Code, "error", diagnostics.Redact(outcome.Result.Error.Message))...)
		} else {
			slog.InfoContext(turnCtx, "room task Agent turn finished", attrs...)
		}
	}
	if err != nil && item.event.TaskID == "" {
		slog.ErrorContext(turnCtx, "handle built-in IM event failed",
			"binding_id", w.binding.StableID(),
			"participant_id", w.binding.ParticipantID,
			"message_id", item.event.MessageID,
			"conversation_key", item.conversationKey,
			"turn_id", outcome.Turn.TurnID,
			"error", diagnostics.Redact(err.Error()),
		)
	}
}

func (w *Worker) acceptAndEnqueue(event channel.Event) (queuedEvent, bool, error) {
	key := eventDedupKey(event)
	if key == "" {
		return queuedEvent{}, false, nil
	}
	conversationKey, err := conv.ConversationKey(w.binding, event)
	if err != nil {
		return queuedEvent{}, false, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return queuedEvent{}, false, fmt.Errorf("binding worker is stopped")
	}
	if w.seen.Has(key) {
		return queuedEvent{}, false, nil
	}
	if _, ok := w.queued[key]; ok {
		return queuedEvent{}, false, nil
	}
	if _, ok := w.processing[key]; ok {
		return queuedEvent{}, false, nil
	}
	if event.NotificationOnly && w.notifications[conversationKey] != "" {
		w.seen.Add(key)
		w.latest[string(conversationKey)] = w.notifications[conversationKey]
		w.signalUnreadLocked(conversationKey)
		w.signalRelatedUnreadLocked(event)
		return queuedEvent{}, false, nil
	}
	if len(w.queued) >= defaultQueueSize {
		return queuedEvent{}, false, fmt.Errorf("binding worker queue is full")
	}
	generation := w.generation[conversationKey]
	if execution.Classify(event) == "reset" {
		generation++
	}
	item := queuedEvent{
		diagnostic:      w.adapter.BeginDiagnostics(w.binding, event),
		event:           event,
		dedupeKey:       key,
		conversationKey: conversationKey,
		generation:      generation,
	}
	select {
	case w.queue <- item:
		w.generation[conversationKey] = generation
		if execution.Classify(event) == "reset" {
			delete(w.notifications, conversationKey)
		}
		w.queued[key] = struct{}{}
		if event.NotificationOnly {
			w.notifications[conversationKey] = key
			w.signalUnreadLocked(conversationKey)
			w.signalRelatedUnreadLocked(event)
		}
		w.latest[string(conversationKey)] = key
		return item, true, nil
	default:
		item.diagnostic.Failure("queue_full", "queue", "The channel queue is full")
		item.diagnostic.Finish("failed")
		return queuedEvent{}, false, fmt.Errorf("binding worker queue is full")
	}
}

func (w *Worker) isCurrentGeneration(item queuedEvent) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.generation[item.conversationKey] == item.generation
}

func (w *Worker) discardQueued(key string) {
	w.mu.Lock()
	delete(w.queued, key)
	for scope, source := range w.notifications {
		if source == key {
			delete(w.notifications, scope)
		}
	}
	w.seen.Add(key)
	w.mu.Unlock()
}

func (w *Worker) beginProcessing(key string) {
	w.mu.Lock()
	delete(w.queued, key)
	for scope, source := range w.notifications {
		if source == key {
			delete(w.notifications, scope)
		}
	}
	w.processing[key] = struct{}{}
	w.mu.Unlock()
}

func (w *Worker) finishProcessing(key string) {
	w.mu.Lock()
	delete(w.processing, key)
	w.seen.Add(key)
	w.mu.Unlock()
}

func (w *Worker) setActive(item queuedEvent, turn activeEvent) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.generation[item.conversationKey] != item.generation {
		return false
	}
	if w.active[item.conversationKey] == nil {
		w.active[item.conversationKey] = make(map[string]activeEvent)
	}
	turn.event = item.event
	w.active[item.conversationKey][item.dedupeKey] = turn
	if w.notifications[item.conversationKey] != "" {
		w.signalUnreadLocked(item.conversationKey)
	}
	if item.event.NotificationOnly && item.event.ThreadRootID == "" {
		topic := item.event
		topic.ThreadRootID = item.event.MessageID
		if key, err := conv.ConversationKey(w.binding, topic); err == nil && w.notifications[key] != "" {
			signalUnread(turn)
		}
	}
	return true
}

func (w *Worker) signalUnreadLocked(key agentengine.ConversationKey) {
	for _, turn := range w.active[key] {
		signalUnread(turn)
	}
}

func (w *Worker) signalRelatedUnreadLocked(topic channel.Event) {
	if topic.ThreadRootID == "" {
		return
	}
	main := topic
	main.ThreadRootID = ""
	key, err := conv.ConversationKey(w.binding, main)
	if err != nil {
		return
	}
	for _, turn := range w.active[key] {
		if relatedUnreadTopic(turn.event, topic) {
			signalUnread(turn)
		}
	}
}

func signalUnread(turn activeEvent) {
	select {
	case turn.unread <- struct{}{}:
	default:
	}
}

func (w *Worker) clearActive(key agentengine.ConversationKey, sourceMessageID string) {
	w.mu.Lock()
	if turns := w.active[key]; turns != nil {
		delete(turns, sourceMessageID)
		if len(turns) == 0 {
			delete(w.active, key)
		}
	}
	w.mu.Unlock()
}

func (w *Worker) cancelConversation(key agentengine.ConversationKey) {
	if key == "" {
		return
	}
	w.mu.Lock()
	turns := w.active[key]
	delete(w.active, key)
	w.mu.Unlock()
	for _, turn := range turns {
		turn.cancel()
	}
}

func nextDispatchable(pending []queuedEvent, active map[agentengine.ConversationKey]queuedEvent) int {
next:
	for index, item := range pending {
		if _, busy := active[item.conversationKey]; busy {
			continue
		}
		for _, running := range active {
			if relatedUnreadTopic(running.event, item.event) || relatedUnreadTopic(item.event, running.event) {
				continue next
			}
		}
		return index
	}
	return -1
}

func relatedUnreadTopic(main, topic channel.Event) bool {
	return main.NotificationOnly && topic.NotificationOnly && main.RoomID == topic.RoomID &&
		main.ThreadRootID == "" && topic.ThreadRootID != "" && topic.ThreadRootID == main.MessageID
}

func eventDedupKey(event channel.Event) string {
	return strings.TrimSpace(event.MessageID)
}
