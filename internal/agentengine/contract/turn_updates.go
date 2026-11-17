package contract

import "context"

// TurnInputUpdates offers coalescible text updates while a turn is running.
// Latest returns an empty text when there is no update after the supplied sequence.
// A runtime may leave the updates unread if it cannot accept input mid-turn.
type TurnInputUpdates struct {
	Ready  <-chan struct{}
	Latest func(after uint64) (through uint64, text string)
}

type turnInputUpdatesKey struct{}

func WithTurnInputUpdates(ctx context.Context, updates TurnInputUpdates) context.Context {
	return context.WithValue(ctx, turnInputUpdatesKey{}, updates)
}

func TurnInputUpdatesFromContext(ctx context.Context) TurnInputUpdates {
	updates, _ := ctx.Value(turnInputUpdatesKey{}).(TurnInputUpdates)
	return updates
}
