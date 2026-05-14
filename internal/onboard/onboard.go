package onboard

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/app/runtimewiring"
	"csgclaw/internal/bot"
	"csgclaw/internal/config"
	"csgclaw/internal/hub"
	"csgclaw/internal/im"
	"csgclaw/internal/sandboxproviders"
)

var (
	CreateManagerBot       = createManagerBot
	EnsureIMBootstrapState = im.EnsureBootstrapState
	defaultAgentsPath      = config.DefaultAgentsPath
	defaultIMStatePath     = config.DefaultIMStatePath
)

type EnsureStateOptions struct {
	ConfigPath string
}

type EnsureStateResult struct {
	ConfigPath string
	Config     config.Config
}

func EnsureState(ctx context.Context, opts EnsureStateOptions) (EnsureStateResult, error) {
	path, err := configPath(opts.ConfigPath)
	if err != nil {
		return EnsureStateResult{}, err
	}

	cfg, err := ensureConfigState(path)
	if err != nil {
		return EnsureStateResult{}, err
	}
	if err := ensureBootstrapState(ctx, cfg); err != nil {
		return EnsureStateResult{}, err
	}

	return EnsureStateResult{
		ConfigPath: path,
		Config:     cfg,
	}, nil
}

func ensureConfigState(path string) (config.Config, error) {
	cfg, hasExistingConfig, err := loadConfig(path)
	if err != nil {
		return config.Config{}, err
	}
	existingContent := ""
	if hasExistingConfig {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return config.Config{}, fmt.Errorf("read config: %w", readErr)
		}
		existingContent = string(data)
	}
	if !hasExistingConfig {
		cfg = defaultConfig()
	}
	needsSave := !hasExistingConfig
	if hasExistingConfig && configNeedsCompletion(existingContent) {
		needsSave = true
	}
	if needsSave {
		if err := cfg.Save(path); err != nil {
			return config.Config{}, err
		}
	}
	return cfg, nil
}

func ensureBootstrapState(ctx context.Context, cfg config.Config) error {
	agentsPath, imStatePath, err := bootstrapPaths()
	if err != nil {
		return err
	}
	if err := EnsureIMBootstrapState(imStatePath); err != nil {
		return err
	}
	if _, err := CreateManagerBot(ctx, agentsPath, imStatePath, cfg); err != nil {
		return err
	}
	return nil
}

func bootstrapPaths() (agentsPath, imStatePath string, err error) {
	agentsPath, err = defaultAgentsPath()
	if err != nil {
		return "", "", err
	}
	imStatePath, err = defaultIMStatePath()
	if err != nil {
		return "", "", err
	}
	return agentsPath, imStatePath, nil
}

func createManagerBot(ctx context.Context, agentsPath, imStatePath string, cfg config.Config) (bot.Bot, error) {
	hubSvc, err := hub.NewService(cfg.Hub, hub.DefaultStoreFactory)
	if err != nil {
		return bot.Bot{}, err
	}
	bootstrapDefaults, err := hub.ResolveBootstrapDefaults(ctx, cfg.Bootstrap, hubSvc)
	if err != nil {
		return bot.Bot{}, err
	}
	opts, err := sandboxproviders.ServiceOptions(cfg.Sandbox)
	if err != nil {
		return bot.Bot{}, err
	}
	opts = append(opts,
		runtimewiring.WithPicoClawSandboxRuntime(nil),
		runtimewiring.WithOpenClawSandboxRuntime(),
		runtimewiring.WithNotifierRuntime(),
		agent.WithGatewayRuntime(bootstrapDefaults.ManagerRuntimeKind),
		agent.WithBootstrapDefaultTemplates(cfg.Bootstrap),
		agent.WithHubService(hubSvc),
	)
	agentSvc, err := agent.NewServiceWithLLM(effectiveLLMConfig(cfg), cfg.Server, bootstrapDefaults.ManagerImage, agentsPath, opts...)
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

func defaultConfig() config.Config {
	return config.Config{
		Server: config.ServerConfig{
			ListenAddr:  config.DefaultListenAddr(),
			AccessToken: config.DefaultAccessToken,
			NoAuth:      false,
		},
		Bootstrap: config.BootstrapConfig{},
		Sandbox:   config.SandboxConfig{},
	}
}

func loadConfig(path string) (config.Config, bool, error) {
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

func configNeedsCompletion(content string) bool {
	requiredSnippets := []string{
		"[server]",
		`listen_addr = `,
		`advertise_base_url = `,
		`access_token = `,
		`no_auth = `,
		"[sandbox]",
		`provider = `,
		`debian_registries_override = `,
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			return true
		}
	}
	if !strings.Contains(content, "[bootstrap]") {
		return true
	}
	hasBootstrapTemplates := strings.Contains(content, `default_manager_template = `) && strings.Contains(content, `default_worker_template = `)
	hasLegacyBootstrapImage := strings.Contains(content, `manager_image_override = `)
	if !hasBootstrapTemplates && !hasLegacyBootstrapImage {
		return true
	}
	if !strings.Contains(content, "[hub]") {
		return true
	}
	if !strings.Contains(content, `default_registry = `) {
		return true
	}
	if !strings.Contains(content, `default_publish_registry = `) {
		return true
	}
	if !strings.Contains(content, `[[hub.registries]]`) {
		return true
	}
	return false
}
