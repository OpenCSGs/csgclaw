package openclawsandbox

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
}

var _ agentruntime.Provisioner = (*Runtime)(nil)
var _ agentruntime.ConversationStarter = (*Runtime)(nil)

func New(deps Dependencies) *Runtime {
	deps.RuntimeKind = agentruntime.KindOpenClawSandbox
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
			Name: "node",
			Args: []string{"-e", "const url='http://127.0.0.1:18789/readyz';const ctl=new AbortController();const t=setTimeout(()=>ctl.abort(),1500);fetch(url,{signal:ctl.signal}).then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1)).finally(()=>clearTimeout(t));"},
		}
	}
	return &Runtime{Runtime: sandboxgateway.New(deps)}
}

func (r *Runtime) WorkspaceRoot(agentHome string) string {
	return workspaceRoot(agentHome)
}

func (r *Runtime) NewConversation(_ context.Context, _ agentruntime.Handle, _ agentruntime.ConversationStartRequest) (agentruntime.ConversationStartAction, error) {
	return agentruntime.ConversationStartAction{
		Mode:         agentruntime.ConversationStartActionBotEvent,
		BotEventText: "/new",
	}, nil
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
	participantID := strings.TrimSpace(req.ParticipantID)
	if participantID == "" {
		participantID = strings.TrimSpace(req.AgentID)
	}
	if _, err := EnsureConfig(agentHome, participantID, req.AgentID, gateway.Server, configModelFromProfile(profile), fixedBaseURL(gateway.ManagerBaseURL), r.CurrentFeishuProvider()); err != nil {
		return err
	}
	workspaceRoot := r.WorkspaceRoot(agentHome)
	if err := sandboxgateway.EnsureEmbeddedWorkspace(gateway.WorkspaceTemplate, workspaceRoot); err != nil {
		return err
	}
	if err := sandboxgateway.EnsureWorkspaceProjectsMountpoint(workspaceRoot); err != nil {
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
	return "exec node /app/openclaw.mjs gateway --allow-unconfigured --bind lan --port 18789 1>" + BoxGatewayLogPath + " 2>&1"
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
