package openclawsandbox

import (
	"context"
	"fmt"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/sandboxgateway"
	"csgclaw/internal/sandbox"
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
	deps.WorkspaceGuestPath = workspaceGuestPathForGOOS(goruntime.GOOS)
	deps.ProjectsGuestPath = projectsGuestPathForGOOS(goruntime.GOOS)
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
	return r.Layout(agentHome).WorkspaceRoot
}

func (r *Runtime) Layout(agentHome string) agentruntime.Layout {
	workspace := workspaceRoot(agentHome)
	return agentruntime.Layout{
		WorkspaceRoot: workspace,
		SkillsRoot:    filepath.Join(workspace, "skills"),
		HostLogPaths:  []string{HostGatewayLogPath(agentHome)},
	}
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
	if _, err := EnsureConfigWithMCPServers(agentHome, participantID, req.AgentID, gateway.Server, configModelFromProfile(profile), req.MCPServers, fixedBaseURL(gateway.ManagerBaseURL), r.CurrentFeishuProvider()); err != nil {
		return err
	}
	workspaceRoot := r.Layout(agentHome).WorkspaceRoot
	if err := sandboxgateway.EnsureEmbeddedWorkspace(gateway.WorkspaceTemplate, workspaceRoot); err != nil {
		return err
	}
	if err := sandboxgateway.EnsureWorkspaceProjectsMountpoint(workspaceRoot); err != nil {
		return err
	}
	prepared, err := sandboxgateway.FinalizePreparedGatewayProvision(req, workspaceLayoutForGOOS(agentHome, goruntime.GOOS))
	if err != nil {
		return err
	}
	if err := refreshWorkspaceAgentsFile(filepath.Join(prepared.WorkspaceLayout.WorkspaceHostPath, "AGENTS.md"), req.Instructions); err != nil {
		return err
	}
	r.RememberPreparedGatewayProvision(req.AgentID, prepared)
	return nil
}

func GatewayRunCommand() string {
	return gatewayRunCommandForGOOS(goruntime.GOOS)
}

func gatewayRunCommandForGOOS(goos string) string {
	prefix := ""
	if goos == "windows" {
		prefix = "mkdir -p " + BoxDir + " && rm -rf " + BoxWorkspaceDir + " && ln -sfn " + BoxWindowsWorkspaceDir + " " + BoxWorkspaceDir + " && "
	}
	return prefix + "exec node /app/openclaw.mjs gateway --allow-unconfigured --bind lan --port 18789 1>" + BoxGatewayLogPath + " 2>&1"
}

func workspaceLayoutForGOOS(agentHome, goos string) WorkspaceLayout {
	root := Root(agentHome)
	workspace := workspaceRoot(agentHome)
	layout := WorkspaceLayout{
		MountHostPath:      root,
		MountGuestPath:     BoxDir,
		WorkspaceHostPath:  workspace,
		WorkspaceGuestPath: BoxWorkspaceDir,
	}
	if goos == "windows" {
		layout.MountHostPath = workspace
		layout.MountGuestPath = BoxWindowsWorkspaceDir
		layout.WorkspaceGuestPath = BoxWindowsWorkspaceDir
		layout.ExtraMounts = []sandbox.Mount{
			{
				HostPath:  filepath.Join(root, HostConfig),
				GuestPath: BoxConfigPath,
				ReadOnly:  true,
			},
			{
				HostPath:  filepath.Join(root, HostGatewayLog),
				GuestPath: BoxGatewayLogPath,
			},
		}
	}
	return layout
}

func workspaceGuestPathForGOOS(goos string) string {
	if goos == "windows" {
		return BoxWindowsWorkspaceDir
	}
	return BoxWorkspaceDir
}

func projectsGuestPathForGOOS(goos string) string {
	return workspaceGuestPathForGOOS(goos) + "/projects"
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
