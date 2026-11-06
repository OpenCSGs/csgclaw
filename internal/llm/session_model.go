package llm

import (
	"context"

	agent "csgclaw/internal/agentengine/agents"
)

type sessionModelKey struct{}

type sessionModel struct {
	model  string
	effort string
}

type sessionProfileReader interface {
	ResolvedSessionProfile(string, string, string) (agent.AgentProfile, error)
}

// WithSessionModel carries the selection verified for the active native turn.
func WithSessionModel(ctx context.Context, model, effort string) context.Context {
	return context.WithValue(ctx, sessionModelKey{}, sessionModel{model: model, effort: effort})
}
