package codexsandbox

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
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
var _ agentruntime.RuntimeConfigController = (*Runtime)(nil)

func New(deps Dependencies) *Runtime {
	deps.RuntimeKind = agentruntime.KindCodexSandbox
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
			Name: "wget",
			Args: []string{"-q", "--spider", "-T", "2", "http://127.0.0.1:18791/readyz"},
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
	if _, err := EnsureConfig(agentHome, participantID, req.AgentID, gateway.Server, configModelFromProfile(profile), fixedBaseURL(gateway.ManagerBaseURL), r.CurrentFeishuProvider()); err != nil {
		return err
	}
	workspaceRoot := r.Layout(agentHome).WorkspaceRoot
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

func (r *Runtime) ValidateConfig(context.Context, agentruntime.RuntimeConfigSnapshot) error {
	return nil
}

func (r *Runtime) RestartRequired(change agentruntime.RuntimeConfigChange) (bool, error) {
	return !runtimeProfileConfigEqual(change.Previous.Profile, change.Current.Profile) ||
		!reflect.DeepEqual(change.Previous.Options, change.Current.Options), nil
}

func (r *Runtime) ReconcileConfig(context.Context, agentruntime.Handle, agentruntime.RuntimeConfigChange) error {
	return nil
}

func GatewayRunCommand() string {
	return "exec /usr/local/bin/csgclaw-codex-gateway 1>" + BoxGatewayLogPath + " 2>&1"
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

func runtimeProfileConfigEqual(a, b agentruntime.RuntimeProfileConfig) bool {
	return strings.TrimSpace(a.Provider) == strings.TrimSpace(b.Provider) &&
		strings.TrimRight(strings.TrimSpace(a.BaseURL), "/") == strings.TrimRight(strings.TrimSpace(b.BaseURL), "/") &&
		strings.TrimSpace(a.APIKey) == strings.TrimSpace(b.APIKey) &&
		strings.TrimSpace(a.ModelID) == strings.TrimSpace(b.ModelID) &&
		strings.TrimSpace(a.ReasoningEffort) == strings.TrimSpace(b.ReasoningEffort) &&
		reflect.DeepEqual(a.Headers, b.Headers) &&
		reflect.DeepEqual(a.RequestOptions, b.RequestOptions)
}
