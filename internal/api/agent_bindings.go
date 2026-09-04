package api

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
)

// refreshAgentChannel preserves each supported Runtime's channel ownership.
// Hosted Codex workers reconcile independently; sandbox-native channels load
// their current Participant configuration through Engine-managed recreation.
func (h *Handler) refreshAgentChannel(ctx context.Context, target agent.Agent, channel string) (agent.Agent, string, error) {
	switch strings.TrimSpace(target.RuntimeKind) {
	case agent.RuntimeKindOpenClawSandbox, agent.RuntimeKindPicoClawSandbox:
		if h.agentEngine == nil {
			return target, "", fmt.Errorf("agent engine is not configured")
		}
		item, err := h.agentEngine.Agents().Recreate(ctx, target.ID, agentengine.AgentRecreateOptions{})
		if err != nil {
			return target, "", err
		}
		return serviceAgentFromEngine(item), "runtime_recreated", nil
	default:
		if h.channelBindings == nil {
			return target, "", fmt.Errorf("channel binding reconciler is not configured")
		}
		if err := h.channelBindings.RefreshAgentChannel(ctx, target, channel); err != nil {
			return target, "", err
		}
		return target, "channel_refreshed", nil
	}
}
