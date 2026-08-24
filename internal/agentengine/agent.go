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
	ID        string      `json:"id"`
	Spec      AgentSpec   `json:"spec"`
	Status    AgentStatus `json:"status"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// AgentSpec is the complete desired configuration reconciled by Agent Engine.
// Updating it atomically replaces Skills and MCP configuration together
// with the rest of the desired state.
type AgentSpec struct {
	Name         string                     `json:"name"`
	Description  string                     `json:"description,omitempty"`
	Instructions string                     `json:"instructions,omitempty"`
	Role         AgentRole                  `json:"role"`
	Runtime      RuntimeSpec                `json:"runtime"`
	Model        ModelSpec                  `json:"model"`
	Skills       []string                   `json:"skills,omitempty"`
	MCPServers   map[string]MCPServerConfig `json:"mcp_servers,omitempty"`
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
	Adapter   string `json:"adapter"`
	Sandboxed bool   `json:"sandboxed,omitempty"`
	Image     string `json:"image,omitempty"`

	// Credentials maps Runtime-workspace-relative file paths to complete secret
	// file contents. It is write-only on Create and Update: returned Agent values
	// omit it, and its values must not be logged.
	Credentials map[string]string `json:"credentials,omitempty"`

	// InitShell is an idempotent shell program run with the Runtime workspace as
	// its working directory after credentials are materialized.
	InitShell string `json:"init_shell,omitempty"`

	// Options must contain only JSON-compatible values so the same Spec can cross
	// an HTTP transport without Runtime-specific Go types.
	Options map[string]any `json:"options,omitempty"`
}

// ModelSpec selects model behavior without embedding provider credentials.
type ModelSpec struct {
	ProviderID      string `json:"provider_id,omitempty"`
	ModelID         string `json:"model_id,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	FastMode        bool   `json:"fast_mode,omitempty"`
	// Options must contain only JSON-compatible values.
	Options map[string]any `json:"options,omitempty"`
}

// MCPServerConfig is one MCP server's schema-neutral desired configuration.
// Values must be JSON-compatible, may contain secrets, and must not be logged.
type MCPServerConfig map[string]any

// AgentStatus is observed lifecycle state and is never desired configuration.
type AgentStatus struct {
	State     AgentState `json:"state"`
	RuntimeID string     `json:"runtime_id,omitempty"`
	Ready     bool       `json:"ready"`
	Message   string     `json:"message,omitempty"`
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
