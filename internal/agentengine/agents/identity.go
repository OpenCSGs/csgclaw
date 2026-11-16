package agents

import "context"

type identityActorKey struct{}

func withIdentityActor(ctx context.Context, actor string) context.Context {
	return context.WithValue(ctx, identityActorKey{}, actor)
}

func identityActor(ctx context.Context) string {
	actor, _ := ctx.Value(identityActorKey{}).(string)
	return actor
}
