package serve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"csgclaw/cli/command"
	"csgclaw/internal/agent"
	"csgclaw/internal/api"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/app/channelwiring"
	"csgclaw/internal/app/runtimewiring"
	csgclawchannel "csgclaw/internal/channel/csgclaw"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/channel/feishu/participantprovider"
	"csgclaw/internal/channelbridge"
	"csgclaw/internal/channelbridge/codexbridge"
	"csgclaw/internal/channelbridge/codexmanager"
	"csgclaw/internal/cliproxy"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
	"csgclaw/internal/localstore"
	"csgclaw/internal/modelprovider"
	internalonboard "csgclaw/internal/onboard"
	"csgclaw/internal/participant"
	runtimecodex "csgclaw/internal/runtime/codex"
	"csgclaw/internal/sandboxproviders"
	"csgclaw/internal/server"
	"csgclaw/internal/team"
	hub "csgclaw/internal/template"
	"csgclaw/internal/upgrade"
	appversion "csgclaw/internal/version"
)

var (
	RunServer                 = server.Run
	NewAgentService           = newAgentService
	NewIMService              = newIMService
	NewFeishuService          = newFeishuService
	NewLLMService             = newLLMService
	NewTeamService            = newTeamService
	CheckModelProvider        = checkModelProvider
	CheckCatalogModelProvider = agent.CheckModelProvider
	EnsureCLIProxy            = func(ctx context.Context) error {
		return cliproxy.Default().EnsureStarted(ctx)
	}
	ShutdownCLIProxy = func(ctx context.Context) error {
		return cliproxy.Default().Shutdown(ctx)
	}
	DetectBootstrapState   = internalonboard.DetectState
	EnsureBootstrapState   = internalonboard.EnsureState
	EnsureBootstrapManager = func(ctx context.Context, svc *agent.Service) error {
		if svc == nil {
			return nil
		}
		return svc.EnsureBootstrapManager(ctx, false)
	}
	StartConfiguredAgents = func(ctx context.Context, svc *agent.Service) error {
		if svc == nil {
			return nil
		}
		return svc.StartConfiguredAgents(ctx)
	}
	NewCodexBridgeManager = newCodexBridgeManager
	OpenBrowser           = openBrowser
	WaitForHealthy        = waitForHealthy
)

type serveCmd struct{}
type stopCmd struct{}
type internalServeCmd struct{}

const cliproxyAutoLoginEnv = "CSGCLAW_CLIPROXY_AUTO_LOGIN"

func NewServeCmd() command.Command {
	return serveCmd{}
}

func NewStopCmd() command.Command {
	return stopCmd{}
}

func NewInternalServeCmd() command.Command {
	return internalServeCmd{}
}

func (serveCmd) Name() string {
	return "serve"
}

func (serveCmd) Summary() string {
	return "Start the local HTTP server."
}

func (c serveCmd) Run(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("serve", run.Program+" serve [-d|--daemon] [flags]", c.Summary())
	daemon := fs.Bool("daemon", false, "run server in background")
	fs.BoolVar(daemon, "d", false, "run server in background")
	noBrowser := fs.Bool("no-browser", false, "do not open the browser after startup")
	noAuthDetect := fs.Bool("no-auth-detect", false, "disable automatic auth detection during startup")
	logLevel := fs.String("log-level", "info", "log level: debug, info, warn, error")

	defaultLogPath, err := defaultServerLogPath()
	if err != nil {
		return err
	}
	defaultPIDPath, err := defaultServerPIDPath()
	if err != nil {
		return err
	}
	logPath := fs.String("log", defaultLogPath, "log file path, daemon mode only")
	pidPath := fs.String("pid", defaultPIDPath, "pid file path, daemon mode only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := validateServeInstallation(); err != nil {
		return err
	}
	restoreAuthDetect := applyNoAuthDetectEnv(*noAuthDetect)
	defer restoreAuthDetect()

	restore, err := configureServeLogger(run.Stderr, *logLevel)
	if err != nil {
		return err
	}
	defer restore()

	if err := migrateLocalStore(); err != nil {
		return err
	}
	if err := ensureServeBootstrapState(ctx, globals.Config, *noAuthDetect); err != nil {
		return err
	}

	configPath := strings.TrimSpace(globals.Config)
	if configPath == "" {
		configPath, err = config.DefaultPath()
		if err != nil {
			return err
		}
	}
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	if globals.Endpoint != "" {
		cfg.Server.AdvertiseBaseURL = strings.TrimRight(globals.Endpoint, "/")
	}

	if *daemon {
		serveGlobals := globals
		serveGlobals.Config = configPath
		return serveBackground(run, cfg, serveGlobals, *logPath, *pidPath, *logLevel, *noBrowser, *noAuthDetect)
	}
	return serveForegroundWithConfigPath(ctx, run, cfg, configPath, globals.Output, serveOptions{
		NoBrowser:    *noBrowser,
		NoAuthDetect: *noAuthDetect,
	})
}

func (stopCmd) Name() string {
	return "stop"
}

func (stopCmd) Summary() string {
	return "Stop the local HTTP server."
}

func (c stopCmd) Run(_ context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("stop", run.Program+" stop [flags]", c.Summary())
	defaultPIDPath, err := defaultServerPIDPath()
	if err != nil {
		return err
	}
	pidPath := fs.String("pid", defaultPIDPath, "pid file path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	pid, err := readPIDFile(*pidPath)
	if err != nil {
		return err
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			removePIDFile(*pidPath)
			return command.RenderAction(globals.Output, run.Stdout, command.ActionResult{
				Command: "stop",
				Action:  "stop",
				Status:  "stale_pid_removed",
				PID:     pid,
				PIDPath: *pidPath,
				Message: fmt.Sprintf("removed stale pid file %s", *pidPath),
			})
		}
		return fmt.Errorf("signal process %d: %w", pid, err)
	}
	return command.RenderAction(globals.Output, run.Stdout, command.ActionResult{
		Command: "stop",
		Action:  "stop",
		Status:  "signaled",
		PID:     pid,
		PIDPath: *pidPath,
		Message: fmt.Sprintf("sent SIGTERM to server process %d", pid),
	})
}

func (internalServeCmd) Name() string {
	return "_serve"
}

func (internalServeCmd) Summary() string {
	return "Internal server entrypoint."
}

func (internalServeCmd) Hidden() bool {
	return true
}

func (c internalServeCmd) Run(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("_serve", run.Program+" _serve [flags]", c.Summary())
	pidPath := fs.String("pid", "", "pid file path")
	configPathFlag := fs.String("config", globals.Config, "config file path")
	logLevel := fs.String("log-level", "info", "log level: debug, info, warn, error")
	noBrowser := fs.Bool("no-browser", false, "do not open the browser after startup")
	noAuthDetect := fs.Bool("no-auth-detect", false, "disable automatic auth detection during startup")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := validateServeInstallation(); err != nil {
		return err
	}
	restoreAuthDetect := applyNoAuthDetectEnv(*noAuthDetect)
	defer restoreAuthDetect()

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *pidPath != "" {
		if err := writePIDFile(*pidPath, os.Getpid()); err != nil {
			return err
		}
		defer removePIDFile(*pidPath)
	}

	configPath := strings.TrimSpace(*configPathFlag)
	var err error
	if configPath == "" {
		configPath, err = config.DefaultPath()
		if err != nil {
			return err
		}
	}
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	if globals.Endpoint != "" {
		cfg.Server.AdvertiseBaseURL = strings.TrimRight(globals.Endpoint, "/")
	}
	restore, err := configureServeLogger(run.Stderr, *logLevel)
	if err != nil {
		return err
	}
	defer restore()
	_ = preflightDefaultModelProvider(ctx, cfg)
	if err := migrateLocalStore(); err != nil {
		return err
	}

	printEffectiveConfig(run, cfg, globals.Output)
	imBus := im.NewBus()
	feishuProvider, feishuSvc, err := buildFeishuComponents()
	if err != nil {
		return err
	}
	svc, err := NewAgentService(cfg, feishuProvider)
	if err != nil {
		return err
	}
	imSvc, err := NewIMService(imBus)
	if err != nil {
		return err
	}
	return startServerWithConfigPath(ctx, run, cfg, svc, imSvc, imBus, feishuSvc, configPath, globals.Output, serveOptions{
		NoBrowser:    *noBrowser,
		NoAuthDetect: *noAuthDetect,
	})
}

func serveForeground(ctx context.Context, run *command.Context, cfg config.Config, output string) error {
	return serveForegroundWithConfigPath(ctx, run, cfg, "", output)
}

func validateServeInstallation() error {
	currentVersion := appversion.Current()
	support := (upgrade.Client{}).AutoUpgradeSupport(currentVersion)
	if support.Supported || support.Reason == "local_build" {
		return nil
	}
	platform, command := officialInstallGuidance(runtime.GOOS)
	return fmt.Errorf("refuse to start release %s: %w\nInstall the official bundle on %s with:\n  %s\n拒绝启动正式版本 %s：当前程序并非从官方 CSGClaw bundle 安装。\n请在 %s 上执行：\n  %s", currentVersion, upgrade.ErrNotOfficialBundle, platform, command, currentVersion, platform, command)
}

func officialInstallGuidance(goos string) (string, string) {
	if goos == "windows" {
		return "Windows", "curl.exe -fsSL https://csgclaw.opencsg.com/install.ps1 | powershell -ExecutionPolicy Bypass -Command -"
	}
	return "macOS/Linux", "curl -fsSL https://csgclaw.opencsg.com/install.sh | bash"
}

type serveOptions struct {
	NoBrowser    bool
	NoAuthDetect bool
}

func serveForegroundWithConfigPath(ctx context.Context, run *command.Context, cfg config.Config, configPath string, output string, opts ...serveOptions) error {
	if err := validateServeInstallation(); err != nil {
		return err
	}
	_ = preflightDefaultModelProvider(ctx, cfg)
	if err := migrateLocalStore(); err != nil {
		return err
	}
	imBus := im.NewBus()
	feishuProvider, feishuSvc, err := buildFeishuComponents()
	if err != nil {
		return err
	}
	svc, err := NewAgentService(cfg, feishuProvider)
	if err != nil {
		return err
	}
	imSvc, err := NewIMService(imBus)
	if err != nil {
		return err
	}
	apiURL := apiBaseURL(cfg.Server)
	imURL := imOpenURL(apiURL)

	if output == "json" {
		if err := command.RenderAction(output, run.Stdout, command.ActionResult{
			Command:         "serve",
			Action:          "start",
			Status:          "starting",
			IMURL:           imURL,
			APIURL:          apiURL,
			EffectiveConfig: formatEffectiveConfig(cfg),
		}); err != nil {
			return err
		}
	} else {
		printEffectiveConfig(run, cfg, output)
		fmt.Fprintf(run.Stdout, "CSGClaw IM is available at: %s\n", imURL)
	}

	return startServerWithConfigPath(ctx, run, cfg, svc, imSvc, imBus, feishuSvc, configPath, output, opts...)
}

func serveBackground(run *command.Context, cfg config.Config, globals command.GlobalOptions, logPath, pidPath, logLevel string, noBrowser, noAuthDetect bool) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()

	childArgs := []string{"_serve", "--pid", pidPath}
	if globals.Config != "" {
		childArgs = append(childArgs, "--config", globals.Config)
	}
	if strings.TrimSpace(logLevel) != "" {
		childArgs = append(childArgs, "--log-level", logLevel)
	}
	if noBrowser {
		childArgs = append(childArgs, "--no-browser")
	}
	if noAuthDetect {
		childArgs = append(childArgs, "--no-auth-detect")
	}
	cmd := exec.Command(exe, childArgs...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = backgroundServeSysProcAttr()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	apiURL := apiBaseURL(cfg.Server)
	if err := waitForHealthy(apiURL, 5*time.Second); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("server process started (pid %d) but health check failed: %w; see %s", cmd.Process.Pid, err, logPath)
	}

	result := command.ActionResult{
		Command: "serve",
		Action:  "start",
		Status:  "started",
		PID:     cmd.Process.Pid,
		IMURL:   imOpenURL(apiURL),
		APIURL:  apiURL,
		LogPath: logPath,
		PIDPath: pidPath,
		Message: fmt.Sprintf("server started in background (pid %d)", cmd.Process.Pid),
	}
	if globals.Output == "json" {
		return command.RenderAction(globals.Output, run.Stdout, result)
	}

	fmt.Fprintln(run.Stdout, result.Message)
	fmt.Fprintf(run.Stdout, "im: %s\n", result.IMURL)
	fmt.Fprintf(run.Stdout, "api: %s\n", result.APIURL)
	fmt.Fprintf(run.Stdout, "log: %s\n", result.LogPath)
	fmt.Fprintf(run.Stdout, "pid: %s\n", result.PIDPath)
	return nil
}

func applyNoAuthDetectEnv(disabled bool) func() {
	if !disabled {
		return func() {}
	}
	previous, hadPrevious := os.LookupEnv(cliproxyAutoLoginEnv)
	_ = os.Setenv(cliproxyAutoLoginEnv, "0")
	return func() {
		if hadPrevious {
			_ = os.Setenv(cliproxyAutoLoginEnv, previous)
			return
		}
		_ = os.Unsetenv(cliproxyAutoLoginEnv)
	}
}

func ensureServeBootstrapState(ctx context.Context, configPath string, noAuthDetect bool) error {
	state, err := DetectBootstrapState(internalonboard.DetectStateOptions{ConfigPath: configPath})
	if err != nil {
		return err
	}
	if state.Complete() {
		return nil
	}

	slog.Info("bootstrap state incomplete; auto-initializing local state", "config_path", state.ConfigPath)
	_, err = EnsureBootstrapState(ctx, internalonboard.EnsureStateOptions{
		ConfigPath:   configPath,
		NoAuthDetect: noAuthDetect,
	})
	return err
}

func configureServeLogger(w io.Writer, level string) (func(), error) {
	parsedLevel, err := parseServeLogLevel(level)
	if err != nil {
		return nil, err
	}
	if w == nil {
		w = os.Stderr
	}

	prev := slog.Default()
	logger := slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: parsedLevel}))
	slog.SetDefault(logger)
	return func() {
		slog.SetDefault(prev)
	}, nil
}

func parseServeLogLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", level)
	}
}

func startServer(ctx context.Context, run *command.Context, cfg config.Config, svc *agent.Service, imSvc *im.Service, imBus *im.Bus, feishuSvc *feishu.Service, output string) error {
	return startServerWithConfigPath(ctx, run, cfg, svc, imSvc, imBus, feishuSvc, "", output)
}

func startServerWithConfigPath(ctx context.Context, run *command.Context, cfg config.Config, svc *agent.Service, imSvc *im.Service, imBus *im.Bus, feishuSvc *feishu.Service, configPath, output string, opts ...serveOptions) error {
	serveOpts := serveOptions{}
	if len(opts) > 0 {
		serveOpts = opts[0]
	}
	restoreAuthDetect := applyNoAuthDetectEnv(serveOpts.NoAuthDetect)
	defer restoreAuthDetect()
	_ = EnsureCLIProxy(ctx)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = ShutdownCLIProxy(shutdownCtx)
	}()
	if serveOpts.NoAuthDetect && svc != nil {
		svc.SetStartupProfileDetectionDisabled(true)
	}
	if refreshed, results, err := refreshStartupModelProviders(ctx, cfg, configPath, svc); err != nil {
		slog.Warn("refresh model provider catalog at startup failed", "error", err)
	} else {
		cfg = refreshed
		logStartupModelProviderRefresh(results)
	}
	codexBridgeMgr, err := NewCodexBridgeManager(cfg, svc, feishuSvc)
	if err != nil {
		return err
	}
	if svc != nil {
		svc.SetLifecycleObserver(codexBridgeMgr)
	}
	if codexBridgeMgr != nil {
		defer codexBridgeMgr.Close()
	}
	participantSvc, err := newParticipantService(svc, imSvc)
	if err != nil {
		return err
	}
	llmSvc, err := NewLLMService(cfg, svc)
	if err != nil {
		return err
	}
	apiURL := apiBaseURL(cfg.Server)
	imURL := imOpenURL(apiURL)
	currentVersion := appversion.Current()
	upgradeClient := upgrade.Client{
		HTTPClient: http.DefaultClient,
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
	}
	upgradeSupport := upgradeClient.AutoUpgradeSupport(currentVersion)
	upgradeManager := upgrade.NewManager(upgradeClient, currentVersion, upgrade.ManagerOptions{
		AutoUpgradeSupported:         upgradeSupport.Supported,
		AutoUpgradeUnsupportedReason: upgradeSupport.Reason,
		OnStatusChange: func(status apitypes.UpgradeStatus) {
			if imBus == nil {
				return
			}
			snapshot := status
			if snapshot.LastCheckedAt != nil {
				t := *snapshot.LastCheckedAt
				snapshot.LastCheckedAt = &t
			}
			imBus.Publish(im.Event{
				Type:    im.EventTypeUpgradeStatusChanged,
				Upgrade: &snapshot,
			})
		},
	})
	configureFeishuService(feishuSvc, svc)
	if outcome, err := upgrade.ConsumeApplyStatus(configPath); err != nil {
		slog.Warn("load upgrade helper failure", "error", err)
	} else if outcome.Status == upgrade.ApplyStatusFailed && outcome.Message != "" {
		upgradeManager.MarkUpgradeFailed(errors.New(outcome.Message))
	}
	hubSvc, err := newAgentTemplateHubService(cfg.Hub)
	if err != nil {
		return err
	}
	teamSvc, teamAdapter, err := NewTeamService(imSvc, participantSvc)
	if err != nil {
		return err
	}
	ensureBootstrapManager := EnsureBootstrapManager
	startConfiguredAgents := StartConfiguredAgents
	return RunServer(server.Options{
		ListenAddr:        cfg.Server.ListenAddr,
		Service:           svc,
		Hub:               hubSvc,
		Participant:       participantSvc,
		IM:                imSvc,
		IMBus:             imBus,
		ParticipantBridge: im.NewParticipantBridge(cfg.Server.AccessToken),
		Feishu:            feishuSvc,
		LLM:               llmSvc,
		Team:              teamSvc,
		TeamAdapter:       teamAdapter,
		Upgrade:           upgradeManager,
		ActivityDecider:   channelActivityDecider(codexBridgeMgr),
		ConfigPath:        configPath,
		AccessToken:       cfg.Server.AccessToken,
		NoAuth:            cfg.Server.NoAuth,
		Context:           ctx,
		OnReady: func(handler *api.Handler, router chi.Router) {
			deliver := channelwiring.WireNotificationParticipantPull(ctx, participantSvc, imSvc, apiURL, cfg.Server.AccessToken)
			handler.SetNotificationDeliver(deliver)
			if !serveOpts.NoBrowser && output != "json" && run != nil {
				go func() {
					if err := WaitForHealthy(apiURL, 5*time.Second); err != nil {
						fmt.Fprintln(run.Stdout, "Open this URL in your browser after startup.")
						return
					}
					if err := OpenBrowser(imURL); err != nil {
						fmt.Fprintln(run.Stdout, "Open this URL in your browser after startup.")
					} else {
						fmt.Fprintln(run.Stdout, "Opened this URL in your browser.")
					}
				}()
			}
			go func() {
				if err := ensureBootstrapManager(ctx, svc); err != nil {
					slog.Warn("bootstrap manager failed to start", "error", err)
				}
				if err := startConfiguredAgents(ctx, svc); err != nil {
					slog.Warn("some configured agents failed to start", "error", err)
				}
				if codexBridgeMgr != nil {
					if err := codexBridgeMgr.Start(ctx); err != nil {
						slog.Warn("some codex bridges failed to start", "error", err)
					}
				}
			}()
		},
	})
}

func refreshStartupModelProviders(ctx context.Context, cfg config.Config, configPath string, svc *agent.Service) (config.Config, []agent.ModelProviderCheckResult, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return cfg, nil, nil
	}
	llmCfg := cfg.Models
	if llmCfg.IsZero() && !cfg.LLM.IsZero() {
		llmCfg = cfg.LLM
	}
	updatedLLM, results, changed := agent.RefreshModelProviderCatalog(ctx, llmCfg, CheckCatalogModelProvider)
	if !changed {
		return cfg, results, nil
	}
	cfg.Models = updatedLLM
	cfg.LLM = updatedLLM
	if err := cfg.Save(configPath); err != nil {
		return cfg, results, err
	}
	if svc != nil {
		svc.SetLLMConfig(updatedLLM)
	}
	return cfg, results, nil
}

func logStartupModelProviderRefresh(results []agent.ModelProviderCheckResult) {
	for _, result := range results {
		if result.Status != agent.ModelProviderStatusConnected || len(result.Models) == 0 {
			continue
		}
		slog.Info("refreshed model provider catalog", "provider", result.ID, "models", len(result.Models))
	}
}

func configureFeishuService(feishuSvc *feishu.Service, svc *agent.Service) {
	if feishuSvc == nil {
		return
	}
	provider := feishuSvc.ConfigProvider()
	if provider == nil {
		slog.Warn("skip feishu runtime wiring: provider is not configured")
		return
	}
	runtimewiring.UpdatePicoClawFeishuProvider(svc, provider)
	runtimewiring.UpdateOpenClawFeishuProvider(svc, provider)
}

func preflightDefaultModelProvider(ctx context.Context, cfg config.Config) error {
	if effectiveLLMConfig(cfg).IsZero() {
		return nil
	}
	llmCfg := effectiveLLMConfig(cfg)
	providerName := llmCfg.EffectiveDefaultProvider()
	_, modelCfg, err := llmCfg.Resolve("")
	if err != nil {
		return nil
	}
	if !requiresCSGHubLitePreflight(providerName, modelCfg.BaseURL) {
		return nil
	}
	if err := CheckModelProvider(ctx, modelCfg); err != nil {
		return fmt.Errorf("csghub-lite provider is not reachable at %s (%w); start it with `csghub-lite run <model>` or `csghub-lite serve`, then retry", strings.TrimRight(modelCfg.BaseURL, "/"), err)
	}
	return nil
}

func checkModelProvider(ctx context.Context, modelCfg config.ModelConfig) error {
	_, err := modelprovider.ListOpenAIModels(ctx, modelCfg.BaseURL, modelCfg.APIKey)
	return err
}

func requiresCSGHubLitePreflight(providerName, baseURL string) bool {
	if strings.TrimSpace(providerName) == modelprovider.CSGHubLiteProviderName {
		return true
	}
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	return port == "11435" && (host == "127.0.0.1" || host == "localhost")
}

func defaultServerLogPath() (string, error) {
	dir, err := config.DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "server.log"), nil
}

func defaultServerPIDPath() (string, error) {
	dir, err := config.DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "server.pid"), nil
}

func writePIDFile(path string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create pid dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	return nil
}

func readPIDFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read pid file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse pid file: %w", err)
	}
	if pid <= 0 {
		return 0, fmt.Errorf("parse pid file: invalid pid %d", pid)
	}
	return pid, nil
}

func removePIDFile(path string) {
	_ = os.Remove(path)
}

func waitForHealthy(apiBaseURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: 1 * time.Second}
	deadline := time.Now().Add(timeout)
	url := strings.TrimRight(apiBaseURL, "/") + "/healthz"

	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return nil
		}
		if err == nil {
			lastErr = fmt.Errorf("status %s", resp.Status)
			_ = resp.Body.Close()
		} else {
			lastErr = err
		}
		time.Sleep(200 * time.Millisecond)
	}

	if lastErr == nil {
		lastErr = errors.New("timed out")
	}
	return lastErr
}

func imOpenURL(apiBaseURL string) string {
	return strings.TrimRight(apiBaseURL, "/") + "/"
}

func openBrowser(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("open browser: empty URL")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}

func apiBaseURL(server config.ServerConfig) string {
	return config.ResolveAdvertiseBaseURL(server)
}

func printEffectiveConfig(run *command.Context, cfg config.Config, output string) {
	if output == "json" {
		_ = command.RenderAction(output, run.Stdout, command.ActionResult{
			Command:         "_serve",
			Action:          "start",
			Status:          "starting",
			EffectiveConfig: formatEffectiveConfig(cfg),
		})
		return
	}
	fmt.Fprintf(run.Stdout, "effective config:\n%s", formatEffectiveConfig(cfg))
}

func formatEffectiveConfig(cfg config.Config) string {
	resolvedHub := cfg.Hub.Resolved()
	content := fmt.Sprintf(`[server]
listen_addr = %q
advertise_base_url = %q
access_token = %q
no_auth = %t
show_upgrade = %t

[bootstrap]
default_manager_template = %q
default_worker_template = %q

[sandbox]
provider = %q
`, cfg.Server.ListenAddr, cfg.Server.AdvertiseBaseURL, partiallyMaskSecret(cfg.Server.AccessToken), cfg.Server.NoAuth, cfg.Server.ShowUpgrade, cfg.Bootstrap.ResolvedDefaultManagerTemplate(), cfg.Bootstrap.ResolvedDefaultWorkerTemplate(), cfg.Sandbox.Resolved().Provider)
	if len(cfg.Sandbox.Resolved().DebianRegistriesOverride) > 0 {
		content += fmt.Sprintf("debian_registries_override = %s\n", formatModelList(cfg.Sandbox.Resolved().DebianRegistriesOverride))
	} else {
		content += fmt.Sprintf("# using default debian registries: %s\ndebian_registries_override = []\n", formatModelList(config.DefaultDebianRegistries))
	}
	content += fmt.Sprintf(`
[hub]
default_registry = %q
default_publish_registry = %q
`, resolvedHub.DefaultRegistry, resolvedHub.DefaultPublishRegistry)
	for _, registry := range resolvedHub.Registries {
		content += fmt.Sprintf(`
[[hub.registries]]
name = %q
kind = %q
`, registry.Name, registry.Kind)
		if registry.Path != "" {
			content += fmt.Sprintf("path = %q\n", registry.Path)
		}
		if registry.URL != "" {
			content += fmt.Sprintf("url = %q\n", registry.URL)
		}
		if registry.Token != "" {
			content += fmt.Sprintf("token = %q\n", partiallyMaskSecret(registry.Token))
		}
		content += fmt.Sprintf("enabled = %t\n", registry.Enabled)
	}

	return content
}

func partiallyMaskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return strings.Repeat("*", len(value))
	}
	return value[:2] + strings.Repeat("*", len(value)-4) + value[len(value)-2:]
}

func loadConfig(path string) (config.Config, error) {
	if path == "" {
		defaultPath, err := config.DefaultPath()
		if err != nil {
			return config.Config{}, err
		}
		path = defaultPath
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, err
	}
	if cfg.NeedsMigrationRewrite() {
		if err := cfg.Save(path); err != nil {
			return config.Config{}, err
		}
	}
	return cfg, nil
}

func validateModelConfig(cfg config.Config) error {
	if err := cfg.Bootstrap.Validate(); err != nil {
		return err
	}
	if effectiveLLMConfig(cfg).IsZero() {
		return nil
	}
	if err := effectiveLLMConfig(cfg).Validate(); err != nil {
		var validationErr *config.ModelValidationError
		if errors.As(err, &validationErr) && len(validationErr.MissingFields) > 0 {
			_ = validationErr
			return nil
		}
		return nil
	}
	return nil
}

func missingModelFlags(fields []string) []string {
	flags := make([]string, 0, len(fields))
	for _, field := range fields {
		switch field {
		case "base_url":
			flags = append(flags, "--base-url")
		case "api_key":
			flags = append(flags, "--api-key")
		case "model_id":
			flags = append(flags, "--models")
		case "default", "default_profile":
			flags = append(flags, "--models")
		default:
			flags = append(flags, field)
		}
	}
	return flags
}

func newAgentService(cfg config.Config, feishuProvider feishu.AgentCredentialProvider) (*agent.Service, error) {
	agentsPath, err := config.DefaultAgentsPath()
	if err != nil {
		return nil, err
	}
	hubSvc, err := newAgentTemplateHubService(cfg.Hub)
	if err != nil {
		return nil, err
	}
	bootstrapDefaults, err := hub.ResolveBootstrapDefaults(context.Background(), cfg.Bootstrap, hubSvc)
	if err != nil {
		return nil, err
	}
	opts, err := sandboxServiceOptions(cfg.Sandbox)
	if err != nil {
		return nil, err
	}
	opts = append(opts,
		runtimewiring.WithPicoClawSandboxRuntime(feishuProvider),
		runtimewiring.WithOpenClawSandboxRuntime(feishuProvider),
		runtimewiring.WithCodexRuntime(),
		agent.WithGatewayRuntime(bootstrapDefaults.ManagerRuntimeKind),
		agent.WithBootstrapDefaultTemplates(cfg.Bootstrap),
	)
	if hubSvc != nil {
		opts = append(opts, agent.WithHubService(hubSvc))
	}
	return agent.NewServiceWithLLM(effectiveLLMConfig(cfg), cfg.Server, bootstrapDefaults.ManagerImage, agentsPath, opts...)
}

func newAgentTemplateHubService(cfg config.HubConfig) (*hub.Service, error) {
	return hub.NewService(cfg.Resolved(), hub.DefaultStoreFactory)
}

type codexBridgeManager = codexmanager.Manager

func channelActivityDecider(m codexBridgeManager) api.ActivityDecider {
	withPermissions, ok := m.(interface {
		PermissionDecider() runtimecodex.PermissionDecider
	})
	if !ok {
		return nil
	}
	decider := withPermissions.PermissionDecider()
	if decider == nil {
		return nil
	}
	return runtimecodex.NewPermissionActivityDecider(csgclawchannel.ChannelID, decider)
}

func newCodexBridgeManager(cfg config.Config, svc *agent.Service, feishuSvc *feishu.Service) (codexBridgeManager, error) {
	if svc == nil {
		return nil, nil
	}
	opts := codexmanager.Options{
		Agents:    svc,
		Runtimes:  svc,
		Restarter: svc,
		CSGClawClient: &codexbridge.HTTPClient{
			BaseURL:     apiBaseURL(cfg.Server),
			Token:       cfg.Server.AccessToken,
			MentionOnly: true,
		},
	}
	if feishuSvc != nil {
		if provider := feishuSvc.ConfigProvider(); provider != nil {
			opts.FeishuClient = channelbridge.NewFeishuClient(feishuSvc)
			opts.FeishuProvider = provider
		}
	}
	return codexmanager.New(opts)
}

func sandboxServiceOptions(cfg config.SandboxConfig) ([]agent.ServiceOption, error) {
	return sandboxproviders.ServiceOptions(cfg)
}

func migrateLocalStore() error {
	root, err := config.DefaultDir()
	if err != nil {
		return err
	}
	if localstore.NeedsTypedIDMigration(root) {
		if _, err := localstore.MigrateTypedIDs(localstore.MigrateOptions{Root: root}); err != nil {
			return fmt.Errorf("migrate local store: %w", err)
		}
	}
	if err := localstore.ReconcileTypedAgentDirs(root); err != nil {
		return fmt.Errorf("reconcile local store agent dirs: %w", err)
	}
	return nil
}

func newIMService(bus *im.Bus) (*im.Service, error) {
	imStatePath, err := config.DefaultIMStatePath()
	if err != nil {
		return nil, err
	}
	return im.NewServiceFromPathWithBus(imStatePath, bus)
}

func newParticipantService(agentSvc *agent.Service, imSvc *im.Service) (*participant.Service, error) {
	participantsPath, err := defaultParticipantsPath()
	if err != nil {
		return nil, err
	}
	store, err := participant.NewStore(participantsPath)
	if err != nil {
		return nil, err
	}
	return participant.NewService(
		store,
		participant.WithAgentService(agentSvc),
		participant.WithIMService(imSvc),
	), nil
}

func newTeamService(imSvc *im.Service, participantSvc *participant.Service) (*team.Service, team.TeamChannelAdapter, error) {
	teamsDir, err := config.DefaultTeamsDir()
	if err != nil {
		return nil, nil, err
	}
	store, err := team.NewStore(teamsDir)
	if err != nil {
		return nil, nil, err
	}
	adapter := team.NewCSGClawAdapter(imSvc, participantSvc)
	projector := team.NewProjector(adapter, nil)
	return team.NewService(team.WithStore(store), team.WithProjector(projector)), adapter, nil
}

func buildFeishuComponents() (feishu.AgentCredentialProvider, *feishu.Service, error) {
	participantsPath, err := defaultParticipantsPath()
	if err != nil {
		return nil, nil, err
	}
	provider := participantprovider.New(participantsPath)
	svc, err := NewFeishuService(provider)
	if err != nil {
		return nil, nil, err
	}
	return provider, svc, nil
}

func newFeishuService(provider feishu.Provider) (*feishu.Service, error) {
	return feishu.NewServiceWithProvider(provider), nil
}

func defaultParticipantsPath() (string, error) {
	return config.DefaultStatePath()
}

func newLLMService(cfg config.Config, svc *agent.Service) (*llm.Service, error) {
	if svc == nil {
		return nil, nil
	}
	_, modelCfg, err := effectiveLLMConfig(cfg).Resolve("")
	if err != nil {
		modelCfg = config.ModelConfig{}
	}
	return llm.NewService(modelCfg, svc), nil
}

func effectiveLLMConfig(cfg config.Config) config.LLMConfig {
	if !cfg.Models.IsZero() {
		return cfg.Models.Normalized()
	}
	if !cfg.LLM.IsZero() {
		return cfg.LLM.Normalized()
	}
	return config.SingleProfileLLM(cfg.Model)
}

func formatEffectiveProviders(llmCfg config.LLMConfig) string {
	llmCfg = llmCfg.Normalized()
	var b strings.Builder
	for _, name := range sortedProviderNames(llmCfg.Providers) {
		provider := llmCfg.Providers[name].Resolved()
		fmt.Fprintf(&b, `
[models.providers.%s]
base_url = %q
api_key = %q
models = %s
`, name, provider.BaseURL, partiallyMaskSecret(provider.APIKey), formatModelList(provider.Models))
		if provider.ReasoningEffort != "" {
			fmt.Fprintf(&b, "reasoning_effort = %q\n", provider.ReasoningEffort)
		}
	}
	return b.String()
}

func sortedProviderNames(providers map[string]config.ProviderConfig) []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func formatModelList(models []string) string {
	if len(models) == 0 {
		return "[]"
	}
	quoted := make([]string, 0, len(models))
	for _, modelID := range models {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		quoted = append(quoted, strconv.Quote(modelID))
	}
	if len(quoted) == 0 {
		return "[]"
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
