package llm

import (
	"context"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
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

func applySessionReasoningEffort(ctx context.Context, payload map[string]any, responses bool) {
	selection, ok := ctx.Value(sessionModelKey{}).(sessionModel)
	if !ok {
		return
	}
	if _, task := ctx.Value(taskProfileKey{}).(agent.AgentProfile); task {
		return
	}
	effort := config.NormalizeReasoningEffort(selection.effort)
	delete(payload, "reasoning_effort")
	if !responses {
		if !config.UsesModelReasoningDefault(effort) {
			payload["reasoning_effort"] = effort
		}
		return
	}
	reasoning, _ := payload["reasoning"].(map[string]any)
	if reasoning == nil {
		reasoning = make(map[string]any)
	}
	delete(reasoning, "effort")
	if !config.UsesModelReasoningDefault(effort) {
		reasoning["effort"] = effort
	}
	if len(reasoning) == 0 {
		delete(payload, "reasoning")
	} else {
		payload["reasoning"] = reasoning
	}
}
