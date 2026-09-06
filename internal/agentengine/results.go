package agentengine

import (
	"context"
	"errors"
)

func emitTurnEvent(ctx context.Context, sink EventSink, event TurnEvent) error {
	if sink == nil {
		return nil
	}
	return sink.Emit(ctx, event)
}

func resultFromContext(ctx context.Context, err error) TurnResult {
	if err == nil {
		err = ctx.Err()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return TurnResult{Status: TurnCanceled, Error: &TurnError{Code: ErrorCanceled, Message: err.Error()}}
	}
	return failedResult(ErrorRuntimeFailed, err.Error())
}
