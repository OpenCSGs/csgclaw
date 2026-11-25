package llm

import (
	"context"
	"strings"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/config"
)

type sessionModelKey struct{}

type sessionProfileReader interface {
	ResolvedSessionProfile(string, string, string) (agent.AgentProfile, error)
}

// WithSessionModel carries the verified selection for the active native turn.
func WithSessionModel(ctx context.Context, selection contract.TurnModel) context.Context {
	return context.WithValue(ctx, sessionModelKey{}, selection)
}

func autoProfileHeaders(base map[string]string, profileID string) map[string]string {
	const header = "CSG-Router-Auto-Profile"
	headers := make(map[string]string, len(base)+1)
	for key, value := range base {
		if !strings.EqualFold(strings.TrimSpace(key), header) {
			headers[key] = value
		}
	}
	headers[header] = profileID
	return headers
}

func applySessionReasoningEffort(ctx context.Context, payload map[string]any, responses bool) {
	selection, ok := ctx.Value(sessionModelKey{}).(contract.TurnModel)
	if !ok {
		return
	}
	if _, task := ctx.Value(taskProfileKey{}).(agent.AgentProfile); task {
		return
	}
	effort := config.NormalizeReasoningEffort(selection.ReasoningEffort)
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
