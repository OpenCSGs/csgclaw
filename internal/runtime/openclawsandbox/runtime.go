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
	deps.HomeEnv = BoxUserHome
	deps.WorkspaceGuestPath = BoxDir
	deps.ProjectsGuestPath = BoxProjectsDir
	deps.GatewayLogPath = BoxGatewayLogPath
	if deps.WorkspaceTemplate == nil {
		deps.WorkspaceTemplate = func(_, _ string) string { return WorkspaceTemplateWorker }
	}
	if deps.GatewayCommand == nil {
		deps.GatewayCommand = GatewayRunCommand
	}
	return sandboxgateway.New(deps)
}

func GatewayRunCommand() string {
	return "node /app/openclaw.mjs gateway stop 2>/dev/null; sleep 2; exec node /app/openclaw.mjs gateway --allow-unconfigured --bind lan --port 18789 1>>" + BoxGatewayLogPath + " 2>&1"
}
