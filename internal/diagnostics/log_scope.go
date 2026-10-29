package diagnostics

import (
	"context"
	"sync"
)

type requestLogScopeKey struct{}
type requestLogScope struct {
	mu    sync.Mutex
	bound context.Context
}

// A scope belongs to one HTTP request. Binding its derived native identity
// lets outer access logging observe the same turn after the handler returns.
func WithRequestLogScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestLogScopeKey{}, &requestLogScope{})
}
func BindRequestLogScope(original, bound context.Context) {
	scope, _ := original.Value(requestLogScopeKey{}).(*requestLogScope)
	if scope == nil {
		return
	}
	scope.mu.Lock()
	scope.bound = bound
	scope.mu.Unlock()
}
func logRecord(ctx context.Context) *Record {
	if record := From(ctx); record != nil {
		return record
	}
	if record := Resolve(ctx); record != nil {
		return record
	}
	scope, _ := ctx.Value(requestLogScopeKey{}).(*requestLogScope)
	if scope == nil {
		return nil
	}
	scope.mu.Lock()
	bound := scope.bound
	scope.mu.Unlock()
	if bound == nil {
		return nil
	}
	if record := From(bound); record != nil {
		return record
	}
	if lookup, ok := bound.Value(nativeLookupKey{}).(nativeLookup); ok && lookup.store != nil {
		return lookup.store.native(lookup.agent, lookup.session, lookup.turn, false)
	}
	return nil
}
