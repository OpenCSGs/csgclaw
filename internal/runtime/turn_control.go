package runtime

import (
	"context"
	"errors"
	"time"
)

var (
	ErrTurnControlUnavailable = errors.New("turn control is unavailable")
	ErrTurnNotFound           = errors.New("active turn not found")
)

// TurnRef identifies one runtime turn across the participant work protocol and
// the runtime-specific execution bridge that owns the active request.
type TurnRef struct {
	RegistryEpoch string
	ParticipantID string
	RoomID        string
	LeaseID       string
	RequestID     string
	RequestedAt   time.Time
}

// TurnController is an optional runtime execution capability. Runtime
// implementations that cannot stop an individual turn do not implement it.
type TurnController interface {
	StopTurn(ctx context.Context, ref TurnRef) error
}

// TurnControllerRegistrar lets runtime bridges register participant-scoped
// controllers without adding turn control to the process-lifecycle Runtime
// interface.
type TurnControllerRegistrar interface {
	RegisterTurnController(participantID string, controller TurnController) func()
}
