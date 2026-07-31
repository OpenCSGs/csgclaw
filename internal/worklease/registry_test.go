package worklease

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	agentruntime "csgclaw/internal/runtime"
)

func TestRegistryLeaseLifecycleAndTombstone(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	registry, events := newTestRegistry(t, &now)
	lease := testLease(NewID())

	started, err := registry.StartOrRenew(context.Background(), lease)
	if err != nil {
		t.Fatal(err)
	}
	if started.Revision != 1 || started.Reason != apitypes.ParticipantWorkReasonStarted || started.State != apitypes.ParticipantWorkStateWorking {
		t.Fatalf("started update = %#v", started)
	}
	if started.ParticipantID != "pt-worker" || started.UserID != "user-worker" {
		t.Fatalf("normalized identities = %q, %q", started.ParticipantID, started.UserID)
	}
	if want := now.Add(15 * time.Second); !started.ExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %s, want %s", started.ExpiresAt, want)
	}
	assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStarted, 1)

	now = now.Add(5 * time.Second)
	renewed, err := registry.StartOrRenew(context.Background(), lease)
	if err != nil {
		t.Fatal(err)
	}
	if renewed.Revision != 2 || renewed.Reason != apitypes.ParticipantWorkReasonRenewed {
		t.Fatalf("renewed update = %#v", renewed)
	}
	assertWorkEvent(t, events, apitypes.ParticipantWorkReasonRenewed, 2)

	conflict := lease
	conflict.RequestID = "message-other"
	if _, err := registry.StartOrRenew(context.Background(), conflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting renew error = %v, want conflict", err)
	}

	if err := registry.Stop(context.Background(), "worker", lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	assertWorkEvent(t, events, apitypes.ParticipantWorkReasonReleased, 3)
	if got := registry.ActiveCount("room-1", "pt-worker"); got != 0 {
		t.Fatalf("active count = %d, want 0", got)
	}
	if err := registry.Stop(context.Background(), "pt-worker", lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	assertNoWorkEvent(t, events)
	if _, err := registry.StartOrRenew(context.Background(), lease); !errors.Is(err, ErrClosed) {
		t.Fatalf("late put error = %v, want closed", err)
	}

	now = now.Add(TombstoneTTL + time.Second)
	registry.Sweep(now)
	restarted, err := registry.StartOrRenew(context.Background(), lease)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Revision != 1 || restarted.Reason != apitypes.ParticipantWorkReasonStarted {
		t.Fatalf("restarted update = %#v", restarted)
	}
}

func TestRegistryUnknownDeleteAndExpiry(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	registry, events := newTestRegistry(t, &now)
	unknownID := NewID()
	if err := registry.Stop(context.Background(), "pt-worker", unknownID); err != nil {
		t.Fatal(err)
	}
	assertNoWorkEvent(t, events)
	unknown := testLease(unknownID)
	if _, err := registry.StartOrRenew(context.Background(), unknown); !errors.Is(err, ErrClosed) {
		t.Fatalf("put after unknown delete error = %v, want closed", err)
	}

	lease := testLease(NewID())
	lease.TTLSeconds = 1
	lease.TTLExplicit = true
	started, err := registry.StartOrRenew(context.Background(), lease)
	if err != nil {
		t.Fatal(err)
	}
	if want := now.Add(5 * time.Second); !started.ExpiresAt.Equal(want) {
		t.Fatalf("clamped lower expiry = %s, want %s", started.ExpiresAt, want)
	}
	assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStarted, 1)
	now = started.ExpiresAt
	registry.Sweep(now)
	expired := assertWorkEvent(t, events, apitypes.ParticipantWorkReasonExpired, 2)
	if !expired.ExpiresAt.Equal(started.ExpiresAt) {
		t.Fatalf("expired event changed expires_at: %s != %s", expired.ExpiresAt, started.ExpiresAt)
	}
}

func TestRegistryKeepsConcurrentLeasesIndependent(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	registry, _ := newTestRegistry(t, &now)
	first := testLease(NewID())
	second := testLease(NewID())
	second.RequestID = "message-2"
	if _, err := registry.StartOrRenew(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.StartOrRenew(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if got := registry.ActiveCount("room-1", "user-worker"); got != 2 {
		t.Fatalf("active count = %d, want 2", got)
	}
	if err := registry.Stop(context.Background(), first.ParticipantID, first.LeaseID); err != nil {
		t.Fatal(err)
	}
	if got := registry.ActiveCount("room-1", "pt-worker"); got != 1 {
		t.Fatalf("active count after first release = %d, want 1", got)
	}
}

func TestRegistryConcurrentRenewAndReleaseCannotResurrectLease(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	registry, _ := newTestRegistry(t, &now)
	lease := testLease(NewID())
	if _, err := registry.StartOrRenew(context.Background(), lease); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	var workers sync.WaitGroup
	for index := 0; index < 32; index++ {
		workers.Add(1)
		go func(release bool) {
			defer workers.Done()
			<-start
			if release {
				_ = registry.Stop(context.Background(), lease.ParticipantID, lease.LeaseID)
				return
			}
			_, _ = registry.StartOrRenew(context.Background(), lease)
		}(index%2 == 0)
	}
	close(start)
	workers.Wait()

	if got := registry.ActiveCount(lease.RoomID, lease.ParticipantID); got != 0 {
		t.Fatalf("active count after concurrent release = %d, want 0", got)
	}
	if _, err := registry.StartOrRenew(context.Background(), lease); !errors.Is(err, ErrClosed) {
		t.Fatalf("renew after concurrent release error = %v, want closed", err)
	}
}

func TestRegistryConstructionUsesNewEpoch(t *testing.T) {
	first := NewRegistry(nil, nil, nil)
	second := NewRegistry(nil, nil, nil)
	if first.Epoch() == "" || second.Epoch() == "" || first.Epoch() == second.Epoch() {
		t.Fatalf("registry epochs = %q and %q", first.Epoch(), second.Epoch())
	}
}

func TestRegistryStatusAndTurnStopLifecycle(t *testing.T) {
	now := time.Date(2026, 7, 20, 3, 0, 0, 0, time.UTC)
	registry, events := newTestRegistry(t, &now)
	controls, cancelControls := registry.controlBus.Subscribe("pt-worker")
	t.Cleanup(cancelControls)
	lease := testLease(NewID())

	started, err := registry.StartOrRenew(context.Background(), lease)
	if err != nil {
		t.Fatal(err)
	}
	assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStarted, 1)
	statusRequest := apitypes.ParticipantWorkStatusPatchRequest{
		Capabilities: []string{
			apitypes.ParticipantWorkCapabilityThinkingStatusV1,
			apitypes.ParticipantWorkCapabilityTurnStopV1,
			apitypes.ParticipantWorkCapabilityStageV1,
		},
		Sequence: 1,
		Phase:    apitypes.ParticipantWorkPhaseThinking,
		Stage:    apitypes.ParticipantWorkStageThinking,
		Thinking: &apitypes.ParticipantThinkingStatus{
			Format: apitypes.ParticipantThinkingFormatPlainText,
			Text:   "checking configuration",
		},
	}
	status, accepted, err := registry.UpdateStatus(context.Background(), "worker", lease.LeaseID, statusRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !accepted || status.Revision != 2 || !status.ExpiresAt.Equal(started.ExpiresAt) {
		t.Fatalf("status update = %#v, accepted=%v", status, accepted)
	}
	if status.Status == nil || status.Status.Stage != apitypes.ParticipantWorkStageThinking ||
		status.Status.Thinking == nil || status.Status.Thinking.Text != "checking configuration" {
		t.Fatalf("thinking status = %#v", status.Status)
	}
	assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStatusUpdated, 2)

	if _, accepted, err := registry.UpdateStatus(context.Background(), "worker", lease.LeaseID, statusRequest); err != nil || accepted {
		t.Fatalf("stale status accepted=%v err=%v", accepted, err)
	}
	assertNoWorkEvent(t, events)

	now = now.Add(time.Second)
	stop, err := registry.RequestStop(context.Background(), "worker", apitypes.ParticipantWorkStopRequest{
		RoomID:    lease.RoomID,
		LeaseID:   lease.LeaseID,
		RequestID: lease.RequestID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !stop.Accepted || stop.State != "stop_requested" || !stop.RequestedAt.Equal(now) {
		t.Fatalf("stop response = %#v", stop)
	}
	stopping := assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStopRequested, 3)
	if stopping.StopRequestedAt == nil || !stopping.StopRequestedAt.Equal(now) {
		t.Fatalf("stop update = %#v", stopping)
	}
	select {
	case control := <-controls:
		if control.LeaseID != lease.LeaseID || control.RequestID != lease.RequestID {
			t.Fatalf("control = %#v", control)
		}
	default:
		t.Fatal("missing stop control")
	}

	repeated, err := registry.RequestStop(context.Background(), "worker", apitypes.ParticipantWorkStopRequest{
		RoomID:    lease.RoomID,
		LeaseID:   lease.LeaseID,
		RequestID: lease.RequestID,
	})
	if err != nil || !repeated.RequestedAt.Equal(stop.RequestedAt) {
		t.Fatalf("repeated stop = %#v, err=%v", repeated, err)
	}
	assertNoWorkEvent(t, events)

	statusRequest.Sequence = 2
	statusRequest.Thinking.Text = "must remain frozen"
	frozen, accepted, err := registry.UpdateStatus(context.Background(), "worker", lease.LeaseID, statusRequest)
	if err != nil || !accepted {
		t.Fatalf("status after stop accepted=%v err=%v", accepted, err)
	}
	if frozen.Status == nil || frozen.Status.Thinking == nil || frozen.Status.Thinking.Text != "checking configuration" {
		t.Fatalf("frozen status = %#v", frozen.Status)
	}
	assertNoWorkEvent(t, events)

	if err := registry.Finish(context.Background(), "worker", lease.LeaseID, apitypes.ParticipantWorkOutcomeStopped); err != nil {
		t.Fatal(err)
	}
	stopped := assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStopped, 4)
	if stopped.StopState != apitypes.ParticipantWorkStopStateStopped {
		t.Fatalf("stopped update = %#v", stopped)
	}
	if err := registry.Finish(context.Background(), "worker", lease.LeaseID, apitypes.ParticipantWorkOutcomeStopped); err != nil {
		t.Fatal(err)
	}
	assertNoWorkEvent(t, events)
	if _, _, err := registry.UpdateStatus(context.Background(), "worker", lease.LeaseID, statusRequest); !errors.Is(err, ErrClosed) {
		t.Fatalf("late status error = %v, want closed", err)
	}
	if _, err := registry.RequestStop(context.Background(), "worker", apitypes.ParticipantWorkStopRequest{
		RoomID: lease.RoomID, LeaseID: lease.LeaseID, RequestID: lease.RequestID,
	}); !errors.Is(err, ErrClosed) {
		t.Fatalf("late stop error = %v, want closed", err)
	}
}

func TestRegistryReportsTurnControlFailureAndAllowsRetry(t *testing.T) {
	now := time.Date(2026, 7, 22, 13, 0, 0, 0, time.UTC)
	registry, events := newTestRegistry(t, &now)
	lease := testLease(NewID())
	if _, err := registry.StartOrRenew(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStarted, 1)
	if _, _, err := registry.UpdateStatus(context.Background(), "worker", lease.LeaseID, apitypes.ParticipantWorkStatusPatchRequest{
		Capabilities: []string{apitypes.ParticipantWorkCapabilityTurnStopV1},
		Sequence:     1,
		Phase:        apitypes.ParticipantWorkPhaseWorking,
	}); err != nil {
		t.Fatal(err)
	}
	assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStatusUpdated, 2)

	request := apitypes.ParticipantWorkStopRequest{RoomID: lease.RoomID, LeaseID: lease.LeaseID, RequestID: lease.RequestID}
	if _, err := registry.RequestStop(context.Background(), "worker", request); !errors.Is(err, ErrTurnControlFailed) {
		t.Fatalf("stop delivery error = %v", err)
	}
	assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStopRequested, 3)
	failed := assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStopFailed, 4)
	if failed.StopState != apitypes.ParticipantWorkStopStateFailed || failed.StopError == "" {
		t.Fatalf("failed stop update = %#v", failed)
	}

	delivered := false
	unregister := registry.RegisterTurnController("pt-worker", turnControllerFunc(func(context.Context, agentruntime.TurnRef) error {
		delivered = true
		return nil
	}))
	defer unregister()
	now = now.Add(time.Second)
	retried, err := registry.RequestStop(context.Background(), "worker", request)
	if err != nil {
		t.Fatal(err)
	}
	if !delivered || !retried.RequestedAt.Equal(now) {
		t.Fatalf("retry response = %#v, delivered=%v", retried, delivered)
	}
	retrying := assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStopRequested, 5)
	if retrying.StopState != apitypes.ParticipantWorkStopStateRequested || retrying.StopError != "" {
		t.Fatalf("retry stop update = %#v", retrying)
	}
	if err := registry.Stop(context.Background(), "worker", lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	assertWorkEvent(t, events, apitypes.ParticipantWorkReasonReleased, 6)
}

func TestRegistryReportsTurnControlTimeout(t *testing.T) {
	now := time.Date(2026, 7, 22, 13, 0, 0, 0, time.UTC)
	registry, events := newTestRegistry(t, &now)
	lease := testLease(NewID())
	if _, err := registry.StartOrRenew(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStarted, 1)
	if _, _, err := registry.UpdateStatus(context.Background(), "worker", lease.LeaseID, apitypes.ParticipantWorkStatusPatchRequest{
		Capabilities: []string{apitypes.ParticipantWorkCapabilityTurnStopV1},
		Sequence:     1,
		Phase:        apitypes.ParticipantWorkPhaseWorking,
	}); err != nil {
		t.Fatal(err)
	}
	assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStatusUpdated, 2)
	unregister := registry.RegisterTurnController("pt-worker", turnControllerFunc(func(context.Context, agentruntime.TurnRef) error {
		return context.DeadlineExceeded
	}))
	defer unregister()

	if _, err := registry.RequestStop(context.Background(), "worker", apitypes.ParticipantWorkStopRequest{
		RoomID: lease.RoomID, LeaseID: lease.LeaseID, RequestID: lease.RequestID,
	}); !errors.Is(err, ErrTurnControlTimedOut) {
		t.Fatalf("stop timeout error = %v", err)
	}
	assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStopRequested, 3)
	timedOut := assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStopTimedOut, 4)
	if timedOut.StopState != apitypes.ParticipantWorkStopStateTimedOut {
		t.Fatalf("timed out stop update = %#v", timedOut)
	}
	if err := registry.Finish(context.Background(), "worker", lease.LeaseID, apitypes.ParticipantWorkOutcomeStopTimedOut); err != nil {
		t.Fatal(err)
	}
	terminal := assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStopTimedOut, 5)
	if terminal.State != apitypes.ParticipantWorkStateIdle || terminal.StopState != apitypes.ParticipantWorkStopStateTimedOut {
		t.Fatalf("terminal timeout update = %#v", terminal)
	}
}

func TestRegistryRejectsInvalidStatusAndStopMetadata(t *testing.T) {
	now := time.Date(2026, 7, 20, 3, 0, 0, 0, time.UTC)
	registry, events := newTestRegistry(t, &now)
	lease := testLease(NewID())
	if _, err := registry.StartOrRenew(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	assertWorkEvent(t, events, apitypes.ParticipantWorkReasonStarted, 1)

	if _, _, err := registry.UpdateStatus(context.Background(), "worker", lease.LeaseID, apitypes.ParticipantWorkStatusPatchRequest{
		Capabilities: []string{apitypes.ParticipantWorkCapabilityThinkingStatusV1},
		Sequence:     1,
		Phase:        apitypes.ParticipantWorkPhaseThinking,
		Thinking: &apitypes.ParticipantThinkingStatus{
			Format: apitypes.ParticipantThinkingFormatPlainText,
			Text:   strings.Repeat("x", MaxThinkingBytes+1),
		},
	}); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("oversized status error = %v", err)
	}
	if _, _, err := registry.UpdateStatus(context.Background(), "worker", lease.LeaseID, apitypes.ParticipantWorkStatusPatchRequest{
		Capabilities: []string{apitypes.ParticipantWorkCapabilityThinkingStatusV1},
		Sequence:     1,
		Phase:        apitypes.ParticipantWorkPhaseThinking,
		Stage:        apitypes.ParticipantWorkStagePreparingReply,
	}); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("stage without capability error = %v", err)
	}
	if _, _, err := registry.UpdateStatus(context.Background(), "worker", lease.LeaseID, apitypes.ParticipantWorkStatusPatchRequest{
		Capabilities: []string{
			apitypes.ParticipantWorkCapabilityThinkingStatusV1,
			apitypes.ParticipantWorkCapabilityStageV1,
		},
		Sequence: 1,
		Phase:    apitypes.ParticipantWorkPhaseWorking,
		Stage:    apitypes.ParticipantWorkStagePreparingReply,
	}); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("stage with incompatible phase error = %v", err)
	}
	if _, err := registry.RequestStop(context.Background(), "worker", apitypes.ParticipantWorkStopRequest{
		RoomID: lease.RoomID, LeaseID: lease.LeaseID, RequestID: lease.RequestID,
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stop without capability error = %v", err)
	}
}

func newTestRegistry(t *testing.T, now *time.Time) (*Registry, <-chan Event) {
	t.Helper()
	imService := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "user-admin",
		Users: []im.User{
			{ID: "user-admin", Name: "Admin"},
			{ID: "user-worker", Name: "Worker"},
		},
		Rooms: []im.Room{{ID: "room-1", Members: []string{"user-admin", "u-worker"}}},
	})
	participantService := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "pt-worker",
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		ChannelUserRef:  "u-worker",
		LifecycleStatus: participant.LifecycleStatusActive,
	}}))
	bus := NewBus()
	controlBus := NewControlBus()
	events, cancel := bus.Subscribe()
	t.Cleanup(cancel)
	return NewRegistry(
		participantService,
		imService,
		bus,
		WithClock(func() time.Time { return *now }),
		WithEpoch("epoch-test"),
		WithControlBus(controlBus),
	), events
}

func testLease(id string) ParticipantWorkLease {
	return ParticipantWorkLease{
		ParticipantID: "worker",
		LeaseID:       id,
		RoomID:        "room-1",
		RequestID:     "message-1",
		Kind:          apitypes.ParticipantWorkKindAgentTurn,
	}
}

func assertWorkEvent(t *testing.T, events <-chan Event, reason string, revision uint64) apitypes.ParticipantWorkUpdate {
	t.Helper()
	select {
	case event := <-events:
		if event.Type != EventTypeParticipantWorkUpdated || event.Work.Reason != reason || event.Work.Revision != revision {
			t.Fatalf("event = %#v, want reason %q revision %d", event, reason, revision)
		}
		return event.Work
	default:
		t.Fatalf("missing work event reason %q revision %d", reason, revision)
		return apitypes.ParticipantWorkUpdate{}
	}
}

func assertNoWorkEvent(t *testing.T, events <-chan Event) {
	t.Helper()
	select {
	case event := <-events:
		t.Fatalf("unexpected event: %#v", event)
	default:
	}
}
