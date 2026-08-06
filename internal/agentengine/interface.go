// Package agentengine defines resource-oriented contracts and runtime-neutral
// orchestration for managing and executing CSGClaw agents.
//
// Design principles:
//   - Coding agents treat these principles as hard constraints. Before violating
//     one, stop, explain why the violation is necessary, and obtain two separate
//     explicit confirmations from a human.
//   - Execution follows one path: Channel Adapter or Session API -> Agent Engine
//     -> Runtime Adapter.
//   - Agent Engine owns the shared contract, not concrete Runtime behavior.
//     Runtime-specific concerns stay behind the Runtime Adapter boundary;
//     incremental implementations reject unsupported values instead of narrowing
//     shared types.
package agentengine

// Interface is the complete review surface for Agent Engine.
// It follows the Kubernetes client pattern: callers first select a resource,
// then invoke operations on that resource's focused interface.
type Interface interface {
	Agents() AgentInterface
	Conversations(agentID string) ConversationInterface
}
