package ingress

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	channeltypes "csgclaw/internal/channel"
	"csgclaw/internal/channel/feishu/interaction"
	"csgclaw/internal/channel/feishu/presentation"
	feishustate "csgclaw/internal/channel/feishu/state"
	"csgclaw/internal/channel/feishu/transport"
)

const (
	defaultQueueSize       = 32
	defaultIntakeWorkers   = 8
	defaultStartGrace      = 30 * time.Second
	defaultMaxEventAge     = 2 * time.Minute
	defaultFutureClockSkew = 2 * time.Minute
	maxConversationClocks  = 1024
)

type IntakeOptions struct {
	Binding      channeltypes.Binding
	State        *feishustate.Store
	Runner       intakeRunner
	Interactions *interaction.Handler
	Notifier     notifier
	Comments     transport.CommentAdapter
	Messages     transport.MessageAdapter
	QueueSize    int
	MaxEventAge  time.Duration
	StartGrace   time.Duration
	Now          func() time.Time
}

type intakeItem struct {
	stop    *channeltypes.InboundMessage
	message *channeltypes.InboundMessage
	card    *normalizedCardAction
	comment *normalizedComment
}

type intakeRunner interface {
	Submit(context.Context, channeltypes.InboundMessage) error
	Reset(context.Context, channeltypes.InboundMessage) error
	Cancel(context.Context, string, string, string) error
	IsResetCommand(string) bool
	ActiveTurn(string) string
}

type notifier interface {
	Notify()
}

// Intake performs fast normalization and bounded in-memory handoff. It owns no
// conversation queue and no restart recovery; Agent Engine owns Turn admission.
type Intake struct {
	binding      channeltypes.Binding
	state        *feishustate.Store
	dedup        *Deduplicator
	runner       intakeRunner
	interactions *interaction.Handler
	notifier     notifier
	comments     transport.CommentAdapter
	messages     transport.MessageAdapter
	queue        chan intakeItem
	activate     chan struct{}
	activateOnce sync.Once

	identityMu sync.RWMutex
	identity   transport.Identity

	clockMu sync.Mutex
	latest  map[string]time.Time

	now         func() time.Time
	startedAt   time.Time
	startGrace  time.Duration
	maxEventAge time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func NewIntake(options IntakeOptions) (*Intake, error) {
	if strings.TrimSpace(options.Binding.ID) == "" || strings.TrimSpace(options.Binding.AgentID) == "" {
		return nil, fmt.Errorf("feishu intake: binding and agent IDs are required")
	}
	if options.State == nil || options.Runner == nil {
		return nil, fmt.Errorf("feishu intake: memory state and runner are required")
	}
	size := options.QueueSize
	if size <= 0 {
		size = defaultQueueSize
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	maxAge := options.MaxEventAge
	if maxAge <= 0 {
		maxAge = defaultMaxEventAge
	}
	grace := options.StartGrace
	if grace <= 0 {
		grace = defaultStartGrace
	}
	return &Intake{
		binding:      options.Binding,
		state:        options.State,
		dedup:        NewDeduplicator(defaultSeenWindow),
		runner:       options.Runner,
		interactions: options.Interactions,
		notifier:     options.Notifier,
		comments:     options.Comments,
		messages:     options.Messages,
		queue:        make(chan intakeItem, size),
		activate:     make(chan struct{}),
		latest:       make(map[string]time.Time),
		now:          now,
		startedAt:    now().UTC(),
		startGrace:   grace,
		maxEventAge:  maxAge,
		done:         make(chan struct{}),
	}, nil
}

func (i *Intake) Start(ctx context.Context) error {
	if i == nil {
		return fmt.Errorf("feishu intake is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	i.mu.Lock()
	if i.cancel != nil {
		i.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	i.cancel = cancel
	i.mu.Unlock()
	go i.run(runCtx)
	return nil
}

func (i *Intake) Activate() error {
	if i == nil {
		return fmt.Errorf("feishu intake is required")
	}
	i.mu.Lock()
	started := i.cancel != nil
	i.mu.Unlock()
	if !started {
		return fmt.Errorf("feishu intake is not started")
	}
	i.activateOnce.Do(func() { close(i.activate) })
	return nil
}

func (i *Intake) SetIdentity(identity transport.Identity) {
	i.identityMu.Lock()
	i.identity = identity
	i.identityMu.Unlock()
}

func (i *Intake) HandleEvent(_ context.Context, event transport.Event) error {
	if i == nil {
		return fmt.Errorf("feishu intake is required")
	}
	slog.Info("receive Feishu ingress event", eventLogAttrs(i.binding, event, "occurred_at", eventTime(event))...)
	switch event.Kind {
	case transport.EventMessage:
		i.identityMu.RLock()
		identity := i.identity
		i.identityMu.RUnlock()
		message, accepted, err := normalizeMessage(i.binding, event, identity)
		if err != nil {
			slog.Warn("normalize Feishu message event failed", eventLogAttrs(i.binding, event, "error", err)...)
			return err
		}
		if !accepted {
			slog.Info("ignore Feishu message event", eventLogAttrs(i.binding, event, "reason", "not accepted")...)
			return nil
		}
		if !i.acceptFresh(event, message.ConversationKey) {
			return nil
		}
		if !i.dedup.Claim(message.Source) {
			slog.Debug("drop duplicate Feishu message event", inboundMessageLogAttrs(message)...)
			return nil
		}
		if strings.EqualFold(strings.TrimSpace(message.Text), "/stop") && len(message.Files) == 0 {
			message.TurnID = i.runner.ActiveTurn(message.ConversationKey)
			slog.Info("admit Feishu stop command", inboundMessageLogAttrs(message)...)
			i.admit(intakeItem{stop: &message})
			return nil
		}
		slog.Info("admit Feishu message event", inboundMessageLogAttrs(message)...)
		i.admit(intakeItem{message: &message})
		return nil

	case transport.EventCardAction:
		card, err := normalizeCardAction(i.binding, event, i.runner, i.state)
		if err != nil {
			slog.Warn("normalize Feishu card action failed", eventLogAttrs(i.binding, event, "error", err)...)
			return err
		}
		slog.Info("resolve Feishu card control", cardLogAttrs(card)...)
		if !i.acceptFresh(event, firstNonEmpty(card.conversationKey, card.source.ChatID)) {
			return nil
		}
		if !i.dedup.Claim(card.source) {
			slog.Info("drop duplicate Feishu card action", cardLogAttrs(card)...)
			return nil
		}
		slog.Info("admit Feishu card action", cardLogAttrs(card)...)
		i.admit(intakeItem{card: &card})
		return nil

	case transport.EventComment:
		i.identityMu.RLock()
		identity := i.identity
		i.identityMu.RUnlock()
		comment, accepted, err := normalizeComment(i.binding, event, identity)
		if err != nil {
			slog.Warn("normalize Feishu comment event failed", eventLogAttrs(i.binding, event, "error", err)...)
			return err
		}
		if !accepted {
			slog.Debug("ignore Feishu comment event", eventLogAttrs(i.binding, event, "reason", "not accepted")...)
			return nil
		}
		if i.comments == nil {
			slog.Warn("drop Feishu comment event because comment adapter is unavailable", eventLogAttrs(i.binding, event)...)
			return ErrCommentUnsupported
		}
		if !i.acceptFresh(event, comment.ConversationKey) {
			return nil
		}
		if !i.dedup.Claim(comment.Source) {
			slog.Debug("drop duplicate Feishu comment event", commentLogAttrs(comment, "agent_id", i.binding.AgentID)...)
			return nil
		}
		slog.Debug("admit Feishu comment event", commentLogAttrs(comment, "agent_id", i.binding.AgentID)...)
		i.admit(intakeItem{comment: &comment})
		return nil

	default:
		err := fmt.Errorf("unsupported Feishu ingress event %q", event.Kind)
		slog.Warn("drop unsupported Feishu ingress event", eventLogAttrs(i.binding, event, "error", err)...)
		return err
	}
}

func (i *Intake) admit(item intakeItem) {
	select {
	case i.queue <- item:
	default:
		// There is intentionally no durable fallback. A saturated process drops
		// new ingress instead of replaying an old message after the user context
		// has moved on.
		slog.Warn("drop Feishu event because in-memory intake is full",
			intakeItemLogAttrs(i.binding, item, "queue_size", len(i.queue), "queue_capacity", cap(i.queue))...)
	}
}

func (i *Intake) acceptFresh(event transport.Event, scope string) bool {
	occurredAt := eventTime(event)
	if occurredAt.IsZero() {
		return true
	}
	occurredAt = occurredAt.UTC()
	now := i.now().UTC()
	if occurredAt.Before(i.startedAt.Add(-i.startGrace)) ||
		now.Sub(occurredAt) > i.maxEventAge ||
		occurredAt.After(now.Add(defaultFutureClockSkew)) {
		slog.Warn("drop stale Feishu event",
			eventLogAttrs(i.binding, event,
				"scope", scope,
				"occurred_at", occurredAt,
				"received_at", now,
				"started_at", i.startedAt,
				"max_event_age", i.maxEventAge,
			)...)
		return false
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return true
	}
	i.clockMu.Lock()
	defer i.clockMu.Unlock()
	if latest := i.latest[scope]; !latest.IsZero() && occurredAt.Before(latest) {
		slog.Warn("drop out-of-order Feishu event",
			eventLogAttrs(i.binding, event,
				"scope", scope,
				"occurred_at", occurredAt,
				"latest_at", latest,
			)...)
		return false
	}
	if len(i.latest) >= maxConversationClocks {
		for key := range i.latest {
			delete(i.latest, key)
			break
		}
	}
	i.latest[scope] = occurredAt
	return true
}

func eventTime(event transport.Event) time.Time {
	if !event.OccurredAt.IsZero() {
		return event.OccurredAt
	}
	if event.Message != nil {
		return event.Message.CreatedAt
	}
	if event.Comment != nil {
		return event.Comment.CreatedAt
	}
	if event.CardAction != nil {
		return event.CardAction.CreatedAt
	}
	return time.Time{}
}

func (i *Intake) run(ctx context.Context) {
	defer close(i.done)
	select {
	case <-ctx.Done():
		return
	case <-i.activate:
	}
	workers := make(chan struct{}, defaultIntakeWorkers)
	var running sync.WaitGroup
	defer running.Wait()
	for {
		select {
		case <-ctx.Done():
			return
		case item := <-i.queue:
			select {
			case workers <- struct{}{}:
			case <-ctx.Done():
				return
			}
			running.Add(1)
			go func() {
				defer running.Done()
				defer func() { <-workers }()
				i.process(ctx, item)
			}()
		}
	}
}

func (i *Intake) process(ctx context.Context, item intakeItem) {
	var err error
	switch {
	case item.stop != nil:
		if stopper, ok := i.runner.(interface {
			Stop(context.Context, channeltypes.InboundMessage) error
		}); ok {
			err = stopper.Stop(ctx, *item.stop)
		} else {
			err = fmt.Errorf("Feishu task cancellation is unavailable")
		}
	case item.message != nil:
		message := *item.message
		if hydrated, hydrateErr := hydrateQuotedMessage(ctx, i.messages, message); hydrateErr != nil {
			slog.Warn("load quoted Feishu message failed",
				inboundMessageLogAttrs(message,
					"quoted_message_id", firstNonEmpty(message.Source.ParentID, message.Source.RootID),
					"error", hydrateErr,
				)...)
		} else {
			message = hydrated
		}
		if i.runner.IsResetCommand(message.Text) {
			err = i.runner.Reset(ctx, message)
		} else {
			err = i.runner.Submit(ctx, message)
		}

	case item.card != nil:
		card := *item.card
		if !card.trusted {
			err = i.completeCardResult(card, expiredCardActionText)
		} else if card.input.Action.Operation == interaction.OperationReset {
			err = i.runner.Reset(ctx, cardResetMessage(card))
		} else {
			err = i.handleCard(ctx, card)
		}

	case item.comment != nil:
		message, executable, prepareErr := prepareCommentMessage(ctx, i.binding, i.comments, *item.comment)
		if prepareErr != nil {
			err = prepareErr
		} else if executable {
			err = i.runner.Submit(ctx, message)
		}
	}
	if err != nil && ctx.Err() == nil {
		slog.Warn("process Feishu inbound event failed", intakeItemLogAttrs(i.binding, item, "error", err)...)
	}
}

func cardResetMessage(card normalizedCardAction) channeltypes.InboundMessage {
	return channeltypes.InboundMessage{
		Source:          card.source,
		AgentID:         card.input.AgentID,
		ConversationKey: card.input.ConversationKey,
		TurnID:          card.turnID,
		Text:            "/new",
	}
}

func (i *Intake) handleCard(ctx context.Context, card normalizedCardAction) error {
	if card.input.Action.Operation == interaction.OperationCancel {
		canceler, ok := i.runner.(interface {
			CancelRequest(context.Context, interaction.CancelRequest) error
		})
		if !ok {
			return i.completeCardResult(card, "任务取消暂时不可用。")
		}
		err := canceler.CancelRequest(ctx, interaction.CancelRequest{BindingID: i.binding.ID, AgentID: card.input.AgentID, ConversationKey: card.input.ConversationKey, TurnID: card.input.TurnID, MessageID: card.source.MessageID, ChatID: card.source.ChatID, ThreadID: card.source.ThreadID, RequesterID: card.input.ResponderID})
		if err != nil {
			return i.completeCardResult(card, cardActionErrorText(err))
		}
		return nil
	}
	if card.input.Action.Operation == interaction.OperationResolve {
		if runner, ok := i.runner.(interface {
			ResolveInteraction(context.Context, interaction.Input) error
		}); ok {
			err := runner.ResolveInteraction(ctx, card.input)
			if err != nil {
				return i.completeCardResult(card, err.Error())
			}
			return nil
		}
		return i.completeCardResult(card, "此确认暂时无法提交。")
	}
	var err error
	if i.interactions == nil {
		err = fmt.Errorf("Feishu card action handler is unavailable")
	} else {
		err = i.interactions.Handle(ctx, card.input)
	}
	text := card.successText
	if err != nil {
		text = cardActionErrorText(err)
	}
	return i.completeCardResult(card, text)
}

func (i *Intake) completeCardResult(card normalizedCardAction, text string) error {
	intent := channeltypes.DeliveryIntent{
		ID:        card.turnID + ":card-action-result",
		BindingID: i.binding.ID,
		TurnID:    card.turnID,
		Kind:      channeltypes.DeliveryCard,
		ChatID:    card.source.ChatID,
		ReplyTo:   card.source.MessageID,
		ThreadID:  card.source.ThreadID,
		Card:      presentation.Card(text),
	}
	if err := i.state.Enqueue(intent); err != nil {
		return err
	}
	if i.notifier != nil {
		i.notifier.Notify()
	}
	return nil
}

func (i *Intake) Close() {
	if i == nil {
		return
	}
	i.mu.Lock()
	cancel := i.cancel
	if cancel != nil {
		cancel()
		i.cancel = nil
	}
	i.mu.Unlock()
	if cancel != nil {
		<-i.done
	}
}

var _ transport.Sink = (*Intake)(nil)
