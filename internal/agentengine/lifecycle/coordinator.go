// Package lifecycle coordinates all execution and mutation of an Agent.
package lifecycle

import (
	"context"
	"errors"
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
	token    chan struct{}
	mu       sync.Mutex
	reopened chan struct{}
	active   int
	drained  chan struct{}
}
type mutation struct {
	owner   *Coordinator
	agentID string
	parent  *mutation
}
type mutationKey struct{}

var ErrMutationInProgress = errors.New("agent lifecycle change is in progress")

// TryUpdateFiles serializes an on-disk configuration update with mutations,
// without draining the execution that requested it. Never wait for a mutation:
// it may already be waiting for that same execution to finish.
func (c *Coordinator) TryUpdateFiles(ctx context.Context, id string, update func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	g := c.gate(id)
	select {
	case <-g.token:
		defer func() { g.token <- struct{}{} }()
		return update()
	default:
		return ErrMutationInProgress
	}
}

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
	return c.pin(ctx, id, false)
}

// Read pins an Agent for an administrative lookup and its dependent writes.
// It can share an active execution or nest inside an existing mutation.
func (c *Coordinator) Read(ctx context.Context, id string) (func(), error) {
	return c.pin(ctx, id, true)
}

func (c *Coordinator) pin(ctx context.Context, id string, read bool) (func(), error) {
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
	if read && c.holdsMutation(ctx, id) {
		return func() {}, nil
	}
	g := c.gate(id)
	g.mu.Lock()
	for g.reopened != nil {
		// An active conversation may itself be requesting this binding.
		// Waiting for a mutation that is draining it would block both.
		if !read || g.active > 0 {
			g.mu.Unlock()
			return nil, fmt.Errorf("agent %q lifecycle change is in progress", id)
		}
		reopened := g.reopened
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-reopened:
		}
		g.mu.Lock()
	}
	defer g.mu.Unlock()
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

func (c *Coordinator) holdsMutation(ctx context.Context, id string) bool {
	current, _ := ctx.Value(mutationKey{}).(*mutation)
	for ; current != nil; current = current.parent {
		if current.owner == c && current.agentID == id {
			return true
		}
	}
	return false
}

// Mutation returns a reentrant context and exclusive lease. Both lock waiting
// and draining respect cancellation; draining defaults to at most two minutes.
func (c *Coordinator) Mutation(ctx context.Context, id string) (context.Context, func(), error) {
	return c.MutationInterrupt(ctx, id, nil)
}

// MutationInterrupt closes admission before interrupting active work, then drains it.
func (c *Coordinator) MutationInterrupt(ctx context.Context, id string, interrupt func()) (context.Context, func(), error) {
	return c.MutationPrepare(ctx, id, func(context.Context) error {
		if interrupt != nil {
			interrupt()
		}
		return nil
	})
}

// MutationPrepare closes admission and serializes mutations before validating
// their intent. Preparation may cancel active work after validation, but must
// not change runtime resources until the returned execution drain completes.
func (c *Coordinator) MutationPrepare(ctx context.Context, id string, prepare func(context.Context) error) (context.Context, func(), error) {
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
	if c.holdsMutation(ctx, id) {
		if prepare != nil {
			if err := prepare(ctx); err != nil {
				return ctx, nil, err
			}
		}
		return ctx, func() {}, nil
	}
	g := c.gate(id)
	select {
	case <-ctx.Done():
		return ctx, nil, ctx.Err()
	case <-g.token:
	}
	g.mu.Lock()
	g.reopened = make(chan struct{})
	drained := g.drained
	g.mu.Unlock()
	release := sync.OnceFunc(func() {
		g.mu.Lock()
		close(g.reopened)
		g.reopened = nil
		g.mu.Unlock()
		g.token <- struct{}{}
	})
	parent, _ := ctx.Value(mutationKey{}).(*mutation)
	mutationCtx := context.WithValue(ctx, mutationKey{}, &mutation{owner: c, agentID: id, parent: parent})
	if prepare != nil {
		if err := prepare(mutationCtx); err != nil {
			release()
			return ctx, nil, err
		}
	}
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
	return mutationCtx, release, nil
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
