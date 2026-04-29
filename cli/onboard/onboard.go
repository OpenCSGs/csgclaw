package onboard

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/agent"
	"csgclaw/internal/bot"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/sandboxproviders"

	"golang.org/x/term"
)

var (
	CreateManagerBot       = createManagerBot
	EnsureIMBootstrapState = im.EnsureBootstrapState
	isTerminalFD           = term.IsTerminal
)

type cmd struct{}

func NewCmd() command.Command {
	return cmd{}
}

func (cmd) Name() string {
	return "onboard"
}

func (cmd) Summary() string {
	return "Initialize local config and bootstrap state."
}

func (c cmd) Run(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("onboard", run.Program+" onboard [flags]", c.Summary())
	debianRegistries := fs.String("debian-registries", "", "comma-separated OCI registries used for debian:bookworm-slim pulls (persisted to config)")
	logLevel := fs.String("log-level", "info", "log level: debug, info, warn, error")
	if err := fs.Parse(args); err != nil {
		return err
	}

	restore, err := configureOnboardLogger(run.Stderr, *logLevel)
	if err != nil {
		return err
	}
	defer restore()

	path, err := configPath(globals.Config)
	if err != nil {
		return err
	}

	cfg, hasExistingConfig, err := loadOnboardConfig(path)
	if err != nil {
		return err
	}
	if !hasExistingConfig {
		cfg = config.Config{
			Server: config.ServerConfig{
				ListenAddr:  config.DefaultListenAddr(),
				AccessToken: config.DefaultAccessToken,
				NoAuth:      false,
			},
			Bootstrap: config.BootstrapConfig{
				ManagerImage: config.DefaultManagerImage,
			},
			Sandbox: config.SandboxConfig{
				Provider:    config.DefaultSandboxProvider,
				HomeDirName: config.DefaultSandboxHomeDirName,
			},
		}
	}

	if strings.TrimSpace(*debianRegistries) != "" {
		cfg.Sandbox.DebianRegistries = parseRegistriesFlag(*debianRegistries)
	}

	if err := cfg.Save(path); err != nil {
		return err
	}

	agentsPath, err := config.DefaultAgentsPath()
	if err != nil {
		return err
	}
	imStatePath, err := config.DefaultIMStatePath()
	if err != nil {
		return err
	}
	if err := EnsureIMBootstrapState(imStatePath); err != nil {
		return err
	}
	if _, err := CreateManagerBot(ctx, agentsPath, imStatePath, cfg); err != nil {
		return err
	}

	result := command.ActionResult{
		Command:      "onboard",
		Action:       "initialize",
		Status:       "initialized",
		ConfigPath:   path,
		ManagerImage: cfg.Bootstrap.ManagerImage,
		Users:        []string{"admin", "manager"},
		Message:      fmt.Sprintf("initialized config at %s", path),
	}
	if globals.Output == "json" {
		return command.RenderAction(globals.Output, run.Stdout, result)
	}
	fmt.Fprintln(run.Stdout, result.Message)
	fmt.Fprintf(run.Stdout, "ensured bootstrap agent %q with image %q\n", agent.ManagerName, cfg.Bootstrap.ManagerImage)
	fmt.Fprintf(run.Stdout, "ensured IM members %q and %q\n", "admin", "manager")
	fmt.Fprintln(run.Stdout, "cleared IM invite draft data")
	return nil
}

func createManagerBot(ctx context.Context, agentsPath, imStatePath string, cfg config.Config) (bot.Bot, error) {
	opts, err := sandboxServiceOptions(cfg.Sandbox)
	if err != nil {
		return bot.Bot{}, err
	}
	agentSvc, err := agent.NewServiceWithLLMAndChannels(effectiveLLMConfig(cfg), cfg.Server, cfg.Channels, cfg.Bootstrap.ManagerImage, agentsPath, opts...)
	if err != nil {
		return bot.Bot{}, err
	}
	defer func() {
		_ = agentSvc.Close()
	}()

	imSvc, err := im.NewServiceFromPath(imStatePath)
	if err != nil {
		return bot.Bot{}, err
	}
	store, err := bot.NewStore(filepath.Join(filepath.Dir(imStatePath), "bots.json"))
	if err != nil {
		return bot.Bot{}, err
	}
	botSvc, err := bot.NewServiceWithDependencies(store, agentSvc, imSvc)
	if err != nil {
		return bot.Bot{}, err
	}
	return botSvc.CreateManager(ctx, bot.CreateRequest{
		Name:    agent.ManagerName,
		Role:    string(bot.RoleManager),
		Channel: string(bot.ChannelCSGClaw),
	}, false)
}

func sandboxServiceOptions(cfg config.SandboxConfig) ([]agent.ServiceOption, error) {
	return sandboxproviders.ServiceOptions(cfg)
}

func configureOnboardLogger(w io.Writer, level string) (func(), error) {
	parsedLevel, err := parseOnboardLogLevel(level)
	if err != nil {
		return nil, err
	}
	if w == nil {
		w = os.Stderr
	}

	prev := slog.Default()
	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: parsedLevel})
	if isTerminal(w) {
		handler = nil
	}
	var logger *slog.Logger
	if handler != nil {
		logger = slog.New(handler)
	} else {
		logger = slog.New(&onboardTerminalHandler{
			writer: w,
			level:  parsedLevel,
		})
	}
	slog.SetDefault(logger)
	return func() {
		slog.SetDefault(prev)
	}, nil
}

func parseOnboardLogLevel(level string) (slog.Level, error) {
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

type onboardTerminalHandler struct {
	writer io.Writer
	level  slog.Leveler
	attrs  []slog.Attr
	groups []string
}

func (h *onboardTerminalHandler) Enabled(_ context.Context, level slog.Level) bool {
	threshold := slog.LevelInfo
	if h != nil && h.level != nil {
		threshold = h.level.Level()
	}
	return level >= threshold
}

func (h *onboardTerminalHandler) Handle(_ context.Context, record slog.Record) error {
	if h == nil || h.writer == nil {
		return nil
	}

	attrs := make([]slog.Attr, 0, len(h.attrs)+record.NumAttrs())
	attrs = append(attrs, h.attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})

	parts := make([]string, 0, len(attrs)+1)
	parts = append(parts, record.Message)
	for _, attr := range attrs {
		key := qualifyOnboardLogKey(h.groups, attr.Key)
		parts = append(parts, fmt.Sprintf("%s=%s", key, onboardLogAttrValue(attr.Value)))
	}

	_, err := fmt.Fprintf(h.writer, "%s %s\n", strings.ToUpper(record.Level.String()), strings.Join(parts, " "))
	return err
}

func (h *onboardTerminalHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &clone
}

func (h *onboardTerminalHandler) WithGroup(name string) slog.Handler {
	clone := *h
	clone.groups = append(append([]string{}, h.groups...), name)
	return &clone
}

func qualifyOnboardLogKey(groups []string, key string) string {
	if len(groups) == 0 {
		return key
	}
	return strings.Join(append(append([]string{}, groups...), key), ".")
}

func onboardLogAttrValue(value slog.Value) string {
	return fmt.Sprint(value.Any())
}

func isTerminal(value any) bool {
	file, ok := value.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	return isTerminalFD(int(file.Fd()))
}

func loadOnboardConfig(path string) (config.Config, bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return config.Config{}, false, nil
		}
		return config.Config{}, false, fmt.Errorf("stat config: %w", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, false, err
	}
	return cfg, true, nil
}

func configPath(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	return config.DefaultPath()
}

func effectiveLLMConfig(cfg config.Config) config.LLMConfig {
	if !cfg.Models.IsZero() {
		return cfg.Models.Normalized()
	}
	if !cfg.LLM.IsZero() {
		return cfg.LLM.Normalized()
	}
	return config.SingleProfileLLM(cfg.Model).Normalized()
}

func parseRegistriesFlag(raw string) []string {
	values := strings.Split(raw, ",")
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
