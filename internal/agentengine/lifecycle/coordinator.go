// Package lifecycle coordinates all execution and mutation of an Agent.
package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Coordinator is shared by Agent administration, conversations and extensions.
// Mutations exclude other mutations, close new execution admission and drain
// active leases. A zero Coordinator is ready to use.
type Coordinator struct {
	mu           sync.Mutex
	gates        map[string]*gate
	DrainTimeout time.Duration
}
type gate struct {
	token   chan struct{}
	mu      sync.Mutex
	closed  bool
	active  int
	drained chan struct{}
}
type mutation struct {
	owner   *Coordinator
	agentID string
	parent  *mutation
}
type mutationKey struct{}

func (c *Coordinator) gate(id string) *gate {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gates == nil {
		c.gates = make(map[string]*gate)
	}
	g := c.gates[id]
	if g == nil {
		g = &gate{token: make(chan struct{}, 1), drained: make(chan struct{})}
		g.token <- struct{}{}
		close(g.drained)
		c.gates[id] = g
	}
	return g
}

// Execution pins a slot until the idempotent release is called.
// New execution fails while a mutation is draining, preventing starvation.
func (c *Coordinator) Execution(ctx context.Context, id string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	if c == nil || id == "" {
		return nil, fmt.Errorf("lifecycle coordinator and agent ID are required")
	}
	g := c.gate(id)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, fmt.Errorf("agent %q lifecycle change is in progress", id)
	}
	if g.active == 0 {
		g.drained = make(chan struct{})
	}
	g.active++
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			g.active--
			if g.active == 0 {
				close(g.drained)
			}
		})
	}, nil
}

// Mutation returns a reentrant context and exclusive lease. Both lock waiting
// and draining respect cancellation; draining defaults to at most two minutes.
func (c *Coordinator) Mutation(ctx context.Context, id string) (context.Context, func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	id = strings.TrimSpace(id)
	if c == nil || id == "" {
		return ctx, nil, fmt.Errorf("lifecycle coordinator and agent ID are required")
	}
	if err := ctx.Err(); err != nil {
		return ctx, nil, err
	}
	parent, _ := ctx.Value(mutationKey{}).(*mutation)
	for held := parent; held != nil; held = held.parent {
		if held.owner == c && held.agentID == id {
			return ctx, func() {}, nil
		}
	}
	g := c.gate(id)
	select {
	case <-ctx.Done():
		return ctx, nil, ctx.Err()
	case <-g.token:
	}
	g.mu.Lock()
	g.closed = true
	drained := g.drained
	g.mu.Unlock()
	var once sync.Once
	release := func() { once.Do(func() { g.mu.Lock(); g.closed = false; g.mu.Unlock(); g.token <- struct{}{} }) }
	timeout := c.DrainTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	drainCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case <-drainCtx.Done():
		release()
		return ctx, nil, fmt.Errorf("drain agent %q executions: %w", id, drainCtx.Err())
	case <-drained:
	}
	return context.WithValue(ctx, mutationKey{}, &mutation{owner: c, agentID: id, parent: parent}), release, nil
}
func (c *Coordinator) Mutate(ctx context.Context, id string, fn func(context.Context) error) error {
	if fn == nil {
		return fmt.Errorf("lifecycle mutation is required")
	}
	ctx, release, err := c.Mutation(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	return fn(ctx)
}
