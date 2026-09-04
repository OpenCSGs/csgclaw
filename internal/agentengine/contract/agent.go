package contract

import (
	"context"
	"time"
)

// AgentInterface is the collection-scoped API for Agent resources.
// Its implementation owns Agent persistence and Runtime lifecycle; the
// current internal/agent.Service is not part of this contract.
type AgentInterface interface {
	Create(ctx context.Context, request AgentCreateRequest) (Agent, error)
	Get(ctx context.Context, agentID string, options AgentGetOptions) (Agent, error)
	List(ctx context.Context, options AgentListOptions) ([]Agent, error)
	Update(ctx context.Context, agentID string, request AgentUpdateRequest) (Agent, error)

	// Delete removes an Agent and its Runtime-owned state.
	Delete(ctx context.Context, agentID string) error

	// Recreate rebuilds the Runtime from the Agent's current desired state.
	Recreate(ctx context.Context, agentID string, options AgentRecreateOptions) (Agent, error)
}

// AgentCreateRequest creates one desired Agent resource. Existing-resource
// replacement is expressed through Update with a field mask.
type AgentCreateRequest struct {
	ID           string    `json:"id,omitempty"`
	Spec         AgentSpec `json:"spec"`
	FromTemplate string    `json:"from_template,omitempty"`
}

type AgentGetOptions struct {
	Reload           bool `json:"reload,omitempty"`
	ProbeRuntime     bool `json:"probe_runtime,omitempty"`
	IncludeDocuments bool `json:"include_documents,omitempty"`
	// AdoptMCPServers reads unmanaged Runtime MCP state for an explicit MCP
	// administration operation. Runtime read failures are returned to the caller.
	AdoptMCPServers bool `json:"adopt_mcp_servers,omitempty"`
}

type AgentListOptions struct {
	Reload       bool `json:"reload,omitempty"`
	ProbeRuntime bool `json:"probe_runtime,omitempty"`
}

type AgentUpdateRequest struct {
	Spec            AgentSpec `json:"spec"`
	FieldMask       []string  `json:"field_mask,omitempty"`
	ResourceVersion string    `json:"resource_version,omitempty"`
}

type AgentRecreateOptions struct {
	UpgradeImage bool                `json:"upgrade_image,omitempty"`
	Update       *AgentUpdateRequest `json:"update,omitempty"`
}

// Agent is the resource for one executable agent.
// Spec is desired state with Runtime credentials omitted; Status is the latest
// observed state.
type Agent struct {
	ID              string      `json:"id"`
	ResourceVersion string      `json:"resource_version"`
	Spec            AgentSpec   `json:"spec"`
	Status          AgentStatus `json:"status"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
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
	Memory       *MemorySpec                `json:"memory,omitempty"`
	DesiredState AgentDesiredState          `json:"desired_state,omitempty"`
}

type MemorySpec struct {
	Enabled bool `json:"enabled"`
}

type AgentDesiredState string

const (
	AgentDesiredStateRunning AgentDesiredState = "running"
	AgentDesiredStateStopped AgentDesiredState = "stopped"
)

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
	Selector        string            `json:"selector,omitempty"`
	Name            string            `json:"name,omitempty"`
	Description     string            `json:"description,omitempty"`
	Provider        string            `json:"provider,omitempty"`
	ProviderID      string            `json:"provider_id,omitempty"`
	BaseURL         string            `json:"base_url,omitempty"`
	APIKey          string            `json:"api_key,omitempty"`
	ModelID         string            `json:"model_id,omitempty"`
	ReasoningEffort string            `json:"reasoning_effort,omitempty"`
	FastMode        bool              `json:"fast_mode,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	// Options must contain only JSON-compatible values.
	Options map[string]any `json:"options,omitempty"`
}

type ModelView struct {
	ModelSpec
	APIKeySet            bool                     `json:"api_key_set,omitempty"`
	APIKeyPreview        string                   `json:"api_key_preview,omitempty"`
	ProfileComplete      bool                     `json:"profile_complete,omitempty"`
	EnvRestartRequired   bool                     `json:"env_restart_required,omitempty"`
	ImageUpgradeRequired bool                     `json:"image_upgrade_required,omitempty"`
	DetectionResults     []ProfileDetectionResult `json:"detection_results,omitempty"`
}

type ProfileDetectionResult struct {
	Provider string `json:"provider"`
	Status   string `json:"status"`
	ModelID  string `json:"model_id,omitempty"`
	Error    string `json:"error,omitempty"`
}

// MCPServerConfig is one MCP server's schema-neutral desired configuration.
// Values must be JSON-compatible, may contain secrets, and must not be logged.
type MCPServerConfig map[string]any

// AgentStatus is observed lifecycle state and is never desired configuration.
type AgentStatus struct {
	State          AgentState           `json:"state"`
	RuntimeID      string               `json:"runtime_id,omitempty"`
	RuntimeKind    string               `json:"runtime_kind,omitempty"`
	SandboxID      string               `json:"sandbox_id,omitempty"`
	Ready          bool                 `json:"ready"`
	Message        string               `json:"message,omitempty"`
	Availability   *RuntimeAvailability `json:"availability,omitempty"`
	StartupPending bool                 `json:"startup_pending,omitempty"`
	Model          ModelView            `json:"model,omitempty"`
	Capabilities   AgentCapabilities    `json:"capabilities,omitempty"`
	Instructions   *InstructionsStatus  `json:"instructions,omitempty"`
	Memory         *MemoryStatus        `json:"memory,omitempty"`
}

type AgentCapabilities struct {
	Memory bool `json:"memory,omitempty"`
}

type InstructionsStatus struct {
	Effective string `json:"effective,omitempty"`
	Error     string `json:"error,omitempty"`
}

type MemoryStatus struct {
	Enabled  bool   `json:"enabled"`
	Ready    bool   `json:"ready"`
	Name     string `json:"name,omitempty"`
	Location string `json:"location,omitempty"`
	Content  string `json:"content,omitempty"`
	Error    string `json:"error,omitempty"`
}

type RuntimeAvailability struct {
	State     string    `json:"state"`
	CheckedAt time.Time `json:"checked_at,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Reason    string    `json:"reason,omitempty"`
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
