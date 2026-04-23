package onboard

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/agent"
	"csgclaw/internal/bot"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/modelprovider"
	"csgclaw/internal/sandboxproviders"

	"golang.org/x/term"
)

var (
	CreateManagerBot       = createManagerBot
	EnsureIMBootstrapState = im.EnsureBootstrapState
	ListOpenAIModels       = modelprovider.ListOpenAIModels
	isTerminalFD           = term.IsTerminal
)

const providerCustom = "custom"

var (
	defaultCSGHubLiteBaseURL = modelprovider.CSGHubLiteDefaultBaseURL
	defaultCSGHubLiteAPIKey  = modelprovider.CSGHubLiteDefaultAPIKey
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
	provider := fs.String("provider", "", "LLM provider preset: csghub-lite or custom")
	baseURL := fs.String("base-url", "", "LLM provider base URL")
	apiKey := fs.String("api-key", "", "LLM provider API key")
	modelsValue := fs.String("models", "", "comma-separated LLM model identifiers")
	reasoningEffort := fs.String("reasoning-effort", "", "optional upstream reasoning_effort default")
	managerImage := fs.String("manager-image", "", "bootstrap manager image")
	debianRegistries := fs.String("debian-registries", "", "comma-separated OCI registries used for debian:bookworm-slim pulls (persisted to config)")
	forceRecreateManager := fs.Bool("force-recreate-manager", false, "remove and recreate the bootstrap manager box")
	if err := fs.Parse(args); err != nil {
		return err
	}
	visited := visitedFlags(fs)

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
				Provider:       config.DefaultSandboxProvider,
				HomeDirName:    config.DefaultSandboxHomeDirName,
				BoxLiteCLIPath: config.DefaultBoxLiteCLIPath,
			},
		}
	}

	llmCfg := effectiveLLMConfig(cfg)
	if hasExplicitLLMFlags(visited) {
		var err error
		llmCfg, err = applyExplicitModelFlags(ctx, llmCfg, hasExistingConfig, *provider, *baseURL, *apiKey, *modelsValue, *reasoningEffort, visited)
		if err != nil {
			return err
		}
	} else if canPrompt(run, globals.Output) {
		var err error
		llmCfg, err = promptModelConfig(ctx, run, llmCfg, hasExistingConfig)
		if err != nil {
			return err
		}
	}

	syncConfigWithLLM(&cfg, llmCfg)
	if *managerImage != "" {
		cfg.Bootstrap.ManagerImage = *managerImage
	}
	if strings.TrimSpace(*debianRegistries) != "" {
		cfg.Sandbox.DebianRegistries = parseRegistriesFlag(*debianRegistries)
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

	result := command.ActionResult{
		Command:        "onboard",
		Action:         "initialize",
		Status:         "initialized",
		ConfigPath:     path,
		ManagerImage:   cfg.Bootstrap.ManagerImage,
		Users:          []string{"admin", "manager"},
		ForceRecreated: *forceRecreateManager,
		Message:        fmt.Sprintf("initialized config at %s", path),
	}
	if globals.Output == "json" {
		return command.RenderAction(globals.Output, run.Stdout, result)
	}
	fmt.Fprintln(run.Stdout, result.Message)
	fmt.Fprintf(run.Stdout, "ensured bootstrap agent %q with image %q\n", agent.ManagerName, cfg.Bootstrap.ManagerImage)
	fmt.Fprintf(run.Stdout, "ensured IM members %q and %q\n", "admin", "manager")
	fmt.Fprintln(run.Stdout, "cleared IM invite draft data")
	if *forceRecreateManager {
		fmt.Fprintln(run.Stdout, "manager box was force-recreated")
	}
	return nil
}

func visitedFlags(fs interface{ Visit(func(*flag.Flag)) }) map[string]bool {
	visited := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	return visited
}

func hasExplicitLLMFlags(visited map[string]bool) bool {
	for _, name := range []string{"provider", "base-url", "api-key", "models", "reasoning-effort"} {
		if visited[name] {
			return true
		}
	}
	return false
}

func applyExplicitModelFlags(ctx context.Context, llmCfg config.LLMConfig, hasExistingConfig bool, providerName, baseURL, apiKey, modelsValue, reasoningEffort string, visited map[string]bool) (config.LLMConfig, error) {
	providerName = strings.TrimSpace(providerName)
	switch providerName {
	case "", providerCustom:
		return applyCustomModelFlags(llmCfg, hasExistingConfig, baseURL, apiKey, modelsValue, reasoningEffort, visited)
	case modelprovider.CSGHubLiteProviderName:
		return applyCSGHubLiteModelFlags(ctx, llmCfg, baseURL, apiKey, modelsValue, reasoningEffort, visited)
	default:
		return config.LLMConfig{}, fmt.Errorf("unsupported --provider %q; supported values are %q and %q", providerName, modelprovider.CSGHubLiteProviderName, providerCustom)
	}
}

func applyCustomModelFlags(llmCfg config.LLMConfig, _ bool, baseURL, apiKey, modelsValue, reasoningEffort string, visited map[string]bool) (config.LLMConfig, error) {
	targetProvider := configuredProviderName(llmCfg)
	providerCfg := llmCfg.Providers[targetProvider]
	if visited["base-url"] {
		providerCfg.BaseURL = baseURL
	}
	if visited["api-key"] {
		providerCfg.APIKey = apiKey
	}
	if visited["models"] {
		models, err := parseModelsFlag(modelsValue)
		if err != nil {
			return config.LLMConfig{}, err
		}
		providerCfg.Models = models
	}
	if visited["reasoning-effort"] {
		providerCfg.ReasoningEffort = reasoningEffort
	}
	llmCfg.Providers[targetProvider] = providerCfg
	if len(providerCfg.Models) > 0 && (visited["models"] || strings.TrimSpace(llmCfg.Default) == "" || strings.TrimSpace(llmCfg.DefaultProfile) == "") {
		defaultSelector := config.ModelSelector(targetProvider, providerCfg.Models[0])
		llmCfg.Default = defaultSelector
		llmCfg.DefaultProfile = defaultSelector
	}
	return llmCfg, nil
}

func applyCSGHubLiteModelFlags(ctx context.Context, llmCfg config.LLMConfig, baseURL, apiKey, modelsValue, reasoningEffort string, visited map[string]bool) (config.LLMConfig, error) {
	if !visited["base-url"] || strings.TrimSpace(baseURL) == "" {
		baseURL = defaultCSGHubLiteBaseURL
	}
	if !visited["api-key"] || strings.TrimSpace(apiKey) == "" {
		apiKey = defaultCSGHubLiteAPIKey
	}

	var models []string
	var err error
	if visited["models"] {
		models, err = parseModelsFlag(modelsValue)
		if err != nil {
			return config.LLMConfig{}, err
		}
	} else {
		models, err = discoverCSGHubLiteModels(ctx, baseURL, apiKey)
		if err != nil {
			return config.LLMConfig{}, err
		}
	}

	providerCfg := llmCfg.Providers[modelprovider.CSGHubLiteProviderName]
	providerCfg.BaseURL = baseURL
	providerCfg.APIKey = apiKey
	providerCfg.Models = models
	if visited["reasoning-effort"] {
		providerCfg.ReasoningEffort = reasoningEffort
	}
	if isEmptyProvider(llmCfg.Providers[config.DefaultLLMProfile]) {
		delete(llmCfg.Providers, config.DefaultLLMProfile)
		delete(llmCfg.Profiles, config.DefaultLLMProfile)
	}
	llmCfg.Providers[modelprovider.CSGHubLiteProviderName] = providerCfg

	defaultModel := chooseCSGHubLiteDefaultModel(llmCfg, models)
	llmCfg.Default = config.ModelSelector(modelprovider.CSGHubLiteProviderName, defaultModel)
	llmCfg.DefaultProfile = llmCfg.Default
	return llmCfg, nil
}

func isEmptyProvider(provider config.ProviderConfig) bool {
	provider = provider.Resolved()
	return provider.BaseURL == "" && provider.APIKey == "" && len(provider.Models) == 0 && provider.ReasoningEffort == ""
}

func promptModelConfig(ctx context.Context, run *command.Context, llmCfg config.LLMConfig, hasExistingConfig bool) (config.LLMConfig, error) {
	reader := bufio.NewReader(run.Stdin)
	defaultProvider := modelprovider.CSGHubLiteProviderName
	printProviderPromptIntro(run.Stdout, llmCfg, hasExistingConfig)
	if hasExistingConfig {
		if current := configuredProviderName(llmCfg); current != "" {
			defaultProvider = "keep"
		}
	}

	fmt.Fprintf(run.Stdout, "Provider [%s]: ", providerPromptDefaultLabel(defaultProvider, llmCfg))
	answer, err := readPromptLine(reader)
	if err != nil {
		return config.LLMConfig{}, err
	}
	answer = normalizeProviderSelection(answer, defaultProvider, hasExistingConfig)

	switch answer {
	case "keep":
		printModelConfigSummary(run.Stdout, "Keeping current model provider", llmCfg)
		return llmCfg, nil
	case modelprovider.CSGHubLiteProviderName:
		updated, err := promptCSGHubLiteModelConfig(ctx, reader, run, llmCfg)
		if err != nil {
			return config.LLMConfig{}, err
		}
		printModelConfigSummary(run.Stdout, "Using CSGHub-lite provider", updated)
		return updated, nil
	case providerCustom:
		updated, err := promptCustomModelConfig(reader, run, llmCfg)
		if err != nil {
			return config.LLMConfig{}, err
		}
		printModelConfigSummary(run.Stdout, "Using custom provider", updated)
		return updated, nil
	default:
		return config.LLMConfig{}, fmt.Errorf("unsupported provider selection %q; enter %q, %q, or a listed number", answer, modelprovider.CSGHubLiteProviderName, providerCustom)
	}
}

func promptCSGHubLiteModelConfig(ctx context.Context, reader *bufio.Reader, run *command.Context, llmCfg config.LLMConfig) (config.LLMConfig, error) {
	fmt.Fprintln(run.Stdout)
	fmt.Fprintln(run.Stdout, "CSGHub-lite provider")
	fmt.Fprintln(run.Stdout, "  Endpoint must be OpenAI-compatible and include the /v1 suffix.")
	fmt.Fprintln(run.Stdout, "  CSGClaw will read models from <base-url>/models.")
	baseURL, err := promptValue(reader, run.Stdout, "CSGHub-lite base URL", defaultCSGHubLitePromptBaseURL(llmCfg), true)
	if err != nil {
		return config.LLMConfig{}, err
	}
	fmt.Fprintf(run.Stdout, "\nChecking CSGHub-lite models at %s/models ...\n", strings.TrimRight(baseURL, "/"))
	return applyCSGHubLiteModelFlags(ctx, llmCfg, baseURL, "", "", "", map[string]bool{
		"provider": true,
		"base-url": true,
	})
}

func defaultCSGHubLitePromptBaseURL(llmCfg config.LLMConfig) string {
	providerCfg := llmCfg.Providers[modelprovider.CSGHubLiteProviderName].Resolved()
	if strings.TrimSpace(providerCfg.BaseURL) != "" {
		return providerCfg.BaseURL
	}
	return defaultCSGHubLiteBaseURL
}

func promptCustomModelConfig(reader *bufio.Reader, run *command.Context, llmCfg config.LLMConfig) (config.LLMConfig, error) {
	targetProvider := configuredProviderName(llmCfg)
	providerCfg := llmCfg.Providers[targetProvider]

	fmt.Fprintln(run.Stdout)
	fmt.Fprintln(run.Stdout, "Custom OpenAI-compatible provider")
	fmt.Fprintln(run.Stdout, "  Base URL example: https://api.openai.com/v1")
	fmt.Fprintln(run.Stdout, "  Models: comma-separated; the first model becomes the default.")
	baseURL, err := promptValue(reader, run.Stdout, "Base URL", providerCfg.BaseURL, true)
	if err != nil {
		return config.LLMConfig{}, err
	}
	apiKey, err := promptSecretValue(reader, run.Stdout, "API key", providerCfg.APIKey, true)
	if err != nil {
		return config.LLMConfig{}, err
	}
	modelsRaw, err := promptValue(reader, run.Stdout, "Models (comma-separated)", strings.Join(providerCfg.Models, ","), true)
	if err != nil {
		return config.LLMConfig{}, err
	}
	models, err := parseModelsFlag(modelsRaw)
	if err != nil {
		return config.LLMConfig{}, err
	}

	providerCfg.BaseURL = baseURL
	providerCfg.APIKey = apiKey
	providerCfg.Models = models
	llmCfg.Providers[targetProvider] = providerCfg
	llmCfg.Default = config.ModelSelector(targetProvider, models[0])
	llmCfg.DefaultProfile = llmCfg.Default
	return llmCfg, nil
}

func printProviderPromptIntro(w io.Writer, llmCfg config.LLMConfig, hasExistingConfig bool) {
	fmt.Fprintln(w, "CSGClaw model provider setup")
	fmt.Fprintln(w, "Choose how CSGClaw should reach an OpenAI-compatible model endpoint.")
	if hasExistingConfig {
		fmt.Fprintln(w)
		printCurrentModelConfig(w, llmCfg)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Options:")
		fmt.Fprintln(w, "  1. keep current - leave the existing model provider unchanged")
		fmt.Fprintf(w, "  2. %s - local CSGHub-lite at %s; models are read from /models\n", modelprovider.CSGHubLiteProviderName, defaultCSGHubLiteBaseURL)
		fmt.Fprintln(w, "  3. custom - enter another OpenAI-compatible base URL, API key, and models")
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options:")
	fmt.Fprintf(w, "  1. %s - local default at %s\n", modelprovider.CSGHubLiteProviderName, defaultCSGHubLiteBaseURL)
	fmt.Fprintln(w, "     Start it first with: csghub-lite run <model>  or  csghub-lite serve")
	fmt.Fprintln(w, "     CSGClaw will call /v1/models and use the first discovered model by default.")
	fmt.Fprintln(w, "  2. custom - enter another OpenAI-compatible base URL, API key, and models")
	fmt.Fprintln(w)
}

func printCurrentModelConfig(w io.Writer, llmCfg config.LLMConfig) {
	providerName := configuredProviderName(llmCfg)
	providerCfg := llmCfg.Providers[providerName].Resolved()
	fmt.Fprintln(w, "Current model configuration:")
	fmt.Fprintf(w, "  Provider: %s\n", providerName)
	if strings.TrimSpace(providerCfg.BaseURL) != "" {
		fmt.Fprintf(w, "  Base URL: %s\n", providerCfg.BaseURL)
	}
	if len(providerCfg.Models) > 0 {
		fmt.Fprintf(w, "  Models: %s\n", strings.Join(providerCfg.Models, ", "))
	}
	if strings.TrimSpace(llmCfg.Default) != "" {
		fmt.Fprintf(w, "  Default model: %s\n", llmCfg.Default)
	}
	if strings.TrimSpace(providerCfg.ReasoningEffort) != "" {
		fmt.Fprintf(w, "  Reasoning effort: %s\n", providerCfg.ReasoningEffort)
	}
}

func providerPromptDefaultLabel(defaultProvider string, llmCfg config.LLMConfig) string {
	if defaultProvider == "keep" {
		if current := configuredProviderName(llmCfg); current != "" {
			return "keep current: " + current
		}
		return "keep current"
	}
	return defaultProvider
}

func normalizeProviderSelection(answer, defaultProvider string, hasExistingConfig bool) string {
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "" {
		return defaultProvider
	}
	if hasExistingConfig {
		switch answer {
		case "1":
			return "keep"
		case "2":
			return modelprovider.CSGHubLiteProviderName
		case "3":
			return providerCustom
		}
	} else {
		switch answer {
		case "1":
			return modelprovider.CSGHubLiteProviderName
		case "2":
			return providerCustom
		}
	}
	return answer
}

func printModelConfigSummary(w io.Writer, title string, llmCfg config.LLMConfig) {
	providerName := configuredProviderName(llmCfg)
	providerCfg := llmCfg.Providers[providerName].Resolved()
	fmt.Fprintln(w)
	fmt.Fprintln(w, title)
	fmt.Fprintf(w, "  Provider: %s\n", providerName)
	if strings.TrimSpace(providerCfg.BaseURL) != "" {
		fmt.Fprintf(w, "  Base URL: %s\n", providerCfg.BaseURL)
	}
	if len(providerCfg.Models) > 0 {
		fmt.Fprintf(w, "  Models: %s\n", strings.Join(providerCfg.Models, ", "))
	}
	if strings.TrimSpace(llmCfg.Default) != "" {
		fmt.Fprintf(w, "  Default model: %s\n", llmCfg.Default)
	}
}

func promptValue(reader *bufio.Reader, w io.Writer, label, defaultValue string, required bool) (string, error) {
	if strings.TrimSpace(defaultValue) != "" {
		fmt.Fprintf(w, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(w, "%s: ", label)
	}
	value, err := readPromptLine(reader)
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = strings.TrimSpace(defaultValue)
	}
	if required && value == "" {
		return "", fmt.Errorf("%s is required", strings.ToLower(label))
	}
	return value, nil
}

func promptSecretValue(reader *bufio.Reader, w io.Writer, label, defaultValue string, required bool) (string, error) {
	if strings.TrimSpace(defaultValue) != "" {
		fmt.Fprintf(w, "%s [keep existing]: ", label)
	} else {
		fmt.Fprintf(w, "%s: ", label)
	}
	value, err := readPromptLine(reader)
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = strings.TrimSpace(defaultValue)
	}
	if required && value == "" {
		return "", fmt.Errorf("%s is required", strings.ToLower(label))
	}
	return value, nil
}

func readPromptLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func discoverCSGHubLiteModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	models, err := ListOpenAIModels(ctx, baseURL, apiKey)
	if err != nil {
		return nil, fmt.Errorf("csghub-lite is not reachable at %s (%v); start it with `csghub-lite run <model>` or `csghub-lite serve`, then retry", strings.TrimRight(baseURL, "/"), err)
	}
	return models, nil
}

func chooseCSGHubLiteDefaultModel(llmCfg config.LLMConfig, models []string) string {
	current := strings.TrimSpace(llmCfg.Default)
	if current == "" {
		current = strings.TrimSpace(llmCfg.DefaultProfile)
	}
	const prefix = modelprovider.CSGHubLiteProviderName + "."
	if strings.HasPrefix(current, prefix) {
		modelID := strings.TrimPrefix(current, prefix)
		for _, model := range models {
			if strings.TrimSpace(model) == modelID {
				return model
			}
		}
	}
	return models[0]
}

func canPrompt(run *command.Context, output string) bool {
	if output != "" && output != "table" {
		return false
	}
	if strings.TrimSpace(os.Getenv("CI")) != "" {
		return false
	}
	return isTerminal(run.Stdin) && isTerminal(run.Stdout)
}

func isTerminal(value any) bool {
	file, ok := value.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	return isTerminalFD(int(file.Fd()))
}

func createManagerBot(ctx context.Context, agentsPath, imStatePath string, cfg config.Config, forceRecreateManager bool) (bot.Bot, error) {
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
	}, forceRecreateManager)
}

func sandboxServiceOptions(cfg config.SandboxConfig) ([]agent.ServiceOption, error) {
	return sandboxproviders.ServiceOptions(cfg)
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
				"models config is incomplete (%s); run `csgclaw onboard --provider csghub-lite --models <model>` or `csgclaw onboard --base-url <url> --api-key <key> --models <model[,model...]> [--reasoning-effort <effort>]`",
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
