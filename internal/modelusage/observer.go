package modelusage

import "context"

type requestObserverKey struct{}

// WithRequestObserver scopes usage to the caller's exact native
// turn. Each successful HTTP attempt reports once; nil means no usable final
// provider usage was received.
func WithRequestObserver(ctx context.Context, observer func(*Usage)) context.Context {
	return context.WithValue(ctx, requestObserverKey{}, observer)
}

func RequestObserver(ctx context.Context) func(*Usage) {
	observer, _ := ctx.Value(requestObserverKey{}).(func(*Usage))
	return observer
}
