package picoclawsandbox

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/sandboxgateway"
)

type AgentRef = sandboxgateway.AgentRef
type Dependencies = sandboxgateway.Dependencies
type WorkspaceLayout = sandboxgateway.WorkspaceLayout

type Runtime struct {
	*sandboxgateway.Runtime
	deps Dependencies
}

var _ agentruntime.Provisioner = (*Runtime)(nil)

func New(deps Dependencies) *Runtime {
	deps.RuntimeKind = agentruntime.KindPicoClawSandbox
	deps.HomeEnv = BoxUserHome
	deps.MountGuestPath = BoxDir
	deps.WorkspaceGuestPath = BoxWorkspaceDir
	deps.ProjectsGuestPath = BoxProjectsDir
	deps.GatewayLogPath = BoxGatewayLogPath
	if deps.GatewayCommand == nil {
		deps.GatewayCommand = GatewayRunCommand
	}
	if strings.TrimSpace(deps.ReadinessProbe.Name) == "" {
		deps.ReadinessProbe = sandboxgateway.GatewayReadinessProbe{
			Name: "curl",
			Args: []string{"-sf", "-o", "/dev/null", "--max-time", "2", "http://127.0.0.1:18790/health"},
		}
	}
	return &Runtime{
		Runtime: sandboxgateway.New(deps),
		deps:    deps,
	}
}

func (r *Runtime) Provision(_ context.Context, req agentruntime.ProvisionRequest) error {
	if r == nil {
		return nil
	}
	gateway := req.Gateway
	if gateway == nil {
		return fmt.Errorf("gateway provisioning data is required")
	}
	profile := req.Profile.Normalized()
	if strings.TrimSpace(profile.ModelID) == "" {
		profile.ModelID = strings.TrimSpace(gateway.ModelFallback)
	}
	agentHome := strings.TrimSpace(gateway.AgentHome)
	if agentHome == "" {
		return fmt.Errorf("gateway agent home is required")
	}
	if _, err := EnsureConfig(agentHome, req.AgentID, gateway.Server, configModelFromProfile(profile), fixedBaseURL(gateway.ManagerBaseURL)); err != nil {
		return err
	}
	workspaceRoot := WorkspaceRoot(agentHome)
	if err := sandboxgateway.EnsureEmbeddedWorkspace(gateway.WorkspaceTemplate, workspaceRoot); err != nil {
		return err
	}
	prepared, err := sandboxgateway.FinalizePreparedGatewayProvision(req, WorkspaceLayout{
		MountHostPath:      Root(agentHome),
		MountGuestPath:     BoxDir,
		WorkspaceHostPath:  workspaceRoot,
		WorkspaceGuestPath: BoxWorkspaceDir,
	})
	if err != nil {
		return err
	}
	r.RememberPreparedGatewayProvision(req.AgentID, prepared)
	return nil
}

func GatewayRunCommand() string {
	return "exec /usr/local/bin/picoclaw gateway -d 1>" + BoxGatewayLogPath + " 2>&1"
}

func fixedBaseURL(baseURL string) BaseURLResolver {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return func(config.ServerConfig) string {
		return baseURL
	}
}

func configModelFromProfile(profile agentruntime.Profile) config.ModelConfig {
	return config.ModelConfig{
		Provider:        profile.Provider,
		BaseURL:         profile.BaseURL,
		APIKey:          profile.APIKey,
		ModelID:         profile.ModelID,
		ReasoningEffort: profile.ReasoningEffort,
	}
}
