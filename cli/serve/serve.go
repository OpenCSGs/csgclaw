package serve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"csgclaw/cli/command"
	"csgclaw/internal/agent"
	"csgclaw/internal/bot"
	"csgclaw/internal/channel"
	"csgclaw/internal/cliproxy"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
	"csgclaw/internal/modelprovider"
	"csgclaw/internal/sandboxproviders"
	"csgclaw/internal/server"
)

var (
	RunServer          = server.Run
	NewAgentService    = newAgentService
	NewBotService      = newBotService
	NewIMService       = newIMService
	NewFeishuService   = newFeishuService
	NewLLMService      = newLLMService
	CheckModelProvider = checkModelProvider
	EnsureCLIProxy     = func(ctx context.Context) error {
		return cliproxy.Default().EnsureStarted(ctx)
	}
	ShutdownCLIProxy = func(ctx context.Context) error {
		return cliproxy.Default().Shutdown(ctx)
	}
	EnsureBootstrapManager = func(ctx context.Context, svc *agent.Service, forceRecreate bool) error {
		if svc == nil {
			return nil
		}
		return svc.EnsureBootstrapManager(ctx, forceRecreate)
	}
	StartConfiguredAgents = func(ctx context.Context, svc *agent.Service) error {
		if svc == nil {
			return nil
		}
		return svc.StartConfiguredAgents(ctx)
	}
)

type serveCmd struct{}
type stopCmd struct{}
type internalServeCmd struct{}

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

	cfg, err := loadConfig(globals.Config)
	if err != nil {
		return err
	}
	if globals.Endpoint != "" {
		cfg.Server.AdvertiseBaseURL = strings.TrimRight(globals.Endpoint, "/")
	}

	if *daemon {
		return serveBackground(run, cfg, globals, *logPath, *pidPath, *logLevel)
	}
	restore, err := configureServeLogger(run.Stderr, *logLevel)
	if err != nil {
		return err
	}
	defer restore()
	return serveForeground(ctx, run, cfg, globals.Output)
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
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *pidPath != "" {
		if err := writePIDFile(*pidPath, os.Getpid()); err != nil {
			return err
		}
		defer removePIDFile(*pidPath)
	}

	cfg, err := loadConfig(*configPathFlag)
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

	printEffectiveConfig(run, cfg, globals.Output)
	imBus := im.NewBus()
	svc, err := NewAgentService(cfg)
	if err != nil {
		return err
	}
	imSvc, err := NewIMService(imBus)
	if err != nil {
		return err
	}
	botSvc, err := NewBotService()
	if err != nil {
		return err
	}
	feishuSvc, err := NewFeishuService(cfg)
	if err != nil {
		return err
	}
	return startServer(ctx, cfg, svc, botSvc, imSvc, imBus, feishuSvc)
}

func serveForeground(ctx context.Context, run *command.Context, cfg config.Config, output string) error {
	_ = preflightDefaultModelProvider(ctx, cfg)
	imBus := im.NewBus()
	svc, err := NewAgentService(cfg)
	if err != nil {
		return err
	}
	imSvc, err := NewIMService(imBus)
	if err != nil {
		return err
	}
	botSvc, err := NewBotService()
	if err != nil {
		return err
	}
	feishuSvc, err := NewFeishuService(cfg)
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
		fmt.Fprintln(run.Stdout, "Open this URL in your browser after startup.")
	}

	return startServer(ctx, cfg, svc, botSvc, imSvc, imBus, feishuSvc)
}

func serveBackground(run *command.Context, cfg config.Config, globals command.GlobalOptions, logPath, pidPath, logLevel string) error {
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
	cmd := exec.Command(exe, childArgs...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

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

func startServer(ctx context.Context, cfg config.Config, svc *agent.Service, botSvc *bot.Service, imSvc *im.Service, imBus *im.Bus, feishuSvc *channel.FeishuService) error {
	_ = EnsureCLIProxy(ctx)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = ShutdownCLIProxy(shutdownCtx)
	}()
	if botSvc != nil {
		botSvc.SetDependencies(svc, imSvc, feishuSvc)
	}
	llmSvc, err := NewLLMService(cfg, svc)
	if err != nil {
		return err
	}
	return RunServer(server.Options{
		ListenAddr:  cfg.Server.ListenAddr,
		Service:     svc,
		Bot:         botSvc,
		IM:          imSvc,
		IMBus:       imBus,
		PicoClaw:    im.NewPicoClawBridge(cfg.Server.AccessToken),
		Feishu:      feishuSvc,
		LLM:         llmSvc,
		AccessToken: cfg.Server.AccessToken,
		NoAuth:      cfg.Server.NoAuth,
		Context:     ctx,
		OnReady: func() {
			if err := EnsureBootstrapManager(ctx, svc, false); err != nil {
				slog.Warn("bootstrap manager failed to start", "error", err)
			}
			if err := StartConfiguredAgents(ctx, svc); err != nil {
				slog.Warn("some configured agents failed to start", "error", err)
			}
		},
	})
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

func apiBaseURL(server config.ServerConfig) string {
	if server.AdvertiseBaseURL != "" {
		return strings.TrimRight(server.AdvertiseBaseURL, "/")
	}

	port := config.ListenPort(server.ListenAddr)
	if server.ListenAddr == "" {
		return config.DefaultAPIBaseURL()
	}
	host := "127.0.0.1"
	if parsedHost, _, err := net.SplitHostPort(server.ListenAddr); err == nil {
		if parsedHost != "" && parsedHost != "0.0.0.0" && parsedHost != "::" {
			host = parsedHost
		}
	}
	return fmt.Sprintf("http://%s:%s", host, port)
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
	llmCfg := effectiveLLMConfig(cfg)
	content := fmt.Sprintf(`[server]
listen_addr = %q
advertise_base_url = %q
access_token = %q
no_auth = %t

[bootstrap]
manager_image_override = %q

[sandbox]
provider = %q
home_dir_name = %q
`, cfg.Server.ListenAddr, cfg.Server.AdvertiseBaseURL, partiallyMaskSecret(cfg.Server.AccessToken), cfg.Server.NoAuth, cfg.Bootstrap.ManagerImageOverride, cfg.Sandbox.Resolved().Provider, cfg.Sandbox.Resolved().HomeDirName)
	if strings.TrimSpace(cfg.Bootstrap.ManagerImageOverride) == "" {
		content = strings.Replace(content, "[bootstrap]\nmanager_image_override", fmt.Sprintf("[bootstrap]\n# using default image: %q\nmanager_image_override", config.DefaultManagerImage), 1)
	}
	if len(cfg.Sandbox.Resolved().DebianRegistries) > 0 {
		content = strings.Replace(content, "[sandbox]\n", fmt.Sprintf("[sandbox]\ndebian_registries = %s\n", formatModelList(cfg.Sandbox.Resolved().DebianRegistries)), 1)
	}
	content += fmt.Sprintf(`
[models]
default = %q
`, llmCfg.DefaultSelector()) + formatEffectiveProviders(llmCfg)

	if strings.TrimSpace(cfg.Channels.FeishuAdminOpenID) != "" {
		content += fmt.Sprintf(`
[channels.feishu]
admin_open_id = %q
`, cfg.Channels.FeishuAdminOpenID)
	}

	if len(cfg.Channels.Feishu) > 0 {
		names := make([]string, 0, len(cfg.Channels.Feishu))
		for name := range cfg.Channels.Feishu {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			feishu := cfg.Channels.Feishu[name]
			content += fmt.Sprintf(`
[channels.feishu.%s]
app_id = %q
app_secret = %q
`, name, feishu.AppID, partiallyMaskSecret(feishu.AppSecret))
		}
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
		return config.LoadDefault()
	}
	return config.Load(path)
}

func validateModelConfig(cfg config.Config) error {
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

func newAgentService(cfg config.Config) (*agent.Service, error) {
	agentsPath, err := config.DefaultAgentsPath()
	if err != nil {
		return nil, err
	}
	opts, err := sandboxServiceOptions(cfg.Sandbox)
	if err != nil {
		return nil, err
	}
	return agent.NewServiceWithLLMAndChannels(effectiveLLMConfig(cfg), cfg.Server, cfg.Channels, cfg.Bootstrap.EffectiveManagerImage(), agentsPath, opts...)
}

func sandboxServiceOptions(cfg config.SandboxConfig) ([]agent.ServiceOption, error) {
	return sandboxproviders.ServiceOptions(cfg)
}

func newIMService(bus *im.Bus) (*im.Service, error) {
	imStatePath, err := config.DefaultIMStatePath()
	if err != nil {
		return nil, err
	}
	return im.NewServiceFromPathWithBus(imStatePath, bus)
}

func newBotService() (*bot.Service, error) {
	imStatePath, err := config.DefaultIMStatePath()
	if err != nil {
		return nil, err
	}
	store, err := bot.NewStore(filepath.Join(filepath.Dir(imStatePath), "bots.json"))
	if err != nil {
		return nil, err
	}
	return bot.NewService(store)
}

func newFeishuService(cfg config.Config) (*channel.FeishuService, error) {
	return channel.NewFeishuService(feishuAppsFromConfig(cfg.Channels)), nil
}

func feishuAppsFromConfig(cfg config.ChannelsConfig) map[string]channel.FeishuAppConfig {
	apps := make(map[string]channel.FeishuAppConfig, len(cfg.Feishu))
	for name, app := range cfg.Feishu {
		apps[name] = channel.FeishuAppConfig{
			AppID:       app.AppID,
			AppSecret:   app.AppSecret,
			AdminOpenID: cfg.FeishuAdminOpenID,
		}
	}
	return apps
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
