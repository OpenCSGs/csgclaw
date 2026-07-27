package worklease

import (
	"context"
	"errors"
	"testing"
	"time"

	agentruntime "csgclaw/internal/runtime"
)

type turnControllerFunc func(context.Context, agentruntime.TurnRef) error

func (f turnControllerFunc) StopTurn(ctx context.Context, ref agentruntime.TurnRef) error {
	return f(ctx, ref)
}

func TestTurnControlDispatcherUsesLatestParticipantController(t *testing.T) {
	called := ""
	fallback := turnControllerFunc(func(context.Context, agentruntime.TurnRef) error {
		called = "fallback"
		return nil
	})
	dispatcher := NewTurnControlDispatcher(fallback)
	unregisterFirst := dispatcher.RegisterTurnController("pt-worker", turnControllerFunc(func(context.Context, agentruntime.TurnRef) error {
		called = "first"
		return nil
	}))
	unregisterSecond := dispatcher.RegisterTurnController("pt-worker", turnControllerFunc(func(context.Context, agentruntime.TurnRef) error {
		called = "second"
		return nil
	}))

	unregisterFirst()
	if err := dispatcher.StopTurn(context.Background(), agentruntime.TurnRef{ParticipantID: "pt-worker"}); err != nil {
		t.Fatal(err)
	}
	if called != "second" {
		t.Fatalf("controller = %q, want second", called)
	}

	unregisterSecond()
	if err := dispatcher.StopTurn(context.Background(), agentruntime.TurnRef{ParticipantID: "pt-worker"}); err != nil {
		t.Fatal(err)
	}
	if called != "fallback" {
		t.Fatalf("controller = %q, want fallback", called)
	}
}

func TestTurnControlDispatcherFindsOverlappingMatchingTurn(t *testing.T) {
	dispatcher := NewTurnControlDispatcher(nil)
	dispatcher.RegisterTurnController("pt-worker", turnControllerFunc(func(_ context.Context, ref agentruntime.TurnRef) error {
		if ref.LeaseID == "old-lease" {
			return nil
		}
		return agentruntime.ErrTurnNotFound
	}))
	dispatcher.RegisterTurnController("pt-worker", turnControllerFunc(func(context.Context, agentruntime.TurnRef) error {
		return agentruntime.ErrTurnNotFound
	}))
	if err := dispatcher.StopTurn(context.Background(), agentruntime.TurnRef{
		ParticipantID: "pt-worker",
		LeaseID:       "old-lease",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestControlBusStopTurnReportsDeliveryAvailability(t *testing.T) {
	bus := NewControlBus()
	requestedAt := time.Date(2026, 7, 22, 13, 0, 0, 0, time.UTC)
	ref := agentruntime.TurnRef{
		RegistryEpoch: "epoch-1",
		ParticipantID: "pt-worker",
		RoomID:        "room-1",
		LeaseID:       "lease-1",
		RequestID:     "message-1",
		RequestedAt:   requestedAt,
	}
	if err := bus.StopTurn(context.Background(), ref); !errors.Is(err, agentruntime.ErrTurnControlUnavailable) {
		t.Fatalf("stop without subscriber error = %v", err)
	}

	events, cancel := bus.Subscribe("pt-worker")
	defer cancel()
	if err := bus.StopTurn(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-events:
		if event.RegistryEpoch != ref.RegistryEpoch || event.LeaseID != ref.LeaseID || !event.RequestedAt.Equal(requestedAt) {
			t.Fatalf("control event = %#v", event)
		}
	default:
		t.Fatal("missing delivered control event")
	}
}
