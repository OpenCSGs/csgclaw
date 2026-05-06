package openclawsandbox

import (
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/sandboxgateway"
)

type AgentRef = sandboxgateway.AgentRef
type Dependencies = sandboxgateway.Dependencies
type Runtime = sandboxgateway.Runtime

func New(deps Dependencies) *Runtime {
	deps.RuntimeKind = agentruntime.KindOpenClawSandbox
	return sandboxgateway.New(deps)
}
