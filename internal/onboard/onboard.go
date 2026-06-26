package onboard

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/app/runtimewiring"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/localstore"
	"csgclaw/internal/participant"
	"csgclaw/internal/sandboxproviders"
	hub "csgclaw/internal/template"
)

var (
	CreateManagerParticipant = createManagerParticipant
	EnsureIMBootstrapState   = im.EnsureBootstrapState
	defaultAgentsPath        = config.DefaultAgentsPath
	defaultIMStatePath       = config.DefaultIMStatePath
	defaultParticipantsPath  = config.DefaultStatePath
)

type EnsureStateOptions struct {
	ConfigPath   string
	NoAuthDetect bool
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
	if opts.NoAuthDetect {
		ctx = context.WithValue(ctx, noAuthDetectContextKey{}, true)
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
	needsMigrationRewrite := hasExistingConfig && cfg.NeedsMigrationRewrite()
	if needsMigrationRewrite {
		needsSave = true
	}
	modelsPath, err := config.ModelsPathForConfigPath(path)
	if err != nil {
		return config.Config{}, err
	}
	if _, err := os.Stat(modelsPath); err != nil {
		if os.IsNotExist(err) {
			needsSave = true
		} else {
			return config.Config{}, fmt.Errorf("stat models config: %w", err)
		}
	}
	if needsSave {
		if needsMigrationRewrite {
			if err := backupConfigDir(path); err != nil {
				return config.Config{}, err
			}
		}
		if err := cfg.Save(path); err != nil {
			return config.Config{}, err
		}
	}
	return cfg, nil
}

func backupConfigDir(path string) error {
	root := filepath.Dir(path)
	if filepath.Base(root) != config.AppDirName {
		return nil
	}
	if _, err := localstore.CreateSiblingBackup(root, time.Now()); err != nil {
		return fmt.Errorf("backup config dir: %w", err)
	}
	return nil
}

func ensureBootstrapState(ctx context.Context, cfg config.Config) error {
	agentsPath, imStatePath, err := bootstrapPaths()
	if err != nil {
		return err
	}
	if err := EnsureIMBootstrapState(imStatePath); err != nil {
		return err
	}
	if _, err := CreateManagerParticipant(ctx, agentsPath, imStatePath, cfg); err != nil {
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

func createManagerParticipant(ctx context.Context, agentsPath, imStatePath string, cfg config.Config) (participant.Participant, error) {
	hubSvc, err := hub.NewService(cfg.Hub, hub.DefaultStoreFactory)
	if err != nil {
		return participant.Participant{}, err
	}
	bootstrapDefaults, err := hub.ResolveBootstrapDefaults(ctx, cfg.Bootstrap, hubSvc)
	if err != nil {
		return participant.Participant{}, err
	}
	opts, err := sandboxproviders.ServiceOptions(cfg.Sandbox)
	if err != nil {
		return participant.Participant{}, err
	}
	opts = append(opts,
		runtimewiring.WithPicoClawSandboxRuntime(nil),
		runtimewiring.WithOpenClawSandboxRuntime(nil),
		agent.WithGatewayRuntime(bootstrapDefaults.ManagerRuntimeKind),
		agent.WithBootstrapDefaultTemplates(cfg.Bootstrap),
		agent.WithHubService(hubSvc),
	)
	if noAuthDetectFromContext(ctx) {
		opts = append(opts, agent.WithStartupProfileDetectionDisabled())
	}
	agentSvc, err := agent.NewServiceWithLLM(effectiveLLMConfig(cfg), cfg.Server, bootstrapDefaults.ManagerImage, agentsPath, opts...)
	if err != nil {
		return participant.Participant{}, err
	}
	defer func() {
		_ = agentSvc.Close()
	}()

	imSvc, err := im.NewServiceFromPath(imStatePath)
	if err != nil {
		return participant.Participant{}, err
	}
	participantsPath, err := defaultParticipantsPath()
	if err != nil {
		return participant.Participant{}, err
	}
	store, err := participant.NewStore(participantsPath)
	if err != nil {
		return participant.Participant{}, err
	}
	participantSvc := participant.NewService(
		store,
		participant.WithAgentService(agentSvc),
		participant.WithIMService(imSvc),
	)
	if _, err := participantSvc.EnsureBootstrapAdmin(ctx); err != nil {
		return participant.Participant{}, err
	}
	created, err := participantSvc.EnsureBootstrapManager(ctx)
	if err != nil {
		return participant.Participant{}, err
	}
	return created, nil
}

type noAuthDetectContextKey struct{}

func noAuthDetectFromContext(ctx context.Context) bool {
	value, _ := ctx.Value(noAuthDetectContextKey{}).(bool)
	return value
}

func defaultConfig() config.Config {
	return config.Config{
		Server: config.ServerConfig{
			ListenAddr:  config.DefaultListenAddr(),
			AccessToken: config.DefaultAccessToken,
			NoAuth:      false,
			ShowUpgrade: true,
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
