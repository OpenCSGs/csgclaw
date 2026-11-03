package contract

import (
	"context"
	"strings"
)

type archiveActorKey struct{}

// WithArchiveActor records the operator's display name for the archive snapshot.
// Callers derive it from their authenticated local user, never from request JSON.
func WithArchiveActor(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, archiveActorKey{}, strings.TrimSpace(name))
}

func ArchiveActorFromContext(ctx context.Context) string {
	name, _ := ctx.Value(archiveActorKey{}).(string)
	return name
}
