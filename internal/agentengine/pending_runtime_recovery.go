package agentengine

import (
	"context"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/runtime"
)

func runtimeNeedsPendingRecovery(selected Agent) bool {
	if selected.Status.RuntimeKind != runtime.KindCodex || selected.Spec.Runtime.Sandboxed ||
		selected.Spec.DesiredState != AgentDesiredStateRunning ||
		!selected.Status.Model.ProfileComplete || !selected.Status.Model.EnvRestartRequired || selected.Status.Model.ImageUpgradeRequired {
		return false
	}
	switch runtime.State(selected.Status.State) {
	case runtime.StateFailed, runtime.StateStopped, runtime.StateExited, runtime.StateUnknown, runtime.State("profile_incomplete"):
		return true
	}
	return false
}

// Recovery runs before acquiring the conversation's execution lease, so the
// existing mutation gate can drain safely. Recheck intent under that gate:
// a concurrent configuration save or user stop must win over an old snapshot.
func (a runtimeBackend) recoverPendingRuntime(ctx context.Context, id string) error {
	return a.lifecycle.Mutate(ctx, agent.CanonicalID(id), func(ctx context.Context) error {
		selected, err := a.agents.Get(ctx, id, AgentGetOptions{ProbeRuntime: true})
		if err != nil || !runtimeNeedsPendingRecovery(selected) {
			return err
		}
		_, err = a.agents.Update(ctx, id, AgentUpdateRequest{
			ResourceVersion: selected.ResourceVersion,
			FieldMask:       []string{"desired_state"},
			Spec:            AgentSpec{DesiredState: AgentDesiredStateRunning},
		})
		return err
	})
}
