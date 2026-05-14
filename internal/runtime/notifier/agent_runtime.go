package notifier

import (
	"context"
	"strings"
	"time"

	agentruntime "csgclaw/internal/runtime"
)

// AgentRuntime implements agentruntime.Runtime for in-server notification workers (no container).
type AgentRuntime struct{}

// NewAgentRuntime returns the notifier agent lifecycle runtime implementation.
func NewAgentRuntime() *AgentRuntime {
	return &AgentRuntime{}
}

func (r *AgentRuntime) Kind() string {
	return agentruntime.KindNotifier
}

// HydrateTrustPersistedStopped implements agent runtime hydrate behavior: Info always reports running.
func (r *AgentRuntime) HydrateTrustPersistedStopped() bool {
	return true
}

func (r *AgentRuntime) Create(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
	return agentruntime.Handle{
		RuntimeID: strings.TrimSpace(spec.RuntimeID),
		HandleID:  "",
	}, nil
}

func (r *AgentRuntime) Start(_ context.Context, h agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateRunning, nil
}

func (r *AgentRuntime) Stop(_ context.Context, _ agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateStopped, nil
}

func (r *AgentRuntime) Delete(_ context.Context, _ agentruntime.Handle) error {
	return nil
}

func (r *AgentRuntime) State(_ context.Context, _ agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateRunning, nil
}

func (r *AgentRuntime) Info(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
	return agentruntime.Info{
		HandleID:  strings.TrimSpace(h.HandleID),
		State:     agentruntime.StateRunning,
		CreatedAt: time.Now().UTC(),
	}, nil
}
