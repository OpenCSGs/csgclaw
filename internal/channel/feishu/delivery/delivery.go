package delivery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	channeltypes "csgclaw/internal/channel"
	feishustate "csgclaw/internal/channel/feishu/state"
	"csgclaw/internal/channel/feishu/transport"
)

const (
	defaultRetryInterval = 2 * time.Second
	maxDeliveryAttempts  = 3
)

var (
	ErrDeliverySuperseded = errors.New("feishu delivery superseded by the terminal presentation")
)

type DispatcherOptions struct {
	State         *feishustate.Store
	Adapter       transport.Adapter
	Files         FileResolver
	RetryInterval time.Duration
}

// Dispatcher drains process-local delivery intents outside Agent Engine event
// sinks. Retries are bounded and are not recovered after process restart.
type Dispatcher struct {
	state    *feishustate.Store
	adapter  transport.Adapter
	files    FileResolver
	interval time.Duration
	wake     chan struct{}
	cotWake  chan struct{}

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	cotDone chan struct{}

	uploadMu sync.Mutex
	uploads  map[string]mediaUpload
}

func NewDispatcher(options DispatcherOptions) (*Dispatcher, error) {
	if options.State == nil {
		return nil, fmt.Errorf("feishu delivery dispatcher: state is required")
	}
	if options.Adapter == nil {
		return nil, fmt.Errorf("feishu delivery dispatcher: transport adapter is required")
	}
	interval := options.RetryInterval
	if interval <= 0 {
		interval = defaultRetryInterval
	}
	return &Dispatcher{
		state:    options.State,
		adapter:  options.Adapter,
		files:    options.Files,
		interval: interval,
		wake:     make(chan struct{}, 1),
		cotWake:  make(chan struct{}, 1),
		done:     make(chan struct{}),
		cotDone:  make(chan struct{}),
	}, nil
}

func (d *Dispatcher) Start(ctx context.Context) error {
	if d == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	d.mu.Lock()
	if d.cancel != nil {
		d.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	d.mu.Unlock()
	go d.run(runCtx)
	go d.runCOT(runCtx)
	d.Notify()
	return nil
}

func (d *Dispatcher) Notify() {
	if d == nil {
		return
	}
	select {
	case d.wake <- struct{}{}:
	default:
	}
	select {
	case d.cotWake <- struct{}{}:
	default:
	}
}

func (d *Dispatcher) Close() {
	if d == nil {
		return
	}
	d.mu.Lock()
	cancel := d.cancel
	if cancel != nil {
		cancel()
		d.cancel = nil
	}
	d.mu.Unlock()
	if cancel != nil {
		<-d.done
		<-d.cotDone
	}
}

func (d *Dispatcher) run(ctx context.Context) {
	defer close(d.done)
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.wake:
			d.drain(ctx)
		case <-ticker.C:
			d.drain(ctx)
		}
	}
}

func (d *Dispatcher) drain(ctx context.Context) {
	pending := d.state.Pending()
	superseded := d.supersededDeliveries(pending)
	blockedScopes := make(map[string]struct{})
	for _, intent := range pending {
		if isCOT(intent.Kind) {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		lane := deliveryLane(intent)
		if _, blocked := blockedScopes[lane]; blocked {
			continue
		}
		if _, stale := superseded[intent.ID]; stale {
			if err := d.state.MarkFailed(intent.ID, ErrDeliverySuperseded); err != nil {
				slog.Error("record superseded Feishu presentation delivery failed",
					intentLogAttrs(intent, "error", err)...)
				blockedScopes[lane] = struct{}{}
			}
			continue
		}
		if !retryReady(intent, time.Now()) {
			blockedScopes[lane] = struct{}{}
			continue
		}
		if err := d.state.Begin(intent.ID); err != nil {
			slog.Error("begin Feishu delivery failed", intentLogAttrs(intent, "error", err)...)
			blockedScopes[lane] = struct{}{}
			continue
		}
		slog.Debug("dispatch Feishu delivery intent", intentLogAttrs(intent)...)
		delivered, err := d.deliver(ctx, intent)
		if err != nil {
			failedAt := time.Now()
			var markErr error
			retry := false
			var nextAttemptAt time.Time
			if errors.Is(err, ErrDependencyTerminal) {
				markErr = d.state.MarkFailed(intent.ID, err)
			} else if errors.Is(err, ErrDependencyPending) || retryableDelivery(intent, err) {
				retry = true
				nextAttemptAt = nextRetryAt(failedAt, d.interval, intent.Attempts+1)
				markErr = d.state.MarkRetryable(intent.ID, err, nextAttemptAt)
			} else {
				// Permanent and unclassified failures are terminal.
				// Delivery failure never causes the Agent turn to run again.
				markErr = d.state.MarkFailed(intent.ID, err)
			}
			if markErr != nil {
				slog.Error("record Feishu delivery failure failed",
					deliveryErrorLogAttrs(intent, err, "record_error", markErr)...)
				blockedScopes[lane] = struct{}{}
			} else if !retry && intent.Kind == channeltypes.DeliveryFile {
				d.discardMediaUpload(intent.ID)
			}
			d.logDeliveryFailure(intent, err, retry, nextAttemptAt)
			if retry {
				blockedScopes[lane] = struct{}{}
			}
			continue
		}
		if err := d.state.MarkDelivered(delivered); err != nil {
			slog.Error("record delivered Feishu intent failed", intentLogAttrs(delivered, "error", err)...)
			blockedScopes[lane] = struct{}{}
			continue
		}
		if intent.Kind == channeltypes.DeliveryFile {
			d.discardMediaUpload(intent.ID)
		}
		slog.Debug("delivered Feishu intent", intentLogAttrs(delivered)...)
	}
}

func (d *Dispatcher) logDeliveryFailure(intent channeltypes.DeliveryIntent, err error, retry bool, nextAttemptAt time.Time) {
	attrs := deliveryErrorLogAttrs(intent, err, "retry", retry)
	if !nextAttemptAt.IsZero() {
		attrs = append(attrs, "next_attempt_at", nextAttemptAt)
	}
	if retry {
		slog.Warn("Feishu delivery failed; will retry", attrs...)
		return
	}
	slog.Error("Feishu delivery failed permanently", attrs...)
}

func (d *Dispatcher) supersededDeliveries(pending []channeltypes.DeliveryIntent) map[string]struct{} {
	superseded := supersededPresentationUpdates(pending)
	for _, intent := range pending {
		// A terminal snapshot is the authoritative representation of a Turn. It
		// must also win over an older update that became pending after a retry.
		// The terminal update may already be delivered while an older streaming
		// update remains retryable.
		if d.supersededByTerminalPresentation(intent) {
			if superseded == nil {
				superseded = make(map[string]struct{})
			}
			superseded[intent.ID] = struct{}{}
		}
	}
	return superseded
}

func (d *Dispatcher) supersededByTerminalPresentation(intent channeltypes.DeliveryIntent) bool {
	if d == nil || d.state == nil || !presentationUpdate(intent.Kind) {
		return false
	}
	terminalID := presentationTerminalID(intent)
	if terminalID == "" {
		return false
	}
	terminal, ok := d.state.Intent(terminalID)
	return ok && terminal.ID != intent.ID && terminal.Kind == intent.Kind &&
		terminal.TurnID == intent.TurnID && terminal.BindingID == intent.BindingID
}

func presentationTerminalID(intent channeltypes.DeliveryIntent) string {
	if !presentationUpdate(intent.Kind) {
		return ""
	}
	relatedID := strings.TrimSpace(intent.RelatedID)
	if relatedID == "" || !strings.HasSuffix(relatedID, ":create") {
		return ""
	}
	return strings.TrimSuffix(relatedID, ":create") + ":final"
}

func supersededPresentationUpdates(pending []channeltypes.DeliveryIntent) map[string]struct{} {
	newest := make(map[string]channeltypes.DeliveryIntent)
	for _, intent := range pending {
		if !presentationUpdate(intent.Kind) {
			continue
		}
		lane := deliveryLane(intent)
		current, ok := newest[lane]
		if !ok || presentationUpdateAfter(intent, current) {
			newest[lane] = intent
		}
	}
	if len(newest) == 0 {
		return nil
	}
	superseded := make(map[string]struct{})
	for _, intent := range pending {
		if !presentationUpdate(intent.Kind) {
			continue
		}
		if latest := newest[deliveryLane(intent)]; latest.ID != intent.ID {
			superseded[intent.ID] = struct{}{}
		}
	}
	return superseded
}

func presentationUpdate(kind channeltypes.DeliveryKind) bool {
	return kind == channeltypes.DeliveryCardUpdate
}

func presentationUpdateAfter(candidate, current channeltypes.DeliveryIntent) bool {
	if candidate.Sequence != current.Sequence {
		return candidate.Sequence > current.Sequence
	}
	if !candidate.CreatedAt.Equal(current.CreatedAt) {
		return candidate.CreatedAt.After(current.CreatedAt)
	}
	return candidate.ID > current.ID
}

func deliveryScope(intent channeltypes.DeliveryIntent) string {
	if intent.Kind == channeltypes.DeliveryCommentReply {
		return "comment:\x00" + intent.ResourceType + "\x00" + intent.ResourceID + "\x00" + intent.ParentID
	}
	if intent.ChatID == "" {
		return "message:\x00" + intent.MessageID
	}
	return intent.ChatID + "\x00" + intent.ThreadID + "\x00" + intent.ReplyTo
}

func deliveryLane(intent channeltypes.DeliveryIntent) string {
	scope := deliveryScope(intent)
	switch intent.Kind {
	case channeltypes.DeliveryCard:
		return scope + "\x00card\x00" + intent.TurnID + "\x00" + intent.ID
	case channeltypes.DeliveryCardUpdate:
		return scope + "\x00card\x00" + intent.TurnID + "\x00" + presentationMessageID(intent)
	case channeltypes.DeliveryReactionAdd, channeltypes.DeliveryReactionDelete:
		return scope + "\x00reaction\x00" + intent.TurnID
	default:
		return scope + "\x00message"
	}
}

func presentationMessageID(intent channeltypes.DeliveryIntent) string {
	if relatedID := strings.TrimSpace(intent.RelatedID); relatedID != "" {
		return relatedID
	}
	return intent.ID
}

func retryReady(intent channeltypes.DeliveryIntent, now time.Time) bool {
	if intent.NextAttemptAt == nil {
		return true
	}
	return !now.Before(*intent.NextAttemptAt)
}

func nextRetryAt(now time.Time, base time.Duration, attempt int) time.Time {
	if base <= 0 {
		base = defaultRetryInterval
	}
	shift := min(max(attempt-1, 0), 5)
	return now.Add(base * time.Duration(1<<shift))
}

func retryableDelivery(intent channeltypes.DeliveryIntent, err error) bool {
	attempt := intent.Attempts + 1
	if attempt >= maxDeliveryAttempts || !transport.IsRetryable(err) {
		return false
	}
	// Creates carry a stable Feishu UUID; updates and deletion target stable
	// remote IDs. Reaction creation and comment reply have no equivalent
	// deduplication key, so an ambiguous outcome must not be repeated.
	switch intent.Kind {
	case channeltypes.DeliveryCard,
		channeltypes.DeliveryFile,
		channeltypes.DeliveryCardUpdate, channeltypes.DeliveryReactionDelete:
		return true
	default:
		return false
	}
}

func (d *Dispatcher) deliver(ctx context.Context, intent channeltypes.DeliveryIntent) (channeltypes.DeliveryIntent, error) {
	switch intent.Kind {
	case channeltypes.DeliveryFile:
		return d.deliverMedia(ctx, intent)
	case channeltypes.DeliveryCard:
		if intent.RelatedID != "" {
			if _, err := d.cardUpdateMessageID(intent); err != nil {
				return intent, err
			}
		}
		return deliverCard(ctx, d.adapter, intent)
	case channeltypes.DeliveryCardUpdate:
		return d.deliverCardUpdate(ctx, intent)
	case channeltypes.DeliveryReactionAdd:
		return deliverReactionAdd(ctx, d.adapter, intent)
	case channeltypes.DeliveryReactionDelete:
		return d.deliverReactionDelete(ctx, intent)
	case channeltypes.DeliveryCommentReply:
		comments, ok := d.adapter.(transport.CommentAdapter)
		if !ok {
			return intent, fmt.Errorf("Feishu comment delivery is unavailable")
		}
		return deliverCommentReply(ctx, comments, intent)
	default:
		return intent, fmt.Errorf("unsupported Feishu delivery kind %q", intent.Kind)
	}
}
