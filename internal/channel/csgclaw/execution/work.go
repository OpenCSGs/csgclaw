package execution

import (
	"context"
	"csgclaw/internal/modelcap"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/worklease"
)

const (
	defaultWorkLeaseTTL        = worklease.DefaultTTLSeconds
	defaultWorkRenewEvery      = 5 * time.Second
	defaultWorkFinishTimeout   = 2 * time.Second
	defaultThinkingUpdateEvery = 400 * time.Millisecond
)

type workOptions struct {
	reporter      worklease.ParticipantWorkReporter
	turnControls  agentruntime.TurnControllerRegistrar
	ttlSeconds    int
	renewEvery    time.Duration
	finishTimeout time.Duration
}

func defaultWorkOptions() workOptions {
	return workOptions{
		ttlSeconds:    defaultWorkLeaseTTL,
		renewEvery:    defaultWorkRenewEvery,
		finishTimeout: defaultWorkFinishTimeout,
	}
}

type activeWorkTurn struct {
	lease          worklease.ParticipantWorkLease
	cancel         context.CancelFunc
	stop           func(context.Context) error
	statusReporter worklease.ParticipantWorkStatusReporter
	capabilities   []string
	statusSequence uint64
	statusStage    string
	thinking       string
	truncated      bool
	lastThinking   time.Time
	finished       bool
	stopping       bool

	mu sync.Mutex
}

func (a *Adapter) startWork(ctx context.Context, turn channel.TurnContext) (
	context.Context,
	func(agentengine.TurnResult),
	func(context.Context, agentengine.TurnEvent),
) {
	if a == nil || a.work.reporter == nil {
		return ctx, func(agentengine.TurnResult) {}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	turnCtx, cancelTurn := context.WithCancel(ctx)
	lease := worklease.ParticipantWorkLease{
		ParticipantID: strings.TrimSpace(turn.ParticipantID),
		LeaseID:       worklease.NewID(),
		RoomID:        strings.TrimSpace(turn.RoomID),
		ThreadRootID:  strings.TrimSpace(turn.ThreadRootID),
		RequestID:     strings.TrimSpace(turn.SourceMessageID),
		TaskID:        strings.TrimSpace(turn.TaskID),
		TaskAttempt:   turn.TaskAttempt,
		Kind:          apitypes.ParticipantWorkKindAgentTurn,
		TTLSeconds:    a.work.ttlSeconds,
		TTLExplicit:   true,
	}
	active := &activeWorkTurn{
		lease:  lease,
		cancel: cancelTurn,
		stop: func(stopCtx context.Context) error {
			return a.Cancel(stopCtx, turn.AgentID, turn.ConversationKey, turn.TurnID)
		},
	}
	unregister := func() {}
	advertiseStop := a.work.turnControls != nil
	if advertiseStop {
		unregister = a.work.turnControls.RegisterTurnController(lease.ParticipantID, active)
		if unregister == nil {
			unregister = func() {}
		}
	}

	statusReporter, reportsStatus := a.work.reporter.(worklease.ParticipantWorkStatusReporter)
	if reportsStatus {
		active.statusReporter = statusReporter
		active.capabilities = []string{
			apitypes.ParticipantWorkCapabilityThinkingStatusV1,
			apitypes.ParticipantWorkCapabilityStageV1,
		}
		if advertiseStop {
			active.capabilities = append(active.capabilities, apitypes.ParticipantWorkCapabilityTurnStopV1)
		}
		active.statusSequence = 1
		active.statusStage = apitypes.ParticipantWorkStagePreparingReply
	}

	closed := false
	if _, err := a.work.reporter.StartOrRenew(turnCtx, lease); err != nil {
		closed = errors.Is(err, worklease.ErrClosed)
		logWorkFailure("start", lease, err)
	}
	if !closed && reportsStatus {
		if _, _, err := statusReporter.UpdateStatus(turnCtx, lease.ParticipantID, lease.LeaseID, apitypes.ParticipantWorkStatusPatchRequest{
			Capabilities: append([]string(nil), active.capabilities...),
			Sequence:     active.statusSequence,
			Phase:        apitypes.ParticipantWorkPhaseThinking,
			Stage:        active.statusStage,
		}); err != nil {
			logWorkFailure("publish initial status", lease, err)
		}
	}

	renewCtx, cancelRenew := context.WithCancel(turnCtx)
	renewDone := make(chan struct{})
	if closed {
		close(renewDone)
	} else {
		go renewWorkLease(renewCtx, renewDone, a.work.reporter, lease, a.work.renewEvery)
	}

	var once sync.Once
	return turnCtx, func(result agentengine.TurnResult) {
		once.Do(func() {
			stopping := active.finish()
			unregister()
			cancelRenew()
			cancelTurn()
			<-renewDone

			outcome := apitypes.ParticipantWorkOutcomeReleased
			if stopping {
				if result.Status == agentengine.TurnCanceled {
					outcome = apitypes.ParticipantWorkOutcomeStopped
				} else {
					outcome = apitypes.ParticipantWorkOutcomeStopTimedOut
				}
			}
			finishCtx, cancelFinish := context.WithTimeout(context.WithoutCancel(ctx), a.work.finishTimeout)
			defer cancelFinish()
			var err error
			if finisher, ok := a.work.reporter.(worklease.ParticipantWorkFinisher); ok {
				err = finisher.Finish(finishCtx, lease.ParticipantID, lease.LeaseID, outcome)
			} else {
				err = a.work.reporter.Stop(finishCtx, lease.ParticipantID, lease.LeaseID)
			}
			if err != nil {
				logWorkFailure("finish", lease, err)
			}
		})
	}, active.observeEvent
}

func (t *activeWorkTurn) observeEvent(ctx context.Context, event agentengine.TurnEvent) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.finished || t.statusReporter == nil {
		t.mu.Unlock()
		return
	}

	now := time.Now()
	request := apitypes.ParticipantWorkStatusPatchRequest{}
	switch event.Kind {
	case agentengine.TurnEventActivityUpdate:
		if event.Activity == nil || event.Activity.Kind != modelcap.ContextUsageKind {
			t.mu.Unlock()
			return
		}
		raw, err := json.Marshal(event.Activity.Payload)
		var usage modelcap.ContextUsage
		if err != nil || json.Unmarshal(raw, &usage) != nil || !usage.Valid() {
			t.mu.Unlock()
			return
		}
		request.ContextUsage = &usage
		request.Stage = t.statusStage
		request.Phase = apitypes.ParticipantWorkPhaseWorking
		if t.statusStage == apitypes.ParticipantWorkStageThinking || t.statusStage == apitypes.ParticipantWorkStagePreparingReply {
			request.Phase = apitypes.ParticipantWorkPhaseThinking
			if t.statusStage == apitypes.ParticipantWorkStageThinking {
				request.Thinking = &apitypes.ParticipantThinkingStatus{Format: apitypes.ParticipantThinkingFormatPlainText, Text: t.thinking, Truncated: t.truncated}
			}
		}
	case agentengine.TurnEventThoughtDelta:
		if event.Thought == "" {
			t.mu.Unlock()
			return
		}
		t.thinking, t.truncated = appendThinkingTail(t.thinking, event.Thought, t.truncated)
		if strings.TrimSpace(t.thinking) == "" ||
			(t.statusStage == apitypes.ParticipantWorkStageThinking &&
				!t.lastThinking.IsZero() && now.Sub(t.lastThinking) < defaultThinkingUpdateEvery) {
			t.mu.Unlock()
			return
		}
		t.statusStage = apitypes.ParticipantWorkStageThinking
		t.lastThinking = now
		request.Phase = apitypes.ParticipantWorkPhaseThinking
		request.Stage = t.statusStage
		request.Thinking = &apitypes.ParticipantThinkingStatus{
			Format:    apitypes.ParticipantThinkingFormatPlainText,
			Text:      t.thinking,
			Truncated: t.truncated,
		}
	case agentengine.TurnEventToolCallStart, agentengine.TurnEventToolCallUpdate:
		if t.statusStage == apitypes.ParticipantWorkStageRunningTool {
			t.mu.Unlock()
			return
		}
		t.statusStage = apitypes.ParticipantWorkStageRunningTool
		request.Phase = apitypes.ParticipantWorkPhaseWorking
		request.Stage = t.statusStage
	case agentengine.TurnEventTextDelta:
		if event.Text == "" || t.statusStage == apitypes.ParticipantWorkStageGeneratingReply {
			t.mu.Unlock()
			return
		}
		t.statusStage = apitypes.ParticipantWorkStageGeneratingReply
		request.Phase = apitypes.ParticipantWorkPhaseWorking
		request.Stage = t.statusStage
	default:
		t.mu.Unlock()
		return
	}

	t.statusSequence++
	request.Sequence = t.statusSequence
	request.Capabilities = append([]string(nil), t.capabilities...)
	reporter := t.statusReporter
	lease := t.lease
	t.mu.Unlock()

	if _, _, err := reporter.UpdateStatus(ctx, lease.ParticipantID, lease.LeaseID, request); err != nil {
		logWorkFailure("publish runtime status", lease, err)
	}
}

func appendThinkingTail(current, delta string, alreadyTruncated bool) (string, bool) {
	value := current + delta
	limit := worklease.MaxThinkingBytes
	if len(value) <= limit {
		return value, alreadyTruncated
	}
	start := len(value) - limit
	for start < len(value) && !utf8.RuneStart(value[start]) {
		start++
	}
	return value[start:], true
}

func renewWorkLease(
	ctx context.Context,
	done chan<- struct{},
	reporter worklease.ParticipantWorkReporter,
	lease worklease.ParticipantWorkLease,
	every time.Duration,
) {
	defer close(done)
	if every <= 0 {
		return
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := reporter.StartOrRenew(ctx, lease); err != nil {
				logWorkFailure("renew", lease, err)
				if errors.Is(err, worklease.ErrClosed) {
					return
				}
			}
		}
	}
}

func (t *activeWorkTurn) StopTurn(ctx context.Context, ref agentruntime.TurnRef) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if t == nil || !sameWorkTurn(t.lease, ref) {
		return agentruntime.ErrTurnNotFound
	}
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return agentruntime.ErrTurnNotFound
	}
	t.stopping = true
	cancel := t.cancel
	stop := t.stop
	t.mu.Unlock()
	// Cancel the caller context first so a stop that races Engine admission
	// prevents dispatch. The exact Engine Cancel then waits for an admitted
	// Runtime turn to finish cleanup.
	if cancel != nil {
		cancel()
	}
	if stop != nil {
		return stop(ctx)
	}
	return nil
}

func (t *activeWorkTurn) finish() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.finished = true
	return t.stopping
}

func sameWorkTurn(lease worklease.ParticipantWorkLease, ref agentruntime.TurnRef) bool {
	return strings.TrimSpace(lease.ParticipantID) == strings.TrimSpace(ref.ParticipantID) &&
		strings.TrimSpace(lease.RoomID) == strings.TrimSpace(ref.RoomID) &&
		strings.TrimSpace(lease.LeaseID) == strings.TrimSpace(ref.LeaseID) &&
		strings.TrimSpace(lease.RequestID) == strings.TrimSpace(ref.RequestID)
}

func logWorkFailure(action string, lease worklease.ParticipantWorkLease, err error) {
	if err == nil {
		return
	}
	slog.Warn("built-in IM participant work lease "+action+" failed",
		"participant_id", lease.ParticipantID,
		"room_id", lease.RoomID,
		"message_id", lease.RequestID,
		"lease_id", lease.LeaseID,
		"error", err,
	)
}
