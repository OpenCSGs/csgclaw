package execution

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/worklease"
)

const (
	defaultWorkLeaseTTL      = worklease.DefaultTTLSeconds
	defaultWorkRenewEvery    = 5 * time.Second
	defaultWorkFinishTimeout = 2 * time.Second
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
	lease    worklease.ParticipantWorkLease
	cancel   context.CancelFunc
	stop     func(context.Context) error
	finished bool
	stopping bool

	mu sync.Mutex
}

func (a *Adapter) startWork(ctx context.Context, turn channel.TurnContext) (context.Context, func(agentengine.TurnResult)) {
	if a == nil || a.work.reporter == nil {
		return ctx, func(agentengine.TurnResult) {}
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

	closed := false
	if _, err := a.work.reporter.StartOrRenew(turnCtx, lease); err != nil {
		closed = errors.Is(err, worklease.ErrClosed)
		logWorkFailure("start", lease, err)
	}
	if !closed && advertiseStop {
		if statusReporter, ok := a.work.reporter.(worklease.ParticipantWorkStatusReporter); ok {
			if _, _, err := statusReporter.UpdateStatus(turnCtx, lease.ParticipantID, lease.LeaseID, apitypes.ParticipantWorkStatusPatchRequest{
				Capabilities: []string{apitypes.ParticipantWorkCapabilityTurnStopV1},
				Sequence:     1,
				Phase:        apitypes.ParticipantWorkPhaseWorking,
			}); err != nil {
				logWorkFailure("advertise stop capability", lease, err)
			}
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
	}
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
