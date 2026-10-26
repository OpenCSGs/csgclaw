package api

import (
	"context"
	"fmt"

	agent "csgclaw/internal/agentengine/agents"
)

// refreshAgentChannel preserves each supported Runtime's channel ownership.
// Hosted Codex workers reconcile independently; sandbox-native channels load
// their current Participant configuration through Engine-managed recreation.
func (h *Handler) refreshAgentChannel(ctx context.Context, target agent.Agent, channel string) (agent.Agent, string, error) {
	if h.channelBindings == nil {
		return target, "", fmt.Errorf("channel binding reconciler is not configured")
	}
	if err := h.channelBindings.RefreshAgentChannel(ctx, target, channel); err != nil {
		return target, "", err
	}
	return target, "channel_refreshed", nil
}
