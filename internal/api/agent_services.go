package api

import (
	"context"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	hub "csgclaw/internal/template"
	"io"
)

// AgentServices supplies read-model, workspace, configuration and Runtime
// support. Agent mutations and conversations use the separately injected Engine.
type AgentServices struct {
	Records   AgentRecords
	Workspace AgentWorkspace
	Models    AgentModels
	Runtime   AgentRuntimeSupport
}
type AgentRecords interface {
	Agent(string) (agent.Agent, bool)
	AgentDisplayName(string) (string, bool)
	Reload() error
	SupportsMemory(string) bool
	AuthorizesConnectorCapability(string, string) bool
}
type AgentWorkspace interface {
	AgentLayout(string) (agentruntime.Layout, error)
	SkillsRoot(string) (string, error)
	WorkspaceRoot(string) (string, error)
	WorkspaceRootByID(string) (string, error)
	HubPublishSpec(string, bool) (hub.PublishSpec, error)
	StreamLogs(context.Context, string, bool, int, io.Writer) error
}
type AgentModels interface {
	ListModelsForRequest(context.Context, agent.ProfileModelRequest) ([]string, error)
	ProfileDefaultsView() agent.AgentProfileView
	SetLLMConfig(config.LLMConfig)
}
type AgentRuntimeSupport interface {
	WithAgentLifecycle(context.Context, string, func(context.Context) error) error
	GatewayRuntime() string
	SetGatewayRuntime(string, string) error
	SandboxProviderName() string
	SetHubService(*hub.Service)
	ResetSandboxRuntimes() error
	RuntimeOptionsSchema(string) []agentruntime.RuntimeOptionSchema
}
