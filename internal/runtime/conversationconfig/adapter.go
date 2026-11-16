package conversationconfig

import (
	"context"
	"sync"

	"csgclaw/internal/agentengine/contract"
)

// Coordinator prevents a process configuration change during another turn.
// It is scoped to an Agent runtime, not a room or native session.
type Coordinator struct {
	once sync.Once
	gate chan struct{}
}

func (c *Coordinator) acquire(ctx context.Context) error {
	c.once.Do(func() { c.gate = make(chan struct{}, 1) })
	select {
	case c.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-c.gate
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type Adapter struct {
	contract.RuntimeConversation
	Coordinator *Coordinator
	Prepare     func(context.Context, contract.TurnRequest) error
}

func (a *Adapter) Run(ctx context.Context, req contract.TurnRequest, sink contract.EventSink) contract.TurnResult {
	if err := a.Coordinator.acquire(ctx); err != nil {
		return failed(ctx, err)
	}
	defer func() { <-a.Coordinator.gate }()
	if err := a.Prepare(ctx, req); err != nil {
		return failed(ctx, err)
	}
	return a.RuntimeConversation.Run(ctx, req, sink)
}

func (a *Adapter) Reset(ctx context.Context, key contract.ConversationKey) *contract.TurnError {
	if err := a.Coordinator.acquire(ctx); err != nil {
		return failed(ctx, err).Error
	}
	defer func() { <-a.Coordinator.gate }()
	return a.RuntimeConversation.Reset(ctx, key)
}

func failed(ctx context.Context, err error) contract.TurnResult {
	status, code := contract.TurnFailed, contract.ErrorRuntimeFailed
	if ctx.Err() != nil {
		status, code = contract.TurnCanceled, contract.ErrorCanceled
	}
	return contract.TurnResult{Status: status, Error: &contract.TurnError{Code: code, Message: err.Error()}}
}

// Compact shares the process configuration gate and never waits behind work.
func (a *Adapter) Compact(ctx context.Context, req contract.CompactRequest) (contract.CompactResult, error) {
	a.Coordinator.once.Do(func() { a.Coordinator.gate = make(chan struct{}, 1) })
	select {
	case a.Coordinator.gate <- struct{}{}:
	default:
		return contract.CompactResult{Status: "busy"}, nil
	}
	defer func() { <-a.Coordinator.gate }()
	if a.Prepare != nil {
		if err := a.Prepare(ctx, contract.TurnRequest{ConversationKey: req.ConversationKey, RoomConfiguration: req.RoomConfiguration}); err != nil {
			return contract.CompactResult{}, err
		}
	}
	c, ok := a.RuntimeConversation.(contract.ConversationCompactor)
	if !ok {
		return contract.CompactResult{Status: "unsupported"}, nil
	}
	return c.Compact(ctx, req)
}
