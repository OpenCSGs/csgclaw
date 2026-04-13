package onboard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/agent"
	"csgclaw/internal/bot"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
)

var (
	CreateManagerBot       = createManagerBot
	EnsureIMBootstrapState = im.EnsureBootstrapState
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
	baseURL := fs.String("base-url", "", "LLM provider base URL")
	apiKey := fs.String("api-key", "", "LLM provider API key")
	modelsValue := fs.String("models", "", "comma-separated LLM model identifiers")
	reasoningEffort := fs.String("reasoning-effort", "", "optional upstream reasoning_effort default")
	managerImage := fs.String("manager-image", "", "bootstrap manager image")
	forceRecreateManager := fs.Bool("force-recreate-manager", false, "remove and recreate the bootstrap manager box")
	if err := fs.Parse(args); err != nil {
		return err
	}

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
			},
			Bootstrap: config.BootstrapConfig{
				ManagerImage: config.DefaultManagerImage,
			},
		}
	}
	llmCfg := effectiveLLMConfig(cfg)
	targetProvider := configuredProviderName(llmCfg)
	providerCfg := llmCfg.Providers[targetProvider]
	if *baseURL != "" {
		providerCfg.BaseURL = *baseURL
	}
	if *apiKey != "" {
		providerCfg.APIKey = *apiKey
	}
	if *modelsValue != "" {
		models, err := parseModelsFlag(*modelsValue)
		if err != nil {
			return err
		}
		providerCfg.Models = models
	}
	if *reasoningEffort != "" {
		providerCfg.ReasoningEffort = *reasoningEffort
	}
	llmCfg.Providers[targetProvider] = providerCfg
	if len(providerCfg.Models) > 0 && (*modelsValue != "" || strings.TrimSpace(llmCfg.Default) == "" || strings.TrimSpace(llmCfg.DefaultProfile) == "") {
		defaultSelector := config.ModelSelector(targetProvider, providerCfg.Models[0])
		llmCfg.Default = defaultSelector
		llmCfg.DefaultProfile = defaultSelector
	}
	syncConfigWithLLM(&cfg, llmCfg)
	if *managerImage != "" {
		cfg.Bootstrap.ManagerImage = *managerImage
	}
	if err := validateModelConfig(cfg); err != nil {
		return err
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
	if _, err := CreateManagerBot(ctx, agentsPath, imStatePath, cfg, *forceRecreateManager); err != nil {
		return err
	}

	fmt.Fprintf(run.Stdout, "initialized config at %s\n", path)
	fmt.Fprintf(run.Stdout, "ensured bootstrap agent %q with image %q\n", agent.ManagerName, cfg.Bootstrap.ManagerImage)
	fmt.Fprintf(run.Stdout, "ensured IM members %q and %q\n", "admin", "manager")
	fmt.Fprintln(run.Stdout, "cleared IM invite draft data")
	if *forceRecreateManager {
		fmt.Fprintln(run.Stdout, "manager box was force-recreated")
	}
	return nil
}

func createManagerBot(ctx context.Context, agentsPath, imStatePath string, cfg config.Config, forceRecreateManager bool) (bot.Bot, error) {
	agentSvc, err := agent.NewServiceWithLLMAndChannels(effectiveLLMConfig(cfg), cfg.Server, cfg.Channels, cfg.Bootstrap.ManagerImage, agentsPath)
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
	}, forceRecreateManager)
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

func validateModelConfig(cfg config.Config) error {
	if err := effectiveLLMConfig(cfg).Validate(); err != nil {
		var validationErr *config.ModelValidationError
		if errors.As(err, &validationErr) && len(validationErr.MissingFields) > 0 {
			return fmt.Errorf(
				"models config is incomplete (%s); run `csgclaw onboard --base-url <url> --api-key <key> --models <model[,model...]> [--reasoning-effort <effort>]`",
				strings.Join(missingModelFlags(validationErr.MissingFields), ", "),
			)
		}
		return fmt.Errorf("models config is invalid: %w", err)
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

func effectiveLLMConfig(cfg config.Config) config.LLMConfig {
	if !cfg.Models.IsZero() {
		return cfg.Models.Normalized()
	}
	if !cfg.LLM.IsZero() {
		return cfg.LLM.Normalized()
	}
	return config.SingleProfileLLM(cfg.Model).Normalized()
}

func configuredProviderName(llmCfg config.LLMConfig) string {
	if name := strings.TrimSpace(llmCfg.EffectiveDefaultProvider()); name != "" {
		return name
	}
	return config.DefaultLLMProfile
}

func syncConfigWithLLM(cfg *config.Config, llmCfg config.LLMConfig) {
	if cfg == nil {
		return
	}
	llmCfg = llmCfg.Normalized()
	cfg.Models = llmCfg
	cfg.LLM = llmCfg
	if selector, modelCfg, err := llmCfg.Resolve(""); err == nil {
		cfg.Models.Default = selector
		cfg.Models.DefaultProfile = selector
		cfg.LLM = cfg.Models
		cfg.Model = modelCfg.Resolved()
		return
	}
	cfg.Model = cfg.Model.Resolved()
}

func parseModelsFlag(raw string) ([]string, error) {
	models := make([]string, 0)
	seen := make(map[string]struct{})
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		models = append(models, value)
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("--models must include at least one model identifier")
	}
	return models, nil
}
