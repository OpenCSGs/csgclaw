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
