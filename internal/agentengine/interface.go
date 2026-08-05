// Package agentengine defines resource-oriented contracts for managing and
// executing CSGClaw agents.
//
// The package contains contracts only and does not depend on the existing
// agent implementation or any concrete Runtime or Channel package.
package agentengine

// Interface is the complete review surface for Agent Engine.
// It follows the Kubernetes client pattern: callers first select a resource,
// then invoke operations on that resource's focused interface.
type Interface interface {
	Agents() AgentInterface
	Conversations(agentID string) ConversationInterface
}
