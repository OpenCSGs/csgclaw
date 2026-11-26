package keylock

import (
	"context"
	"sync"
)

// Locks serializes work for one key while allowing unrelated keys to proceed.
type Locks struct {
	mu      sync.Mutex
	pending map[string]chan struct{}
}

// Acquire waits for the current owner or context cancellation.
func (p *Locks) Acquire(ctx context.Context, key string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		p.mu.Lock()
		pending := p.pending[key]
		if pending == nil {
			if p.pending == nil {
				p.pending = make(map[string]chan struct{})
			}
			pending = make(chan struct{})
			p.pending[key] = pending
			p.mu.Unlock()
			return func() {
				p.mu.Lock()
				delete(p.pending, key)
				close(pending)
				p.mu.Unlock()
			}, nil
		}
		p.mu.Unlock()
		select {
		case <-pending:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
