package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"csgclaw/internal/channelbridge/codexbridge"
	agentruntime "csgclaw/internal/runtime"
	runtimecodex "csgclaw/internal/runtime/codex"
)

const (
	defaultHome        = "/home/codex/.codex-sandbox"
	defaultProfile     = "csgclaw"
	defaultConfig      = defaultHome + "/config.json"
	defaultWorkspace   = defaultHome + "/workspace/projects"
	defaultCodexHome   = defaultHome + "/codex-home"
	defaultBridgeBin   = "/usr/local/bin/lark-channel-bridge"
	defaultCodexBin    = "/usr/local/bin/codex"
	defaultHealthAddr  = "127.0.0.1:18791"
	appSecretEnvKey    = "APP_SECRET"
	startupGracePeriod = 2 * time.Second
)

var (
	startCSGClawBridgeFunc = startCSGClawBridge
	startLarkBridgeFunc    = startLarkBridge
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	state := &gatewayState{}
	healthSrv := startHealthServer(envString("CSGCLAW_CODEX_GATEWAY_HEALTH_ADDR", defaultHealthAddr), state)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = healthSrv.Shutdown(shutdownCtx)
	}()

	if err := run(ctx, state); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("gateway stopped: %v", err)
		os.Exit(1)
	}
}

type gatewayState struct {
	ready           atomic.Bool
	csgclawStarted  atomic.Bool
	larkBridgeAlive atomic.Bool
}

func (s *gatewayState) setReady(ready bool) {
	if s == nil {
		return
	}
	s.ready.Store(ready)
}

func (s *gatewayState) isReady() bool {
	return s != nil && s.ready.Load()
}

func (s *gatewayState) setBridgeStarted(started bool) {
	if s == nil {
		return
	}
	s.larkBridgeAlive.Store(started)
}

func (s *gatewayState) isBridgeStarted() bool {
	return s != nil && s.larkBridgeAlive.Load()
}

func (s *gatewayState) setCSGClawStarted(started bool) {
	if s == nil {
		return
	}
	s.csgclawStarted.Store(started)
}

func (s *gatewayState) isCSGClawStarted() bool {
	return s != nil && s.csgclawStarted.Load()
}

func run(ctx context.Context, state *gatewayState) error {
	home := envString("LARK_CHANNEL_HOME", defaultHome)
	profile := envString("LARK_CHANNEL_PROFILE", defaultProfile)
	configPath := envString("LARK_CHANNEL_CONFIG", filepath.Join(home, "config.json"))
	workspace := envString("CSGCLAW_CODEX_GATEWAY_WORKSPACE", defaultWorkspace)
	bridgeBin := envString("CSGCLAW_CODEX_GATEWAY_BRIDGE_BIN", defaultBridgeBin)
	codexBin := envString("LARK_CHANNEL_CODEX_BIN", defaultCodexBin)

	runtimeCfg := gatewayRuntimeConfigFromEnv(home, workspace, codexBin)
	csgclawBridge, err := startCSGClawBridgeFunc(ctx, runtimeCfg)
	if err != nil {
		return err
	}
	defer csgclawBridge.Close()
	state.setCSGClawStarted(true)
	defer state.setCSGClawStarted(false)

	ready, reason := credentialsReady(configPath, profile)
	var larkDone <-chan error
	if !ready {
		log.Printf("Feishu app config is not ready; lark-channel-bridge is disabled for this container: %s", reason)
	} else {
		larkDone, err = startLarkBridgeFunc(ctx, larkBridgeConfig{
			Home:       home,
			Profile:    profile,
			ConfigPath: configPath,
			Workspace:  workspace,
			BridgeBin:  bridgeBin,
			CodexBin:   codexBin,
		})
		if err != nil {
			return err
		}
		state.setBridgeStarted(true)
		defer state.setBridgeStarted(false)
	}

	state.setReady(true)
	defer func() {
		state.setReady(false)
	}()

	for {
		select {
		case err, ok := <-larkDone:
			if !ok {
				larkDone = nil
				state.setBridgeStarted(false)
				continue
			}
			state.setBridgeStarted(false)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				log.Printf("lark-channel-bridge stopped; Feishu/Lark channel is disabled until container restart: %v", err)
			} else {
				log.Printf("lark-channel-bridge stopped; Feishu/Lark channel is disabled until container restart")
			}
			larkDone = nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

type larkBridgeConfig struct {
	Home       string
	Profile    string
	ConfigPath string
	Workspace  string
	BridgeBin  string
	CodexBin   string
}

func startLarkBridge(ctx context.Context, cfg larkBridgeConfig) (<-chan error, error) {
	args := []string{
		"run",
		"--profile", cfg.Profile,
		"--agent", "codex",
		"--config", cfg.ConfigPath,
		"--workspace", cfg.Workspace,
		"--skip-check-lark-cli",
	}
	cmd := exec.CommandContext(ctx, cfg.BridgeBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = append(os.Environ(),
		"LARK_CHANNEL_HOME="+cfg.Home,
		"LARK_CHANNEL_PROFILE="+cfg.Profile,
		"LARK_CHANNEL_CONFIG="+cfg.ConfigPath,
		"LARK_CHANNEL_CODEX_BIN="+cfg.CodexBin,
	)

	log.Printf("starting lark-channel-bridge profile=%s config=%s workspace=%s", cfg.Profile, cfg.ConfigPath, cfg.Workspace)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start lark-channel-bridge: %w", err)
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
		close(waitCh)
	}()

	select {
	case err := <-waitCh:
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			return nil, fmt.Errorf("lark-channel-bridge exited during startup: %w", err)
		}
		return nil, fmt.Errorf("lark-channel-bridge exited during startup")
	case <-time.After(startupGracePeriod):
	case <-ctx.Done():
		<-waitCh
		return nil, ctx.Err()
	}
	return waitCh, nil
}

type gatewayRuntimeConfig struct {
	Home          string
	Workspace     string
	CodexHome     string
	CodexBin      string
	BaseURL       string
	AccessToken   string
	ParticipantID string
	AgentID       string
	AgentName     string
	RuntimeID     string
	LLMBaseURL    string
	LLMAPIKey     string
	ModelID       string
}

func gatewayRuntimeConfigFromEnv(home, workspace, codexBin string) gatewayRuntimeConfig {
	baseURL := envString("CSGCLAW_BASE_URL", "")
	agentID := envString("CSGCLAW_AGENT_ID", "")
	participantID := envString("CSGCLAW_PARTICIPANT_ID", "")
	if participantID == "" {
		participantID = agentID
	}
	if agentID == "" {
		agentID = participantID
	}
	llmBaseURL := envString("CSGCLAW_LLM_BASE_URL", "")
	if llmBaseURL == "" && baseURL != "" && agentID != "" {
		llmBaseURL = strings.TrimRight(baseURL, "/") + "/api/v1/agents/" + agentID + "/llm"
	}
	return gatewayRuntimeConfig{
		Home:          home,
		Workspace:     workspace,
		CodexHome:     envString("CODEX_HOME", defaultCodexHome),
		CodexBin:      codexBin,
		BaseURL:       baseURL,
		AccessToken:   envString("CSGCLAW_ACCESS_TOKEN", ""),
		ParticipantID: participantID,
		AgentID:       agentID,
		AgentName:     envString("CSGCLAW_AGENT_NAME", participantID),
		RuntimeID:     envString("CSGCLAW_RUNTIME_ID", runtimeIDForAgentID(agentID)),
		LLMBaseURL:    llmBaseURL,
		LLMAPIKey:     envString("CSGCLAW_LLM_API_KEY", envString("CSGCLAW_ACCESS_TOKEN", "")),
		ModelID:       envString("CSGCLAW_LLM_MODEL_ID", envString("OPENAI_MODEL", "")),
	}
}

func runtimeIDForAgentID(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ""
	}
	if strings.HasPrefix(agentID, "rt-") {
		return agentID
	}
	return "rt-" + agentID
}

type runningCSGClawBridge struct {
	service *codexbridge.Service
	runtime *runtimecodex.Runtime
	handle  agentruntime.Handle
}

func (b *runningCSGClawBridge) Close() {
	if b == nil {
		return
	}
	if b.service != nil {
		b.service.Close()
	}
	if b.runtime != nil && strings.TrimSpace(b.handle.RuntimeID) != "" {
		_, _ = b.runtime.Stop(context.Background(), b.handle)
	}
}

func startCSGClawBridge(ctx context.Context, cfg gatewayRuntimeConfig) (*runningCSGClawBridge, error) {
	cfg = cfg.normalized()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if err := os.Setenv("CODEX_HOME", cfg.CodexHome); err != nil {
		return nil, fmt.Errorf("set CODEX_HOME: %w", err)
	}
	events := runtimecodex.NewEventSink()
	rt := runtimecodex.New(runtimecodex.Dependencies{
		BinaryProvider: staticCodexBinaryProvider{path: cfg.CodexBin},
		EventSink:      events,
		AgentHome: func(string) (string, error) {
			return filepath.Join(cfg.Home, "codex-runtime", cfg.AgentID), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (runtimecodex.AgentRef, error) {
			runtimeID := strings.TrimSpace(h.RuntimeID)
			if runtimeID != "" && runtimeID != cfg.RuntimeID {
				return runtimecodex.AgentRef{}, fmt.Errorf("unknown runtime id %q", runtimeID)
			}
			return runtimecodex.AgentRef{
				ID:        cfg.AgentID,
				Name:      cfg.AgentName,
				RuntimeID: cfg.RuntimeID,
				RuntimeOptions: map[string]any{
					"local_workspace_dir": cfg.Workspace,
				},
				Profile: agentruntime.Profile{
					BaseURL: cfg.LLMBaseURL,
					APIKey:  cfg.LLMAPIKey,
					ModelID: cfg.ModelID,
				},
			}, nil
		},
	})
	handle, err := rt.New(ctx, agentruntime.Spec{
		RuntimeID: cfg.RuntimeID,
		AgentID:   cfg.AgentID,
		AgentName: cfg.AgentName,
		Profile: agentruntime.Profile{
			BaseURL: cfg.LLMBaseURL,
			APIKey:  cfg.LLMAPIKey,
			ModelID: cfg.ModelID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("start codex app-server: %w", err)
	}
	session, err := rt.SessionManager().Session(runtimecodex.SessionHandle{RuntimeID: handle.RuntimeID})
	if err != nil {
		_, _ = rt.Stop(context.Background(), handle)
		return nil, fmt.Errorf("resolve codex app-server session: %w", err)
	}

	client := &codexbridge.HTTPClient{
		BaseURL:     cfg.BaseURL,
		Token:       cfg.AccessToken,
		MentionOnly: true,
	}
	service := codexbridge.NewService(client, rt.SessionManager(), events)
	if err := service.StartBot(ctx, codexbridge.Binding{
		BotID:     cfg.ParticipantID,
		RuntimeID: cfg.RuntimeID,
		SessionID: strings.TrimSpace(session.SessionID),
		PromptMeta: map[string]any{
			"channel":        "csgclaw",
			"participant_id": cfg.ParticipantID,
			"agent_id":       cfg.AgentID,
		},
	}); err != nil {
		service.Close()
		_, _ = rt.Stop(context.Background(), handle)
		return nil, fmt.Errorf("start csgclaw codex bridge: %w", err)
	}
	log.Printf("started CSGClaw Codex bridge base_url=%s participant_id=%s runtime_id=%s", cfg.BaseURL, cfg.ParticipantID, cfg.RuntimeID)
	return &runningCSGClawBridge{service: service, runtime: rt, handle: handle}, nil
}

func (cfg gatewayRuntimeConfig) normalized() gatewayRuntimeConfig {
	cfg.Home = strings.TrimSpace(cfg.Home)
	cfg.Workspace = strings.TrimSpace(cfg.Workspace)
	cfg.CodexHome = strings.TrimSpace(cfg.CodexHome)
	cfg.CodexBin = strings.TrimSpace(cfg.CodexBin)
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.AccessToken = strings.TrimSpace(cfg.AccessToken)
	cfg.ParticipantID = strings.TrimSpace(cfg.ParticipantID)
	cfg.AgentID = strings.TrimSpace(cfg.AgentID)
	cfg.AgentName = strings.TrimSpace(cfg.AgentName)
	cfg.RuntimeID = strings.TrimSpace(cfg.RuntimeID)
	cfg.LLMBaseURL = strings.TrimRight(strings.TrimSpace(cfg.LLMBaseURL), "/")
	cfg.LLMAPIKey = strings.TrimSpace(cfg.LLMAPIKey)
	cfg.ModelID = strings.TrimSpace(cfg.ModelID)
	if cfg.ParticipantID == "" {
		cfg.ParticipantID = cfg.AgentID
	}
	if cfg.AgentID == "" {
		cfg.AgentID = cfg.ParticipantID
	}
	if cfg.AgentName == "" {
		cfg.AgentName = cfg.ParticipantID
	}
	if cfg.RuntimeID == "" {
		cfg.RuntimeID = runtimeIDForAgentID(cfg.AgentID)
	}
	return cfg
}

func (cfg gatewayRuntimeConfig) validate() error {
	missing := make([]string, 0, 6)
	if cfg.Home == "" {
		missing = append(missing, "LARK_CHANNEL_HOME")
	}
	if cfg.Workspace == "" {
		missing = append(missing, "CSGCLAW_CODEX_GATEWAY_WORKSPACE")
	}
	if cfg.CodexHome == "" {
		missing = append(missing, "CODEX_HOME")
	}
	if cfg.CodexBin == "" {
		missing = append(missing, "LARK_CHANNEL_CODEX_BIN")
	}
	if cfg.BaseURL == "" {
		missing = append(missing, "CSGCLAW_BASE_URL")
	}
	if cfg.ParticipantID == "" {
		missing = append(missing, "CSGCLAW_PARTICIPANT_ID")
	}
	if cfg.AgentID == "" {
		missing = append(missing, "CSGCLAW_AGENT_ID")
	}
	if cfg.RuntimeID == "" {
		missing = append(missing, "CSGCLAW_RUNTIME_ID")
	}
	if cfg.LLMBaseURL == "" {
		missing = append(missing, "CSGCLAW_LLM_BASE_URL")
	}
	if cfg.ModelID == "" {
		missing = append(missing, "CSGCLAW_LLM_MODEL_ID")
	}
	if len(missing) > 0 {
		return fmt.Errorf("csgclaw codex bridge config is incomplete: %s", strings.Join(missing, ", "))
	}
	return nil
}

type staticCodexBinaryProvider struct {
	path string
}

func (p staticCodexBinaryProvider) Ensure(context.Context) (string, error) {
	path := strings.TrimSpace(p.path)
	if path == "" {
		return "", fmt.Errorf("codex binary path is required")
	}
	return path, nil
}

func startHealthServer(addr string, state *gatewayState) *http.Server {
	srv := &http.Server{Addr: addr, Handler: newHealthHandler(state)}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("health server failed: %v", err)
		}
	}()
	return srv
}

func newHealthHandler(state *gatewayState) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if !state.isReady() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("starting\n"))
			return
		}
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

func credentialsReady(configPath, profile string) (bool, string) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false, err.Error()
	}
	var cfg bridgeRootConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false, "decode config: " + err.Error()
	}
	profile = strings.TrimSpace(profile)
	if profile == "" {
		profile = strings.TrimSpace(cfg.ActiveProfile)
	}
	if profile == "" {
		return false, "active profile is empty"
	}
	prof, ok := cfg.Profiles[profile]
	if !ok {
		return false, "profile not found: " + profile
	}
	if strings.TrimSpace(prof.Accounts.App.ID) == "" {
		return false, "app id is empty"
	}
	secret := strings.TrimSpace(prof.Accounts.App.Secret)
	if secret == "" {
		return false, "app secret is empty"
	}
	if envName, ok := secretEnvName(secret); ok {
		if strings.TrimSpace(os.Getenv(envName)) == "" {
			return false, "env var " + envName + " is empty"
		}
	}
	return true, ""
}

func secretEnvName(value string) (string, bool) {
	if !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
		return "", false
	}
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}"))
	return name, name != ""
}

func envString(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

type bridgeRootConfig struct {
	ActiveProfile string                         `json:"activeProfile"`
	Profiles      map[string]bridgeProfileConfig `json:"profiles"`
}

type bridgeProfileConfig struct {
	Accounts bridgeAccounts `json:"accounts"`
}

type bridgeAccounts struct {
	App bridgeAppCredentials `json:"app"`
}

type bridgeAppCredentials struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}
