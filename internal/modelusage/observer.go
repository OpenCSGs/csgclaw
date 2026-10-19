package modelusage

import "context"

type requestObserverKey struct{}

// WithRequestObserver scopes usage completeness to the caller's exact native
// turn. Each successful HTTP attempt reports once; true means usable final
// provider usage was received. Counts still come from the runtime adapter.
func WithRequestObserver(ctx context.Context, observer func(bool)) context.Context {
	return context.WithValue(ctx, requestObserverKey{}, observer)
}

func RequestObserver(ctx context.Context) func(bool) {
	observer, _ := ctx.Value(requestObserverKey{}).(func(bool))
	return observer
}
