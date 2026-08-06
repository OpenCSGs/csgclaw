package agentengine

import (
	"context"
	"time"
)

// AgentInterface is the collection-scoped API for Agent resources.
// Its implementation owns Agent persistence and Runtime lifecycle; the
// current internal/agent.Service is not part of this contract.
type AgentInterface interface {
	// Create creates an Agent from its desired configuration.
	Create(ctx context.Context, spec AgentSpec) (Agent, error)

	// Get returns one Agent by ID.
	Get(ctx context.Context, agentID string) (Agent, error)

	// List returns the Agents visible to the caller.
	List(ctx context.Context) ([]Agent, error)

	// Update replaces an Agent's complete desired configuration.
	Update(ctx context.Context, agentID string, spec AgentSpec) (Agent, error)

	// Delete removes an Agent and its Runtime-owned state.
	Delete(ctx context.Context, agentID string) error

	// Start makes an existing Agent available for execution.
	Start(ctx context.Context, agentID string) (Agent, error)

	// Stop stops execution without deleting the Agent's persisted state.
	Stop(ctx context.Context, agentID string) (Agent, error)

	// Recreate rebuilds the Runtime from the Agent's current desired state.
	// That state includes instructions, model, Runtime credentials, InitShell,
	// Runtime options, Skills, and MCP.
	Recreate(ctx context.Context, agentID string) (Agent, error)
}

// Agent is the resource for one executable agent.
// Spec is desired state with Runtime credentials omitted; Status is the latest
// observed state.
type Agent struct {
	ID        string
	Spec      AgentSpec
	Status    AgentStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AgentSpec is the complete desired configuration reconciled by Agent Engine.
// Updating it atomically replaces Skills and MCP configuration together
// with the rest of the desired state.
type AgentSpec struct {
	Name         string
	Description  string
	Instructions string
	Role         AgentRole
	Runtime      RuntimeSpec
	Model        ModelSpec
	Skills       []string
	MCPServers   map[string]MCPServerConfig
}

// AgentRole selects orchestration behavior. Agent is the resource kind, so a
// generic "agent" role would not add information.
type AgentRole string

const (
	AgentRoleWorker  AgentRole = "worker"
	AgentRoleManager AgentRole = "manager"
)

// RuntimeSpec selects a registered Runtime Adapter and its desired execution
// environment. Credential names and Options remain adapter-specific.
type RuntimeSpec struct {
	Adapter   string
	Sandboxed bool
	Image     string

	// Credentials contains adapter-specific secrets to materialize in the
	// Runtime environment. It is write-only on Create and Update: returned Agent
	// values omit it, and its values must not be logged.
	Credentials map[string]string

	// InitShell is an idempotent shell program run in the Runtime environment
	// after credentials are materialized and before the Runtime starts.
	InitShell string

	Options map[string]any
}

// ModelSpec selects model behavior without embedding provider credentials.
type ModelSpec struct {
	ProviderID      string
	ModelID         string
	ReasoningEffort string
	FastMode        bool
	Options         map[string]any
}

// MCPServerConfig is one MCP server's schema-neutral desired configuration.
// Values may contain secrets and must not be logged.
type MCPServerConfig map[string]any

// AgentStatus is observed lifecycle state and is never desired configuration.
type AgentStatus struct {
	State     AgentState
	RuntimeID string
	Ready     bool
	Message   string
}

// AgentState is the observed lifecycle state.
type AgentState string

const (
	AgentStateCreating   AgentState = "creating"
	AgentStateStarting   AgentState = "starting"
	AgentStateRunning    AgentState = "running"
	AgentStateStopping   AgentState = "stopping"
	AgentStateStopped    AgentState = "stopped"
	AgentStateRecreating AgentState = "recreating"
	AgentStateFailed     AgentState = "failed"
)
