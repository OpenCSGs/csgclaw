package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	csgclawchannel "csgclaw/internal/channel/csgclaw"
	"csgclaw/internal/channel/csgclaw/notification"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	"csgclaw/internal/hub"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
	"csgclaw/internal/participant"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandboxproviders"
	"csgclaw/internal/team"
	"csgclaw/internal/upgrade"
	"csgclaw/internal/utils"
	"csgclaw/internal/version"
)

type Handler struct {
	svc                        *agent.Service
	participant                *participant.Service
	im                         *im.Service
	csgclaw                    *csgclawchannel.Service
	imBus                      *im.Bus
	imProvisioner              *im.Provisioner
	participantBridge          *im.ParticipantBridge
	feishu                     *feishu.Service
	llm                        *llm.Service
	hub                        *hub.Service
	teamSvc                    *team.Service
	teamAdapter                team.TeamChannelAdapter
	configPath                 string
	serverAccessToken          string
	serverNoAuth               bool
	upgradeManager             *upgrade.Manager
	upgradeConfigPath          string
	upgradeApply               func(upgrade.ApplyHelperOptions) error
	serverRestartApply         func(upgrade.RestartHelperOptions) error
	localRuntimeImages         func(context.Context, config.Config) ([]string, error)
	notificationDeliver        notification.Fanouter
	activityDecider            ActivityDecider
	localDirectoryPicker       func(context.Context) (string, error)
	feishuRegistrationStateDir string

	participantActivityTurnsMu sync.Mutex
	participantActivityTurns   map[string]participantActivityTurn
}

const createOperationTimeout = 10 * time.Minute

var sseHeartbeatInterval = 15 * time.Second

func detachedCreateContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), createOperationTimeout)
}

type imBootstrapResponse struct {
	CurrentUserID      string    `json:"current_user_id"`
	Users              []im.User `json:"users"`
	Rooms              []im.Room `json:"rooms"`
	InviteDraftUserIDs []string  `json:"invite_draft_user_ids,omitempty"`
}

type imEventResponse struct {
	Type    string                  `json:"type"`
	RoomID  string                  `json:"room_id,omitempty"`
	Room    *im.Room                `json:"room,omitempty"`
	User    *im.User                `json:"user,omitempty"`
	Message *im.Message             `json:"message,omitempty"`
	Thread  *im.ThreadView          `json:"thread,omitempty"`
	Sender  *im.User                `json:"sender,omitempty"`
	Upgrade *apitypes.UpgradeStatus `json:"upgrade,omitempty"`
}

type bootstrapConfigResponse struct {
	DefaultManagerTemplate string                                        `json:"default_manager_template"`
	DefaultWorkerTemplate  string                                        `json:"default_worker_template"`
	RuntimeKind            string                                        `json:"runtime_kind"`
	ShowUpgrade            bool                                          `json:"show_upgrade"`
	EffectiveManagerImage  string                                        `json:"effective_manager_image"`
	AdvertiseBaseURL       string                                        `json:"advertise_base_url,omitempty"`
	SupportedRuntimeKinds  []string                                      `json:"supported_runtime_kinds"`
	RuntimeDefaultImages   map[string]string                             `json:"runtime_default_images,omitempty"`
	RuntimeOptionSchemas   map[string][]agentruntime.RuntimeOptionSchema `json:"runtime_option_schemas,omitempty"`
}

type updateBootstrapConfigRequest struct {
	DefaultManagerTemplate *string `json:"default_manager_template,omitempty"`
	DefaultWorkerTemplate  *string `json:"default_worker_template,omitempty"`
}

type agentResponse struct {
	ID                   string                             `json:"id"`
	Name                 string                             `json:"name"`
	Description          string                             `json:"description,omitempty"`
	Instructions         string                             `json:"instructions,omitempty"`
	RuntimeID            string                             `json:"runtime_id,omitempty"`
	RuntimeKind          string                             `json:"runtime_kind,omitempty"`
	Image                string                             `json:"image,omitempty"`
	Avatar               string                             `json:"avatar,omitempty"`
	BoxID                string                             `json:"box_id,omitempty"`
	Role                 string                             `json:"role"`
	Status               string                             `json:"status"`
	CreatedAt            time.Time                          `json:"created_at"`
	Profile              string                             `json:"profile,omitempty"`
	RuntimeOptions       map[string]any                     `json:"runtime_options"`
	RuntimeOptionSchemas []agentruntime.RuntimeOptionSchema `json:"runtime_option_schemas,omitempty"`
	AgentProfile         agent.AgentProfileView             `json:"agent_profile,omitempty"`
	ProfileComplete      bool                               `json:"profile_complete"`
	DetectionResults     []agent.ProfileDetectionResult     `json:"detection_results,omitempty"`
	Participants         []apitypes.Participant             `json:"participants,omitempty"`
}

type directoryPickerResponse struct {
	Path string `json:"path"`
}

func (h *Handler) handleBootstrapConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, path, err := h.loadBootstrapConfig()
		_ = path
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, h.bootstrapConfigView(r.Context(), cfg))
	case http.MethodPut:
		var req updateBootstrapConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		cfg, path, err := h.loadBootstrapConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if req.DefaultManagerTemplate != nil {
			cfg.Bootstrap.DefaultManagerTemplate = *req.DefaultManagerTemplate
		}
		if req.DefaultWorkerTemplate != nil {
			cfg.Bootstrap.DefaultWorkerTemplate = *req.DefaultWorkerTemplate
		}
		if err := cfg.Bootstrap.Validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := cfg.Save(path); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if h.svc != nil {
			if req.DefaultManagerTemplate != nil || req.DefaultWorkerTemplate != nil {
				defaults, err := hub.ResolveBootstrapDefaults(r.Context(), cfg.Bootstrap, h.hub)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				if err := h.svc.SetGatewayRuntime(defaults.ManagerRuntimeKind, defaults.ManagerImage); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}
		}
		writeJSON(w, http.StatusOK, h.bootstrapConfigView(r.Context(), cfg))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) listAgentImageCandidates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, _, err := h.loadBootstrapConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	lister := h.localRuntimeImages
	if lister == nil {
		lister = listLocalRuntimeImages
	}
	images, err := lister(r.Context(), cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if images == nil {
		images = []string{}
	}
	writeJSON(w, http.StatusOK, images)
}

func (h *Handler) handleLocalDirectoryPicker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	picker := h.localDirectoryPicker
	if picker == nil {
		picker = selectLocalDirectory
	}
	path, err := picker(r.Context())
	if err != nil {
		switch {
		case errors.Is(err, errDirectorySelectionCanceled):
			w.WriteHeader(http.StatusNoContent)
		case errors.Is(err, errDirectoryPickerUnsupported):
			http.Error(w, err.Error(), http.StatusNotImplemented)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusOK, directoryPickerResponse{Path: path})
}

func listLocalRuntimeImages(ctx context.Context, cfg config.Config) ([]string, error) {
	provider, err := sandboxproviders.Provider(cfg.Sandbox)
	if err != nil {
		return nil, err
	}
	return listLocalRuntimeImagesWithProvider(ctx, provider)
}

func listLocalRuntimeImagesWithProvider(ctx context.Context, provider sandbox.Provider) ([]string, error) {
	if provider == nil {
		return []string{}, nil
	}
	homeDir, err := agent.SandboxRuntimeHome(agent.ManagerName)
	if err != nil {
		return nil, err
	}
	return provider.ListImages(ctx, homeDir)
}

func (h *Handler) loadBootstrapConfig() (config.Config, string, error) {
	path := strings.TrimSpace(h.configPath)
	if path == "" {
		defaultPath, err := config.DefaultPath()
		if err != nil {
			return config.Config{}, "", err
		}
		path = defaultPath
	}
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return config.Config{}, "", err
		}
		cfg := config.Config{
			Server: config.ServerConfig{
				ListenAddr:  config.DefaultListenAddr(),
				AccessToken: config.DefaultAccessToken,
				NoAuth:      false,
				ShowUpgrade: true,
			},
			Bootstrap: config.BootstrapConfig{},
			Sandbox: config.SandboxConfig{
				Provider: config.DefaultSandboxProvider,
			},
		}
		return cfg, path, nil
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, "", err
	}
	return cfg, path, nil
}

func bootstrapConfigView(ctx context.Context, cfg config.Config, hubSvc *hub.Service, runtimeOptionSchemas map[string][]agentruntime.RuntimeOptionSchema) bootstrapConfigResponse {
	resp := bootstrapConfigResponse{
		DefaultManagerTemplate: cfg.Bootstrap.ResolvedDefaultManagerTemplate(),
		DefaultWorkerTemplate:  cfg.Bootstrap.ResolvedDefaultWorkerTemplate(),
		ShowUpgrade:            cfg.Server.ShowUpgrade,
		AdvertiseBaseURL:       config.ResolveAdvertiseBaseURL(cfg.Server),
		SupportedRuntimeKinds: []string{
			agent.RuntimeKindPicoClawSandbox,
			agent.RuntimeKindOpenClawSandbox,
			agent.RuntimeKindCodex,
		},
		RuntimeDefaultImages: map[string]string{},
		RuntimeOptionSchemas: runtimeOptionSchemas,
	}
	defaults, err := hub.ResolveBootstrapDefaults(ctx, cfg.Bootstrap, hubSvc)
	if err != nil {
		resp.RuntimeKind = bootstrapRuntimeKind("")
		return resp
	}
	resp.RuntimeKind = bootstrapRuntimeKind(defaults.ManagerRuntimeKind)
	resp.EffectiveManagerImage = defaults.ManagerImage
	if defaults.ManagerRuntimeKind != "" && defaults.ManagerImage != "" {
		resp.RuntimeDefaultImages[defaults.ManagerRuntimeKind] = defaults.ManagerImage
	}
	if defaults.WorkerRuntimeKind != "" && defaults.WorkerImage != "" {
		resp.RuntimeDefaultImages[defaults.WorkerRuntimeKind] = defaults.WorkerImage
	}
	return resp
}

func (h *Handler) bootstrapConfigView(ctx context.Context, cfg config.Config) bootstrapConfigResponse {
	var schemas map[string][]agentruntime.RuntimeOptionSchema
	if h != nil {
		schemas = h.runtimeOptionSchemasByKind([]string{
			agent.RuntimeKindPicoClawSandbox,
			agent.RuntimeKindOpenClawSandbox,
			agent.RuntimeKindCodex,
		})
	}
	return bootstrapConfigView(ctx, cfg, h.hub, schemas)
}

func bootstrapRuntimeKind(runtime string) string {
	switch strings.TrimSpace(strings.ToLower(runtime)) {
	case agent.RuntimeKindOpenClawSandbox:
		return agent.RuntimeKindOpenClawSandbox
	default:
		return agent.RuntimeKindPicoClawSandbox
	}
}

type createMessageRequest struct {
	RoomID    string              `json:"room_id"`
	SenderID  string              `json:"sender_id"`
	Content   string              `json:"content"`
	MentionID string              `json:"mention_id,omitempty"`
	RelatesTo *im.MessageRelation `json:"relates_to,omitempty"`
}

type addRoomMembersRequest struct {
	RoomID    string   `json:"room_id"`
	InviterID string   `json:"inviter_id"`
	UserIDs   []string `json:"user_ids"`
	Locale    string   `json:"locale"`
}

func NewHandler(svc *agent.Service, imSvc *im.Service, imBus *im.Bus, participantBridge *im.ParticipantBridge, feishu *feishu.Service, llmSvc *llm.Service) *Handler {
	return NewHandlerWithAccessToken(svc, imSvc, imBus, participantBridge, feishu, llmSvc, "")
}

func NewHandlerWithAccessToken(svc *agent.Service, imSvc *im.Service, imBus *im.Bus, participantBridge *im.ParticipantBridge, feishu *feishu.Service, llmSvc *llm.Service, serverAccessToken string) *Handler {
	return NewHandlerWithAuth(svc, imSvc, imBus, participantBridge, feishu, llmSvc, serverAccessToken, false)
}

func NewHandlerWithAuth(svc *agent.Service, imSvc *im.Service, imBus *im.Bus, participantBridge *im.ParticipantBridge, feishu *feishu.Service, llmSvc *llm.Service, serverAccessToken string, serverNoAuth bool) *Handler {
	h := &Handler{
		svc:               svc,
		im:                imSvc,
		csgclaw:           csgclawchannel.NewService(imSvc),
		imBus:             imBus,
		imProvisioner:     im.NewProvisioner(imSvc, imBus),
		participantBridge: participantBridge,
		feishu:            feishu,
		llm:               llmSvc,
		serverAccessToken: serverAccessToken,
		serverNoAuth:      serverNoAuth,
		upgradeApply:      upgrade.StartApplyHelper,
	}
	return h
}

func (h *Handler) SetNotificationDeliver(d notification.Fanouter) {
	if h != nil {
		h.notificationDeliver = d
	}
}

func (h *Handler) SetParticipantService(svc *participant.Service) {
	if h != nil {
		h.participant = svc
	}
}

func (h *Handler) SetActivityDecider(decider ActivityDecider) {
	if h != nil {
		h.activityDecider = decider
	}
}

func (h *Handler) localChannel() *csgclawchannel.Service {
	if h == nil {
		return nil
	}
	if h.csgclaw != nil {
		return h.csgclaw
	}
	return csgclawchannel.NewService(h.im)
}

func (h *Handler) requireLocalChannel(w http.ResponseWriter) (*csgclawchannel.Service, bool) {
	channel := h.localChannel()
	if channel == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return channel, true
}

func (h *Handler) SetUpgradeManager(manager *upgrade.Manager) {
	h.upgradeManager = manager
}

func (h *Handler) SetHubService(svc *hub.Service) {
	h.hub = svc
}

func (h *Handler) SetTeamService(svc *team.Service) {
	if h != nil {
		h.teamSvc = svc
	}
}

func (h *Handler) SetTeamAdapter(adapter team.TeamChannelAdapter) {
	if h != nil {
		h.teamAdapter = adapter
	}
}

func (h *Handler) SetUpgradeConfigPath(configPath string) {
	h.upgradeConfigPath = strings.TrimSpace(configPath)
}

func (h *Handler) SetServerRestartApplyFunc(apply func(upgrade.RestartHelperOptions) error) {
	if apply == nil {
		h.serverRestartApply = upgrade.StartRestartHelper
		return
	}
	h.serverRestartApply = apply
}

func (h *Handler) SetUpgradeApplyFunc(apply func(upgrade.ApplyHelperOptions) error) {
	if apply == nil {
		h.upgradeApply = upgrade.StartApplyHelper
		return
	}
	h.upgradeApply = apply
}

func (h *Handler) SetConfigPath(path string) {
	if h != nil {
		h.configPath = strings.TrimSpace(path)
	}
}

func (h *Handler) validateServerAccessToken(authHeader string) bool {
	if h.serverNoAuth {
		return true
	}
	token := strings.TrimSpace(h.serverAccessToken)
	return authHeader == "Bearer "+token
}

func (h *Handler) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *Handler) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, apitypes.VersionResponse{
		Version: version.Current(),
	})
}

func (h *Handler) handleUpgradeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.upgradeManager == nil {
		http.Error(w, "upgrade manager is not configured", http.StatusServiceUnavailable)
		return
	}
	if outcome, err := upgrade.ConsumeApplyStatus(h.upgradeConfigPath); err != nil {
		http.Error(w, fmt.Sprintf("read upgrade helper status: %v", err), http.StatusInternalServerError)
		return
	} else {
		switch outcome.Status {
		case upgrade.ApplyStatusFailed:
			if outcome.Message != "" {
				h.upgradeManager.MarkUpgradeFailed(errors.New(outcome.Message))
			}
		case upgrade.ApplyStatusManualRestartRequired:
			h.upgradeManager.MarkManualRestartRequired()
		}
	}
	writeJSON(w, http.StatusOK, h.upgradeManager.Status())
}

func (h *Handler) handleUpgradeApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.upgradeManager == nil {
		http.Error(w, "upgrade manager is not configured", http.StatusServiceUnavailable)
		return
	}
	if status := h.upgradeManager.Status(); !status.AutoUpgradeSupported {
		http.Error(w, "current installation is not an official csgclaw bundle; please upgrade manually", http.StatusConflict)
		return
	}

	apply := h.upgradeApply
	if apply == nil {
		apply = upgrade.StartApplyHelper
	}
	h.upgradeManager.MarkUpgrading()
	if err := apply(upgrade.ApplyHelperOptions{ConfigPath: h.upgradeConfigPath}); err != nil {
		h.upgradeManager.MarkUpgradeFailed(err)
		http.Error(w, fmt.Sprintf("start upgrade helper: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusAccepted, apitypes.UpgradeActionResponse{
		Status:  "accepted",
		Message: "upgrade helper started",
	})
}

func (h *Handler) handleAgents(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if err := h.svc.Reload(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, h.presentAgentsForRequest(r, h.svc.List()))
	case http.MethodPost:
		h.handleCreateAgentWorker(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleAgentByID(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}

	id := pathValue(r, "id")
	if id == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if err := h.svc.Reload(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a, ok := h.svc.Agent(id)
		if !ok {
			http.Error(w, "agent not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, h.presentAgentForRequest(r, a))
	case http.MethodPatch:
		var req agent.UpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		updated, err := h.svc.Update(r.Context(), id, req)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		h.publishUpdatedAgentUser(updated)
		writeJSON(w, http.StatusOK, h.presentAgentResponse(updated))
	case http.MethodDelete:
		if err := h.svc.Delete(r.Context(), id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "agent not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) publishUpdatedAgentUser(updated agent.Agent) {
	if h == nil || h.im == nil {
		return
	}
	user, ok, err := h.im.UpdateAgentUser(im.UpdateAgentUserRequest{
		ID:     updated.ID,
		Name:   updated.Name,
		Role:   updated.Role,
		Avatar: updated.Avatar,
	})
	if err != nil || !ok {
		return
	}
	if h.imBus != nil {
		userCopy := user
		h.imBus.Publish(im.Event{
			Type: im.EventTypeUserUpdated,
			User: &userCopy,
		})
	}
}

func (h *Handler) handleAgentProfileByID(w http.ResponseWriter, r *http.Request) {
	id := pathValue(r, "id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	h.handleAgentProfile(w, r, id)
}

func (h *Handler) handleAgentRecreateByID(w http.ResponseWriter, r *http.Request) {
	id := pathValue(r, "id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	h.handleAgentRecreate(w, r, id)
}

func (h *Handler) handleAgentUpgradeByID(w http.ResponseWriter, r *http.Request) {
	id := pathValue(r, "id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	h.handleAgentUpgrade(w, r, id)
}

func (h *Handler) handleAgentProfile(w http.ResponseWriter, r *http.Request, id string) {
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if err := h.svc.Reload(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		profile, err := h.svc.AgentProfileView(id)
		if err != nil {
			http.Error(w, "agent not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, profile)
	case http.MethodPut:
		var req agent.AgentProfile
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		profile, err := h.svc.UpdateAgentProfile(id, req)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		writeJSON(w, http.StatusOK, profile)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleAgentRecreate(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	recreated, err := h.svc.Recreate(r.Context(), id)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, presentAgent(recreated))
}

func (h *Handler) handleAgentUpgrade(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	recreated, err := h.svc.Upgrade(r.Context(), id)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, presentAgent(recreated))
}

func (h *Handler) handleAgentProfileModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	var req agent.ProfileModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	models, err := h.svc.ListModelsForRequest(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, agent.ProfileModelsResponse{
		Provider: req.Provider,
		Models:   models,
	})
}

func (h *Handler) handleAgentProfileDefaults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, h.svc.ProfileDefaultsView())
}

func (h *Handler) handleAgentStart(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.svc.Reload(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	started, err := h.svc.Start(r.Context(), id)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, presentAgent(started))
}

func (h *Handler) handleAgentStartByID(w http.ResponseWriter, r *http.Request) {
	id := pathValue(r, "id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	h.handleAgentStart(w, r, id)
}

func (h *Handler) handleAgentStop(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.svc.Reload(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	stopped, err := h.svc.Stop(r.Context(), id)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, presentAgent(stopped))
}

func (h *Handler) handleAgentStopByID(w http.ResponseWriter, r *http.Request) {
	id := pathValue(r, "id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	h.handleAgentStop(w, r, id)
}

func (h *Handler) handleAgentLogs(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.svc.Reload(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	lines := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("lines")); raw != "" {
		if _, err := fmt.Sscanf(raw, "%d", &lines); err != nil || lines <= 0 {
			http.Error(w, "invalid lines value", http.StatusBadRequest)
			return
		}
	}
	follow := parseBoolQuery(r.URL.Query().Get("follow"))

	logWriter := io.Writer(w)
	if follow {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming is not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		logWriter = flushWriter{ResponseWriter: w, flusher: flusher}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := h.svc.StreamLogs(r.Context(), id, follow, lines, logWriter); err != nil {
		if !parseBoolQuery(r.URL.Query().Get("follow")) {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		if _, writeErr := io.WriteString(w, err.Error()+"\n"); writeErr != nil {
			return
		}
	}
}

func (h *Handler) handleAgentLogsByID(w http.ResponseWriter, r *http.Request) {
	id := pathValue(r, "id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	h.handleAgentLogs(w, r, id)
}

type flushWriter struct {
	http.ResponseWriter
	flusher http.Flusher
}

func (w flushWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if n > 0 {
		w.flusher.Flush()
	}
	return n, err
}

func parseBoolQuery(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (h *Handler) handleCreateAgentWorker(w http.ResponseWriter, r *http.Request) {
	var req apitypes.CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	created, err := h.svc.Create(r.Context(), agentCreateRequestFromAPI(req))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, h.presentAgentResponse(created))
}

func agentCreateRequestFromAPI(req apitypes.CreateAgentRequest) agent.CreateRequest {
	prof := agentProfileFromAPI(req.AgentProfile)
	return agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			ID:             req.ID,
			Name:           req.Name,
			Description:    req.Description,
			Instructions:   req.Instructions,
			Image:          req.Image,
			Avatar:         req.Avatar,
			RuntimeKind:    req.RuntimeKind,
			FromTemplate:   req.FromTemplate,
			Role:           req.Role,
			Status:         req.Status,
			CreatedAt:      req.CreatedAt,
			Profile:        req.Profile,
			RuntimeOptions: utils.CloneAnyMapShallowNestedStringMaps(req.RuntimeOptions),
			AgentProfile:   prof,
		},
		Replace:   req.Replace,
		FieldMask: req.FieldMask,
	}
}

func (h *Handler) handleHubTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if h.hub == nil {
			http.Error(w, "hub service is not configured", http.StatusServiceUnavailable)
			return
		}
		items, err := h.hub.List(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, presentHubTemplates(items))
	case http.MethodPost:
		if h.hub == nil || h.svc == nil {
			http.Error(w, "hub service is not configured", http.StatusServiceUnavailable)
			return
		}
		var req apitypes.CreateHubTemplateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		spec, err := h.svc.HubPublishSpec(req.AgentID)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		spec.Registry = req.Registry
		item, err := h.hub.Publish(r.Context(), spec)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusCreated, presentHubTemplate(item))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleHubTemplateByID(w http.ResponseWriter, r *http.Request) {
	h.handleHubTemplateByResolvedID(w, r, pathValue(r, "id"))
}

func (h *Handler) handleHubTemplateByResolvedID(w http.ResponseWriter, r *http.Request, id string) {
	if h.hub == nil {
		http.Error(w, "hub service is not configured", http.StatusServiceUnavailable)
		return
	}
	if id == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		item, err := h.hub.Get(r.Context(), id)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		presented, err := h.presentHubTemplateDetail(r.Context(), item)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, os.ErrNotExist) {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		writeJSON(w, http.StatusOK, presented)
	case http.MethodDelete:
		if err := h.hub.Delete(r.Context(), id); err != nil {
			status := http.StatusBadRequest
			switch {
			case errors.Is(err, hub.ErrTemplateNotFound), strings.Contains(strings.ToLower(err.Error()), "not found"):
				status = http.StatusNotFound
			case errors.Is(err, hub.ErrRegistryNotDeletable), errors.Is(err, hub.ErrRegistryNotWritable):
				status = http.StatusForbidden
			}
			http.Error(w, err.Error(), status)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleHubTemplateWorkspaceFileByID(w http.ResponseWriter, r *http.Request) {
	h.handleHubTemplateWorkspaceFile(w, r, pathValue(r, "id"))
}

func presentHubTemplates(items []hub.Template) []apitypes.HubTemplate {
	out := make([]apitypes.HubTemplate, 0, len(items))
	for _, item := range items {
		out = append(out, presentHubTemplate(item))
	}
	return out
}

func presentHubTemplate(item hub.Template) apitypes.HubTemplate {
	return apitypes.HubTemplate{
		ID:          item.ID,
		Name:        item.Name,
		Description: item.Description,
		Role:        item.Role,
		RuntimeKind: item.RuntimeKind,
		Version:     item.Version,
		Image:       item.Image,
		ImageEnv:    append([]apitypes.ImageEnvContract(nil), item.ImageEnv...),
		UpdatedAt:   item.UpdatedAt,
		Source: apitypes.HubTemplateSource{
			Name: item.Source.Name,
			Kind: item.Source.Kind,
		},
		Workspace: apitypes.HubTemplateWorkspace{
			Kind: item.WorkspaceRef.Kind,
		},
	}
}

func agentProfileFromAPI(req *apitypes.CreateAgentProfile) agent.AgentProfile {
	if req == nil {
		return agent.AgentProfile{}
	}
	return agent.AgentProfile{
		Name:            req.Name,
		Description:     req.Description,
		Provider:        req.Provider,
		BaseURL:         req.BaseURL,
		APIKey:          req.APIKey,
		Headers:         req.Headers,
		ModelID:         req.ModelID,
		ReasoningEffort: req.ReasoningEffort,
		EnableFastMode:  req.EnableFastMode,
		RequestOptions:  req.RequestOptions,
		Env:             req.Env,
		ProfileComplete: req.ProfileComplete,
	}
}

func (h *Handler) workerIMProvisioner() *im.Provisioner {
	if h == nil || h.im == nil {
		return nil
	}
	if h.imProvisioner == nil {
		h.imProvisioner = im.NewProvisioner(h.im, h.imBus)
	}
	return h.imProvisioner
}

func (h *Handler) handleIMBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.reloadIM(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, presentBootstrap(h.im.Bootstrap()))
}

func (h *Handler) handleRooms(w http.ResponseWriter, r *http.Request) {
	channel, ok := h.requireLocalChannel(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if err := h.reloadIM(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, channel.ListRooms())
	case http.MethodPost:
		h.handleCreateRoom(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleUsers(w http.ResponseWriter, r *http.Request) {
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if err := h.reloadIM(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, h.im.ListUsers())
	case http.MethodPost:
		h.handleCreateUser(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleMessages(w http.ResponseWriter, r *http.Request) {
	channel, ok := h.requireLocalChannel(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if err := h.reloadIM(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		roomID, err := roomIDFromQuery(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		messages, err := channel.ListMessagesWithOptions(roomID, im.ListMessagesOptions{
			IncludeThreadReplies: parseBoolQuery(r.URL.Query().Get("include_thread_replies")),
		})
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, messages)
	case http.MethodPost:
		h.handleCreateMessage(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleThreadsByRoomID(w http.ResponseWriter, r *http.Request) {
	roomID := pathValue(r, "id")
	if roomID == "" {
		http.NotFound(w, r)
		return
	}
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if err := h.reloadIM(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		opts, err := threadListOptionsFromQuery(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		threads, err := h.im.ListThreads(roomID, opts)
		if err != nil {
			writeIMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, threads)
	case http.MethodPost:
		var req im.StartThreadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		req.RoomID = roomID
		thread, created, err := h.im.StartThread(req)
		if err != nil {
			writeIMError(w, err)
			return
		}
		if created {
			h.publishThreadEvent(im.EventTypeThreadCreated, thread)
			writeJSON(w, http.StatusCreated, thread)
			return
		}
		writeJSON(w, http.StatusOK, thread)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleThreadByID(w http.ResponseWriter, r *http.Request) {
	roomID := pathValue(r, "id")
	rootID := pathValue(r, "thread_id")
	if roomID == "" || rootID == "" {
		http.NotFound(w, r)
		return
	}
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.reloadIM(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	thread, err := h.im.GetThread(roomID, rootID)
	if err != nil {
		writeIMError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, thread)
}

func (h *Handler) handleThreadRelationsByID(w http.ResponseWriter, r *http.Request) {
	roomID := pathValue(r, "id")
	rootID := pathValue(r, "event_id")
	if roomID == "" || rootID == "" {
		http.NotFound(w, r)
		return
	}
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.reloadIM(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	relations, err := h.im.ListThreadRelations(roomID, rootID)
	if err != nil {
		writeIMError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, relations)
}

func threadListOptionsFromQuery(r *http.Request) (im.ThreadListOptions, error) {
	opts := im.ThreadListOptions{
		Include: strings.TrimSpace(r.URL.Query().Get("include")),
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return im.ThreadListOptions{}, fmt.Errorf("invalid limit")
		}
		opts.Limit = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("from")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return im.ThreadListOptions{}, fmt.Errorf("invalid from")
		}
		opts.From = value
	}
	return opts, nil
}

func writeIMError(w http.ResponseWriter, err error) {
	if strings.Contains(err.Error(), "not found") {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}

func (h *Handler) handleRoomByID(w http.ResponseWriter, r *http.Request) {
	id := pathValue(r, "id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	h.handleLocalRoomByID(w, r, id)
}

func (h *Handler) handleLocalRoomByID(w http.ResponseWriter, r *http.Request, id string) {
	channel, ok := h.requireLocalChannel(w)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := channel.DeleteRoom(id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "room not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleClearRoomMessages(w http.ResponseWriter, r *http.Request) {
	roomID := strings.TrimSpace(pathValue(r, "id"))
	if roomID == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}
	if h == nil || h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}

	room, err := h.im.ClearRoomMessages(roomID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, room)
}

func (h *Handler) handleRoomMembersByIDPath(w http.ResponseWriter, r *http.Request) {
	id := pathValue(r, "id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	h.handleRoomMembersByID(w, r, id)
}

func (h *Handler) handleRoomMembersByID(w http.ResponseWriter, r *http.Request, roomID string) {
	channel, ok := h.requireLocalChannel(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if err := h.reloadIM(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case http.MethodPost:
		h.handleAddRoomMembers(w, r, roomID)
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	members, err := channel.ListRoomMembers(roomID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (h *Handler) handleUserByID(w http.ResponseWriter, r *http.Request) {
	id := pathValue(r, "id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	h.handleLocalUserByID(w, r, id)
}

func (h *Handler) handleLocalUserByID(w http.ResponseWriter, r *http.Request, id string) {
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := h.im.DeleteUser(id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "user not found", http.StatusNotFound)
				return
			}
			if strings.Contains(err.Error(), "cannot delete current user") {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	provisioner := h.workerIMProvisioner()
	if provisioner == nil {
		http.Error(w, "im provisioner is not configured", http.StatusServiceUnavailable)
		return
	}

	var req apitypes.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	id := strings.TrimSpace(req.ID)
	name := strings.TrimSpace(req.Name)
	handle := strings.TrimSpace(req.Handle)
	role := strings.TrimSpace(req.Role)
	rawID := id
	id = h.resolveCSGClawParticipantUserID(id)
	if rawID == agent.ManagerUserID {
		id = agent.ManagerParticipantID
	}

	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if handle == "" {
		handle = name
	}
	if id == agent.ManagerParticipantID {
		if user, ok := h.im.User(id); ok {
			writeJSON(w, http.StatusCreated, user)
			return
		}
	}

	if h.participant != nil && h.svc != nil && shouldCreateWorkerForUser(id, role) {
		participantID := workerParticipantIDFromUserID(id)
		created, err := h.participant.Create(r.Context(), participant.CreateRequest{
			ID:      participantID,
			Channel: participant.ChannelCSGClaw,
			Type:    participant.TypeAgent,
			Name:    name,
			ChannelUser: participant.ChannelUserSpec{
				Ref:  id,
				Kind: participant.ChannelUserKindLocalUserID,
			},
			AgentBinding: participant.AgentBindingSpec{
				Mode:    participant.BindingModeCreate,
				AgentID: id,
				Agent: &agent.CreateAgentSpec{
					ID:   id,
					Name: name,
					Role: agent.RoleWorker,
				},
			},
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if user, ok := h.im.User(created.ChannelUserRef); ok {
			h.publishUserEvent(im.EventTypeUserCreated, user)
			if room, ok := h.directRoomWithMembers(im.AdminUserID, user.ID); ok {
				h.publishRoomEvent(im.EventTypeRoomCreated, room)
			}
			writeJSON(w, http.StatusCreated, user)
			return
		}
		http.Error(w, "created worker user not found", http.StatusInternalServerError)
		return
	}

	result, err := provisioner.EnsureAgentUser(r.Context(), im.AgentIdentity{
		ID:     id,
		Name:   name,
		Handle: handle,
		Role:   role,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, result.User)
}

func shouldCreateWorkerForUser(id, role string) bool {
	id = strings.TrimSpace(id)
	switch strings.ToLower(id) {
	case "", im.AdminUserID, "u-admin", agent.ManagerUserID:
		return false
	}

	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", agent.RoleWorker, agent.RoleAgent:
		return true
	case agent.RoleManager, "admin":
		return false
	default:
		return true
	}
}

func workerParticipantIDFromUserID(id string) string {
	id = strings.TrimSpace(id)
	withoutPrefix := strings.TrimPrefix(id, "u-")
	if withoutPrefix != "" && withoutPrefix != id {
		return withoutPrefix
	}
	return id
}

func (h *Handler) directRoomWithMembers(left, right string) (im.Room, bool) {
	if h == nil || h.im == nil {
		return im.Room{}, false
	}
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	for _, room := range h.im.ListRooms() {
		if !room.IsDirect {
			continue
		}
		if roomHasMember(room.Members, left) && roomHasMember(room.Members, right) {
			return room, true
		}
	}
	return im.Room{}, false
}

func roomHasMember(members []string, id string) bool {
	id = strings.TrimSpace(id)
	for _, member := range members {
		if strings.TrimSpace(member) == id {
			return true
		}
	}
	return false
}

func (h *Handler) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	channel, ok := h.requireLocalChannel(w)
	if !ok {
		return
	}
	var req createMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	serviceReq, err := req.toServiceRequest()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	serviceReq = h.resolveCSGClawParticipantMessageRequest(serviceReq)

	message, err := channel.SendMessage(serviceReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.publishMessageCreated(serviceReq.RoomID, message.SenderID, message)
	h.publishThreadUpdated(serviceReq.RoomID, message)
	h.handleTeamRoomCommand(r.Context(), serviceReq.RoomID, message.SenderID, message.Content)
	writeJSON(w, http.StatusCreated, message)
}

func (h *Handler) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	channel, ok := h.requireLocalChannel(w)
	if !ok {
		return
	}
	var req apitypes.CreateRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	req.CreatorID = h.resolveCSGClawParticipantUserID(req.CreatorID)
	req.MemberIDs = h.resolveCSGClawParticipantUserIDs(req.MemberIDs)

	room, err := channel.CreateRoom(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.publishRoomEvent(im.EventTypeRoomCreated, room)
	writeJSON(w, http.StatusCreated, room)
}

func (h *Handler) handleIMMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.handleCreateMessage(w, r)
}

func (h *Handler) handleIMRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.handleCreateRoom(w, r)
}

func (h *Handler) handleIMRoomMembers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.handleAddRoomMembers(w, r, "")
}

func (h *Handler) handleAddRoomMembers(w http.ResponseWriter, r *http.Request, pathRoomID string) {
	channel, ok := h.requireLocalChannel(w)
	if !ok {
		return
	}
	var req addRoomMembersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	pathRoomID = strings.TrimSpace(pathRoomID)
	if pathRoomID != "" {
		bodyRoomID := strings.TrimSpace(req.RoomID)
		if bodyRoomID != "" && bodyRoomID != pathRoomID {
			http.Error(w, "room_id does not match path room id", http.StatusBadRequest)
			return
		}
		req.RoomID = pathRoomID
	}

	serviceReq, err := req.toServiceRequest()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	serviceReq.InviterID = h.resolveCSGClawParticipantUserID(serviceReq.InviterID)
	serviceReq.UserIDs = h.resolveCSGClawParticipantUserIDs(serviceReq.UserIDs)

	room, err := channel.AddRoomMembers(serviceReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.publishRoomEvent(im.EventTypeRoomMembersAdded, room)
	writeJSON(w, http.StatusOK, room)
}

func (h *Handler) handleIMEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.imBus == nil {
		http.Error(w, "im events are not configured", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	events, cancel := h.imBus.Subscribe()
	defer cancel()

	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()

	ticker := time.NewTicker(sseHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case evt, ok := <-events:
			if !ok {
				return
			}
			data, err := json.Marshal(presentEvent(evt))
			if err != nil {
				return
			}
			if _, err := io.Copy(w, bytes.NewReader([]byte("data: "))); err != nil {
				return
			}
			if _, err := io.Copy(w, bytes.NewReader(data)); err != nil {
				return
			}
			if _, err := io.WriteString(w, "\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func deriveAgentHandle(a agent.Agent) string {
	if handle, ok := sanitizeHandle(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(a.Name), " ", "-"))); ok {
		return handle
	}
	if handle, ok := sanitizeHandle(strings.ToLower(strings.TrimPrefix(strings.TrimSpace(a.ID), "u-"))); ok {
		return handle
	}
	switch strings.ToLower(strings.TrimSpace(a.Role)) {
	case agent.RoleManager:
		return "manager"
	case agent.RoleWorker:
		return "worker"
	default:
		return "agent"
	}
}

func displayRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case agent.RoleManager:
		return "manager"
	case agent.RoleWorker:
		return "Worker"
	default:
		return "Agent"
	}
}

func sanitizeHandle(input string) (string, bool) {
	var b strings.Builder
	hasAlphaNum := false
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			hasAlphaNum = true
			b.WriteRune(r)
			continue
		}
		if r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 || !hasAlphaNum {
		return "", false
	}
	return b.String(), true
}

func roomIDFromQuery(r *http.Request) (string, error) {
	roomID := strings.TrimSpace(r.URL.Query().Get("room_id"))
	if roomID == "" {
		return "", fmt.Errorf("room_id is required")
	}
	return roomID, nil
}

func presentBootstrap(state im.Bootstrap) imBootstrapResponse {
	return imBootstrapResponse{
		CurrentUserID:      state.CurrentUserID,
		Users:              state.Users,
		Rooms:              state.Rooms,
		InviteDraftUserIDs: state.InviteDraftUserIDs,
	}
}

func presentAgents(items []agent.Agent) []agentResponse {
	out := make([]agentResponse, 0, len(items))
	for _, item := range items {
		out = append(out, presentAgent(item))
	}
	return out
}

func (h *Handler) presentAgentsForRequest(r *http.Request, items []agent.Agent) []agentResponse {
	out := make([]agentResponse, 0, len(items))
	for _, item := range items {
		out = append(out, h.presentAgentResponse(item))
	}
	if !includeParticipants(r) || h == nil || h.participant == nil {
		return out
	}
	byAgent := participantsByAgentID(presentParticipants(h.participant.List(participant.ListOptions{})))
	for i := range out {
		out[i].Participants = byAgent[out[i].ID]
	}
	return out
}

func (h *Handler) presentAgentForRequest(r *http.Request, item agent.Agent) agentResponse {
	resp := h.presentAgentResponse(item)
	if !includeParticipants(r) || h == nil || h.participant == nil {
		return resp
	}
	resp.Participants = presentParticipants(h.participant.List(participant.ListOptions{AgentID: item.ID}))
	return resp
}

func (h *Handler) presentAgentResponse(item agent.Agent) agentResponse {
	resp := presentAgent(item)
	resp.RuntimeOptionSchemas = h.runtimeOptionSchemasForKind(item.RuntimeKind)
	return resp
}

func (h *Handler) runtimeOptionSchemasByKind(kinds []string) map[string][]agentruntime.RuntimeOptionSchema {
	if h == nil {
		return nil
	}
	out := map[string][]agentruntime.RuntimeOptionSchema{}
	for _, kind := range kinds {
		schemas := h.runtimeOptionSchemasForKind(kind)
		if len(schemas) == 0 {
			continue
		}
		out[strings.TrimSpace(kind)] = schemas
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (h *Handler) runtimeOptionSchemasForKind(kind string) []agentruntime.RuntimeOptionSchema {
	if h == nil || h.svc == nil {
		return nil
	}
	rt, err := h.svc.Runtime(strings.TrimSpace(kind))
	if err != nil {
		return nil
	}
	provider, ok := rt.(agentruntime.RuntimeOptionSchemaProvider)
	if !ok {
		return nil
	}
	schemas := provider.RuntimeOptionsSchema()
	if len(schemas) == 0 {
		return nil
	}
	return append([]agentruntime.RuntimeOptionSchema(nil), schemas...)
}

func includeParticipants(r *http.Request) bool {
	if r == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_participants")), "true")
}

func participantsByAgentID(items []apitypes.Participant) map[string][]apitypes.Participant {
	out := make(map[string][]apitypes.Participant)
	for _, item := range items {
		if strings.TrimSpace(item.AgentID) == "" {
			continue
		}
		out[item.AgentID] = append(out[item.AgentID], item)
	}
	return out
}

func (h *Handler) resolveCSGClawParticipantMessageRequest(req im.CreateMessageRequest) im.CreateMessageRequest {
	req.SenderID = h.resolveCSGClawParticipantUserID(req.SenderID)
	req.MentionID = h.resolveCSGClawParticipantUserID(req.MentionID)
	return req
}

func (h *Handler) resolveCSGClawParticipantUserIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if resolved := h.resolveCSGClawParticipantUserID(id); resolved != "" {
			out = append(out, resolved)
		}
	}
	return out
}

func (h *Handler) resolveCSGClawParticipantUserID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || h == nil || h.participant == nil {
		if id == agent.ManagerUserID {
			return agent.ManagerParticipantID
		}
		return id
	}
	item, ok := h.participant.Get(participant.ChannelCSGClaw, id)
	if ok {
		if ref := strings.TrimSpace(item.ChannelUserRef); ref != "" {
			return ref
		}
		return id
	}
	for _, candidate := range h.participant.List(participant.ListOptions{Channel: participant.ChannelCSGClaw, AgentID: id}) {
		if ref := strings.TrimSpace(candidate.ChannelUserRef); ref != "" {
			return ref
		}
		if participantID := strings.TrimSpace(candidate.ID); participantID != "" {
			return participantID
		}
	}
	if id == agent.ManagerUserID {
		return agent.ManagerParticipantID
	}
	return id
}

func presentAgent(item agent.Agent) agentResponse {
	av := agent.RedactedProfileViewForAgent(item)
	if strings.TrimSpace(av.Name) == strings.TrimSpace(item.Name) {
		av.Name = ""
	}
	if strings.TrimSpace(av.Description) == strings.TrimSpace(item.Description) {
		av.Description = ""
	}
	runtimeOptions := item.RuntimeOptions
	if runtimeOptions == nil {
		runtimeOptions = map[string]any{}
	}
	return agentResponse{
		ID:               item.ID,
		Name:             item.Name,
		Description:      item.Description,
		Instructions:     item.Instructions,
		RuntimeID:        item.RuntimeID,
		RuntimeKind:      item.RuntimeKind,
		Image:            item.Image,
		Avatar:           item.Avatar,
		BoxID:            item.BoxID,
		Role:             item.Role,
		Status:           item.Status,
		CreatedAt:        item.CreatedAt,
		Profile:          item.Profile,
		RuntimeOptions:   runtimeOptions,
		AgentProfile:     av,
		ProfileComplete:  item.ProfileComplete,
		DetectionResults: append([]agent.ProfileDetectionResult(nil), item.DetectionResults...),
	}
}

func presentEvent(evt im.Event) imEventResponse {
	return imEventResponse{
		Type:    evt.Type,
		RoomID:  evt.RoomID,
		Room:    evt.Room,
		User:    evt.User,
		Message: evt.Message,
		Thread:  evt.Thread,
		Sender:  evt.Sender,
		Upgrade: evt.Upgrade,
	}
}

func (r createMessageRequest) toServiceRequest() (im.CreateMessageRequest, error) {
	roomID := strings.TrimSpace(r.RoomID)
	if roomID == "" {
		return im.CreateMessageRequest{}, fmt.Errorf("room_id is required")
	}

	return im.CreateMessageRequest{
		RoomID:    roomID,
		SenderID:  r.SenderID,
		Content:   r.Content,
		MentionID: r.MentionID,
		RelatesTo: r.RelatesTo,
	}, nil
}

func (r addRoomMembersRequest) toServiceRequest() (im.AddRoomMembersRequest, error) {
	roomID := strings.TrimSpace(r.RoomID)
	if roomID == "" {
		return im.AddRoomMembersRequest{}, fmt.Errorf("room_id is required")
	}

	return im.AddRoomMembersRequest{
		RoomID:    roomID,
		InviterID: r.InviterID,
		UserIDs:   r.UserIDs,
		Locale:    r.Locale,
	}, nil
}

func (h *Handler) reloadIM() error {
	if h == nil || h.im == nil {
		return nil
	}
	return h.im.Reload()
}

func (h *Handler) publishMessageCreated(conversationID, senderID string, message im.Message) {
	if h.imBus == nil {
		return
	}
	sender, ok := h.im.User(senderID)
	if !ok {
		return
	}
	messageCopy := message
	senderCopy := sender
	h.imBus.Publish(im.Event{
		Type:    im.EventTypeMessageCreated,
		RoomID:  conversationID,
		Message: &messageCopy,
		Sender:  &senderCopy,
	})
}

func (h *Handler) handleTeamRoomCommand(ctx context.Context, roomID string, senderID string, content string) {
	if h == nil || h.teamSvc == nil || h.teamAdapter == nil {
		return
	}
	parser := team.NewCommandParser(h.teamSvc, h.teamAdapter, func(id string) bool {
		id = strings.TrimSpace(id)
		if id == "" {
			return false
		}
		if strings.HasPrefix(strings.ToLower(id), "bot-") {
			return false
		}
		return !h.isAgentSender(id)
	})
	parser.HandleMessage(ctx, roomID, senderID, content)
}

func (h *Handler) publishThreadUpdated(roomID string, message im.Message) {
	if h.imBus == nil || h.im == nil || message.RelatesTo == nil || strings.TrimSpace(message.RelatesTo.RelType) != im.RelationTypeThread {
		return
	}
	thread, err := h.im.GetThread(roomID, message.RelatesTo.EventID)
	if err != nil {
		return
	}
	h.publishThreadEvent(im.EventTypeThreadUpdated, thread)
}

func (h *Handler) publishThreadEvent(eventType string, thread im.ThreadView) {
	if h.imBus == nil {
		return
	}
	threadCopy := thread
	h.imBus.Publish(im.Event{
		Type:   eventType,
		RoomID: thread.RoomID,
		Thread: &threadCopy,
	})
}

func (h *Handler) publishRoomEvent(eventType string, room im.Room) {
	if h.imBus == nil {
		return
	}
	roomCopy := room
	h.imBus.Publish(im.Event{
		Type:   eventType,
		RoomID: room.ID,
		Room:   &roomCopy,
	})
}

func (h *Handler) publishUserEvent(eventType string, user im.User) {
	if h.imBus == nil {
		return
	}
	userCopy := user
	h.imBus.Publish(im.Event{
		Type: eventType,
		User: &userCopy,
	})
}
