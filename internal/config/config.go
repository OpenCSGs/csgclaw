package config

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"csgclaw/internal/apiclient"
)

type Config struct {
	Server    ServerConfig
	Models    LLMConfig
	LLM       LLMConfig
	Model     ModelConfig
	Bootstrap BootstrapConfig
	Sandbox   SandboxConfig
	Hub       HubConfig

	raw rawConfigValues
}

type ServerConfig struct {
	ListenAddr       string
	AdvertiseBaseURL string
	AccessToken      string
	NoAuth           bool
}

type ModelConfig struct {
	Provider        string
	BaseURL         string
	APIKey          string
	ModelID         string
	ReasoningEffort string
}

type LLMConfig struct {
	Default        string
	Providers      map[string]ProviderConfig
	DefaultProfile string
	Profiles       map[string]ModelConfig
}

type BootstrapConfig struct {
	DefaultManagerTemplate string
	DefaultWorkerTemplate  string
}

func (c BootstrapConfig) ResolvedDefaultManagerTemplate() string {
	if template := strings.TrimSpace(c.DefaultManagerTemplate); template != "" {
		return template
	}
	return DefaultBootstrapManagerTemplate
}

func (c BootstrapConfig) ResolvedDefaultWorkerTemplate() string {
	if template := strings.TrimSpace(c.DefaultWorkerTemplate); template != "" {
		return template
	}
	return DefaultBootstrapWorkerTemplate
}

const (
	RuntimeKindPicoClawSandbox = "picoclaw_sandbox"
	RuntimeKindOpenClawSandbox = "openclaw_sandbox"
)

func (b BootstrapConfig) Validate() error {
	if strings.TrimSpace(b.ResolvedDefaultManagerTemplate()) == "" {
		return fmt.Errorf("bootstrap default_manager_template is required")
	}
	if strings.TrimSpace(b.ResolvedDefaultWorkerTemplate()) == "" {
		return fmt.Errorf("bootstrap default_worker_template is required")
	}
	return nil
}

// DefaultManagerImageForRuntimeKind returns the default manager/worker sandbox image for an explicit runtime_kind value.
func DefaultManagerImageForRuntimeKind(runtimeKind string) string {
	switch normalizeGatewayRuntimeKind(runtimeKind) {
	case RuntimeKindOpenClawSandbox:
		return DefaultOpenClawManagerImage
	default:
		return DefaultManagerImage
	}
}

func normalizeGatewayRuntimeKind(kind string) string {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case RuntimeKindPicoClawSandbox:
		return RuntimeKindPicoClawSandbox
	case RuntimeKindOpenClawSandbox:
		return RuntimeKindOpenClawSandbox
	default:
		return ""
	}
}

type SandboxConfig struct {
	Provider                 string
	StoragePath              string
	DockerCLIPath            string
	DebianRegistriesOverride []string
}

type HubConfig struct {
	DefaultRegistry        string
	DefaultPublishRegistry string
	Registries             []HubRegistryConfig
}

type HubRegistryConfig struct {
	Name    string
	Kind    string
	Path    string
	URL     string
	Token   string
	Enabled bool
}

func (c HubConfig) Resolved() HubConfig {
	c.DefaultRegistry = strings.TrimSpace(c.DefaultRegistry)
	if c.DefaultRegistry == "" {
		c.DefaultRegistry = DefaultHubRegistry
	}
	c.DefaultPublishRegistry = strings.TrimSpace(c.DefaultPublishRegistry)
	if c.DefaultPublishRegistry == "" {
		c.DefaultPublishRegistry = DefaultHubPublishRegistry
	}
	if len(c.Registries) == 0 {
		c.Registries = defaultHubRegistries()
		return c
	}

	out := make([]HubRegistryConfig, 0, len(c.Registries))
	for _, registry := range c.Registries {
		registry.Name = strings.TrimSpace(registry.Name)
		registry.Kind = strings.TrimSpace(registry.Kind)
		registry.Path = strings.TrimSpace(registry.Path)
		registry.URL = strings.TrimSpace(strings.TrimRight(registry.URL, "/"))
		registry.Token = strings.TrimSpace(registry.Token)
		out = append(out, registry)
	}
	if c.DefaultPublishRegistry == DefaultHubPublishRegistry && !hasHubRegistry(out, DefaultHubPublishRegistry) {
		out = append(out, defaultLocalHubRegistry())
	}
	c.Registries = out
	return c
}

func defaultBuiltinHubRegistry() HubRegistryConfig {
	return HubRegistryConfig{
		Name:    DefaultHubRegistry,
		Kind:    HubRegistryKindBuiltin,
		Enabled: true,
	}
}

func defaultLocalHubRegistry() HubRegistryConfig {
	return HubRegistryConfig{
		Name:    DefaultHubPublishRegistry,
		Kind:    HubRegistryKindLocal,
		Path:    DefaultHubRegistryPath(),
		Enabled: true,
	}
}

func defaultHubRegistries() []HubRegistryConfig {
	return []HubRegistryConfig{
		defaultBuiltinHubRegistry(),
		defaultLocalHubRegistry(),
	}
}

func hasHubRegistry(registries []HubRegistryConfig, name string) bool {
	name = strings.TrimSpace(name)
	for _, registry := range registries {
		if strings.TrimSpace(registry.Name) == name {
			return true
		}
	}
	return false
}

func (c SandboxConfig) Resolved() SandboxConfig {
	c.Provider = normalizeSandboxProvider(c.Provider)
	if c.Provider == "" {
		c.Provider = defaultSandboxProvider()
	}
	c.StoragePath = strings.TrimSpace(c.StoragePath)
	c.DebianRegistriesOverride = normalizeStringList(c.DebianRegistriesOverride)
	return c
}

func (c SandboxConfig) EffectiveDebianRegistries() []string {
	c = c.Resolved()
	if len(c.DebianRegistriesOverride) == 0 {
		return append([]string(nil), DefaultDebianRegistries...)
	}
	return append([]string(nil), c.DebianRegistriesOverride...)
}

// EffectiveDockerCLIPath returns the docker binary path for [sandbox].provider = docker.
// When unset, it defaults to "docker" (PATH lookup).
func (c SandboxConfig) EffectiveDockerCLIPath() string {
	p := strings.TrimSpace(c.DockerCLIPath)
	if p != "" {
		return p
	}
	return "docker"
}

type rawConfigValues struct {
	server        ServerConfig
	bootstrap     BootstrapConfig
	sandbox       SandboxConfig
	hub           rawHubConfig
	modelsDefault string
	models        map[string]rawProviderConfig
	resolved      *rawConfigValues
}

type rawProviderConfig struct {
	BaseURL         string
	APIKey          string
	Models          []string
	ReasoningEffort string
}

type rawHubConfig struct {
	DefaultRegistry        string
	DefaultPublishRegistry string
	Registries             []rawHubRegistryConfig
}

type rawHubRegistryConfig struct {
	Name       string
	Kind       string
	Path       string
	URL        string
	Token      string
	EnabledSet bool
}

const (
	AppDirName      = ".csgclaw"
	ConfigFileName  = "config.toml"
	StateFileName   = "state.json"
	AgentsDirName   = "agents"
	HubDirName      = "hub"
	IMDirName       = "im"
	ChannelsDirName = "channels"

	DefaultHTTPPort                 = apiclient.DefaultHTTPPort
	DefaultAccessToken              = "your_access_token"
	DefaultManagerImage             = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.5.9"
	DefaultOpenClawManagerImage     = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/openclaw:20260518.1-csgclaw"
	CSGHubProvider                  = "csghub"
	DockerProvider                  = "docker"
	BoxLiteProvider                 = "boxlite"
	DefaultHubRegistry              = "builtin"
	DefaultHubPublishRegistry       = "local"
	DefaultBootstrapManagerTemplate = "builtin/picoclaw-manager"
	DefaultBootstrapWorkerTemplate  = "builtin/picoclaw-worker"
	HubRegistryKindBuiltin          = "builtin"
	HubRegistryKindLocal            = "local"
	HubRegistryKindRemote           = "remote"
	BoxLiteCLIHomeDirName           = "boxlite"
	RuntimeHomeDirName              = BoxLiteCLIHomeDirName
)

// DefaultDebianRegistries is the default BoxLite Debian registry lookup order when
// [sandbox].debian_registries_override is unset or empty after normalization.
var DefaultDebianRegistries = []string{"harbor.opencsg.com", "docker.io"}

func DefaultListenAddr() string {
	return net.JoinHostPort("0.0.0.0", DefaultHTTPPort)
}

func DefaultAPIBaseURL() string {
	return apiclient.DefaultAPIBaseURL()
}

func ListenPort(listenAddr string) string {
	if listenAddr == "" {
		return DefaultHTTPPort
	}

	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil || port == "" {
		return DefaultHTTPPort
	}
	return port
}

func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, AppDirName), nil
}

func DefaultPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ConfigFileName), nil
}

func DefaultDomainDir(name string) (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func DefaultAgentsDir() (string, error) {
	return DefaultDomainDir(AgentsDirName)
}

func DefaultIMDir() (string, error) {
	return DefaultDomainDir(IMDirName)
}

func DefaultHubRegistryPath() string {
	dir, err := DefaultDomainDir(HubDirName)
	if err != nil {
		return filepath.Join(string(filepath.Separator), AppDirName, HubDirName)
	}
	return dir
}

func DefaultAgentsPath() (string, error) {
	dir, err := DefaultAgentsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, StateFileName), nil
}

func DefaultIMStatePath() (string, error) {
	dir, err := DefaultIMDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, StateFileName), nil
}

func DefaultChannelDir(name string) (string, error) {
	dir, err := DefaultDomainDir(ChannelsDirName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func LoadDefault() (Config, error) {
	path, err := DefaultPath()
	if err != nil {
		return Config{}, err
	}
	return Load(path)
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("config not found at %s; run `csgclaw serve` to initialize local state first", path)
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	modelsCfg := newLLMConfig()
	cfg := Config{
		Models: modelsCfg,
		LLM:    newLLMConfig(),
		raw: rawConfigValues{
			models: make(map[string]rawProviderConfig),
		},
	}

	section := ""
	hubRegistryIndex := -1
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "[["), "]]"))
			hubRegistryIndex = -1
			switch section {
			case "hub.registries":
				cfg.Hub.Registries = append(cfg.Hub.Registries, HubRegistryConfig{})
				cfg.raw.hub.Registries = append(cfg.raw.hub.Registries, rawHubRegistryConfig{})
				hubRegistryIndex = len(cfg.Hub.Registries) - 1
			}
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[]")
			hubRegistryIndex = -1
			if isLegacyConfigSection(section) {
				return Config{}, fmt.Errorf("legacy config section [%s] is no longer supported; migrate to [models] and [models.providers.<name>]", section)
			}
			continue
		}

		key, rawValue, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("invalid line: %q", line)
		}

		key = strings.TrimSpace(key)
		rawValue = strings.TrimSpace(rawValue)
		value := parseStringValue(rawValue)

		switch {
		case section == "server":
			switch key {
			case "listen_addr":
				cfg.raw.server.ListenAddr = parseRawStringValue(rawValue)
				cfg.Server.ListenAddr = value
			case "advertise_base_url":
				cfg.raw.server.AdvertiseBaseURL = parseRawStringValue(rawValue)
				cfg.Server.AdvertiseBaseURL = strings.TrimRight(value, "/")
			case "access_token":
				cfg.raw.server.AccessToken = parseRawStringValue(rawValue)
				cfg.Server.AccessToken = value
			case "no_auth":
				noAuth, err := parseBoolValue(rawValue)
				if err != nil {
					return Config{}, fmt.Errorf("parse server.no_auth: %w", err)
				}
				cfg.Server.NoAuth = noAuth
			}
		case section == "models":
			switch key {
			case "default":
				cfg.raw.modelsDefault = parseRawStringValue(rawValue)
				modelsCfg.Default = value
			}
		case section == "bootstrap":
			switch key {
			case "default_manager_template":
				cfg.raw.bootstrap.DefaultManagerTemplate = parseRawStringValue(rawValue)
				cfg.Bootstrap.DefaultManagerTemplate = value
			case "default_worker_template":
				cfg.raw.bootstrap.DefaultWorkerTemplate = parseRawStringValue(rawValue)
				cfg.Bootstrap.DefaultWorkerTemplate = value
			case "manager_image_override", "manager_image", "runtime_kind":
				// Keep loading legacy bootstrap keys for compatibility, but do not
				// surface them in the public config model anymore.
			}
		case section == "sandbox":
			switch key {
			case "provider":
				cfg.raw.sandbox.Provider = parseRawStringValue(rawValue)
				cfg.Sandbox.Provider = value
			case "home_dir_name":
				// Keep loading legacy configs that still contain this key, but
				// do not surface it in the public config model anymore.
			case "storage_path":
				cfg.raw.sandbox.StoragePath = parseRawStringValue(rawValue)
				cfg.Sandbox.StoragePath = value
			case "docker_cli_path":
				cfg.raw.sandbox.DockerCLIPath = parseRawStringValue(rawValue)
				cfg.Sandbox.DockerCLIPath = value
			case "debian_registries_override":
				registries, parseErr := parseStringArray(rawValue)
				if parseErr != nil {
					return Config{}, fmt.Errorf("parse sandbox.debian_registries_override: %w", parseErr)
				}
				cfg.Sandbox.DebianRegistriesOverride = registries
			}
		case section == "hub":
			switch key {
			case "default_registry":
				cfg.raw.hub.DefaultRegistry = parseRawStringValue(rawValue)
				cfg.Hub.DefaultRegistry = value
			case "default_publish_registry":
				cfg.raw.hub.DefaultPublishRegistry = parseRawStringValue(rawValue)
				cfg.Hub.DefaultPublishRegistry = value
			case "default_manager_template", "default_worker_template":
				// Bootstrap template defaults now live only under [bootstrap].
			}
		case section == "hub.registries":
			if hubRegistryIndex < 0 || hubRegistryIndex >= len(cfg.Hub.Registries) {
				return Config{}, fmt.Errorf("hub registry entry found before [[hub.registries]] header")
			}
			registry := cfg.Hub.Registries[hubRegistryIndex]
			rawRegistry := cfg.raw.hub.Registries[hubRegistryIndex]
			switch key {
			case "name":
				rawRegistry.Name = parseRawStringValue(rawValue)
				registry.Name = value
			case "kind":
				rawRegistry.Kind = parseRawStringValue(rawValue)
				registry.Kind = value
			case "path":
				rawRegistry.Path = parseRawStringValue(rawValue)
				registry.Path = value
			case "url":
				rawRegistry.URL = parseRawStringValue(rawValue)
				registry.URL = value
			case "token":
				rawRegistry.Token = parseRawStringValue(rawValue)
				registry.Token = value
			case "enabled":
				enabled, err := parseBoolValue(rawValue)
				if err != nil {
					return Config{}, fmt.Errorf("parse hub.registries.enabled: %w", err)
				}
				rawRegistry.EnabledSet = true
				registry.Enabled = enabled
			}
			cfg.Hub.Registries[hubRegistryIndex] = registry
			cfg.raw.hub.Registries[hubRegistryIndex] = rawRegistry
		default:
			if name, ok := modelsProviderSectionName(section); ok {
				provider := modelsCfg.Providers[name]
				rawProvider := cfg.raw.models[name]
				switch key {
				case "base_url":
					rawProvider.BaseURL = parseRawStringValue(rawValue)
					provider.BaseURL = value
				case "api_key":
					rawProvider.APIKey = parseRawStringValue(rawValue)
					provider.APIKey = value
				case "models":
					rawProvider.Models, _ = parseRawStringArray(rawValue)
					models, parseErr := parseStringArray(rawValue)
					if parseErr != nil {
						return Config{}, fmt.Errorf("parse models.providers.%s.models: %w", name, parseErr)
					}
					provider.Models = models
				case "reasoning_effort":
					rawProvider.ReasoningEffort = parseRawStringValue(rawValue)
					provider.ReasoningEffort = value
				}
				modelsCfg.Providers[name] = provider
				cfg.raw.models[name] = rawProvider
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("scan config: %w", err)
	}

	if cfg.Server.ListenAddr == "" {
		cfg.Server.ListenAddr = DefaultListenAddr()
	}
	if err := cfg.Bootstrap.Validate(); err != nil {
		return Config{}, err
	}
	if cfg.Server.AccessToken == "" {
		cfg.Server.AccessToken = DefaultAccessToken
	}
	cfg.Sandbox = cfg.Sandbox.Resolved()
	if err := cfg.Sandbox.Validate(); err != nil {
		return Config{}, err
	}
	for i := range cfg.Hub.Registries {
		if i >= len(cfg.raw.hub.Registries) || !cfg.raw.hub.Registries[i].EnabledSet {
			cfg.Hub.Registries[i].Enabled = true
		}
	}
	cfg.Hub = cfg.Hub.Resolved()

	if !modelsCfg.IsZero() {
		cfg.Models = modelsCfg.Normalized()
		cfg.LLM = cfg.Models
		cfg.syncModelFromLLM()
	}
	cfg.raw.resolved = cfg.resolvedRawValues()
	return cfg, nil
}

func (c Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	cfg := c
	writeModels := cfg.hasStaticLLMConfig()
	if writeModels {
		cfg.syncModelFromLLM()
	}
	resolvedSandbox := cfg.Sandbox.Resolved()
	loadedRaw := cfg.raw.resolvedOrZero()

	var b strings.Builder
	fmt.Fprintf(&b, `# Generated by csgclaw.

[server]
listen_addr = %q
advertise_base_url = %q
access_token = %q
no_auth = %t

[bootstrap]
default_manager_template = %q
default_worker_template = %q
`, cfg.rawOrResolvedString(cfg.raw.server.ListenAddr, loadedRaw.server.ListenAddr, cfg.Server.ListenAddr), cfg.rawOrResolvedString(cfg.raw.server.AdvertiseBaseURL, loadedRaw.server.AdvertiseBaseURL, cfg.Server.AdvertiseBaseURL), cfg.rawOrResolvedString(cfg.raw.server.AccessToken, loadedRaw.server.AccessToken, cfg.Server.AccessToken), cfg.Server.NoAuth, cfg.rawOrResolvedString(cfg.raw.bootstrap.DefaultManagerTemplate, loadedRaw.bootstrap.DefaultManagerTemplate, cfg.Bootstrap.ResolvedDefaultManagerTemplate()), cfg.rawOrResolvedString(cfg.raw.bootstrap.DefaultWorkerTemplate, loadedRaw.bootstrap.DefaultWorkerTemplate, cfg.Bootstrap.ResolvedDefaultWorkerTemplate()))
	sandboxSection := fmt.Sprintf(`
[sandbox]
provider = %q
`, cfg.rawOrResolvedSandboxProvider(cfg.raw.sandbox.Provider, loadedRaw.sandbox.Provider, resolvedSandbox.Provider))
	if strings.TrimSpace(resolvedSandbox.StoragePath) != "" {
		sandboxSection = strings.Replace(sandboxSection, "[sandbox]\n", fmt.Sprintf("[sandbox]\nstorage_path = %q\n", cfg.rawOrResolvedString(cfg.raw.sandbox.StoragePath, loadedRaw.sandbox.StoragePath, resolvedSandbox.StoragePath)), 1)
	}
	if strings.TrimSpace(resolvedSandbox.DockerCLIPath) != "" {
		sandboxSection = strings.Replace(sandboxSection, "[sandbox]\n", fmt.Sprintf("[sandbox]\ndocker_cli_path = %q\n", cfg.rawOrResolvedString(cfg.raw.sandbox.DockerCLIPath, loadedRaw.sandbox.DockerCLIPath, resolvedSandbox.DockerCLIPath)), 1)
	}
	overrideRegistries := cfg.rawOrResolvedStringArray(cfg.raw.sandbox.DebianRegistriesOverride, loadedRaw.sandbox.DebianRegistriesOverride, resolvedSandbox.DebianRegistriesOverride)
	sandboxSection += fmt.Sprintf("debian_registries_override = %s\n", formatStringArray(overrideRegistries))
	b.WriteString(sandboxSection)
	resolvedHub := cfg.Hub.Resolved()
	fmt.Fprintf(&b, `
[hub]
default_registry = %q
default_publish_registry = %q
`, cfg.rawOrResolvedString(cfg.raw.hub.DefaultRegistry, loadedRaw.hub.DefaultRegistry, resolvedHub.DefaultRegistry), cfg.rawOrResolvedString(cfg.raw.hub.DefaultPublishRegistry, loadedRaw.hub.DefaultPublishRegistry, resolvedHub.DefaultPublishRegistry))
	for i, registry := range resolvedHub.Registries {
		var rawRegistry rawHubRegistryConfig
		var loadedRegistry rawHubRegistryConfig
		if i < len(cfg.raw.hub.Registries) {
			rawRegistry = cfg.raw.hub.Registries[i]
		}
		if i < len(loadedRaw.hub.Registries) {
			loadedRegistry = loadedRaw.hub.Registries[i]
		}
		fmt.Fprintf(&b, `
[[hub.registries]]
name = %q
kind = %q
`, cfg.rawOrResolvedString(rawRegistry.Name, loadedRegistry.Name, registry.Name), cfg.rawOrResolvedString(rawRegistry.Kind, loadedRegistry.Kind, registry.Kind))
		if registry.Path != "" {
			fmt.Fprintf(&b, "path = %q\n", cfg.rawOrResolvedString(rawRegistry.Path, loadedRegistry.Path, registry.Path))
		}
		if registry.URL != "" {
			fmt.Fprintf(&b, "url = %q\n", cfg.rawOrResolvedString(rawRegistry.URL, loadedRegistry.URL, registry.URL))
		}
		if registry.Token != "" {
			fmt.Fprintf(&b, "token = %q\n", cfg.rawOrResolvedString(rawRegistry.Token, loadedRegistry.Token, registry.Token))
		}
		fmt.Fprintf(&b, "enabled = %t\n", registry.Enabled)
	}
	if writeModels {
		llmCfg := cfg.effectiveLLMConfig()
		defaultSelector := llmCfg.DefaultSelector()
		fmt.Fprintf(&b, `
[models]
default = %q
`, cfg.rawOrResolvedString(cfg.raw.modelsDefault, loadedRaw.modelsDefault, defaultSelector))

		for _, name := range sortedProviderNames(llmCfg.Providers) {
			provider := llmCfg.Providers[name].Resolved()
			rawProvider := cfg.raw.models[name]
			loadedProvider := loadedRaw.models[name]
			fmt.Fprintf(&b, `
[models.providers.%s]
base_url = %q
api_key = %q
models = %s
`, name, cfg.rawOrResolvedString(rawProvider.BaseURL, loadedProvider.BaseURL, provider.BaseURL), cfg.rawOrResolvedString(rawProvider.APIKey, loadedProvider.APIKey, provider.APIKey), formatStringArray(cfg.rawOrResolvedStringArray(rawProvider.Models, loadedProvider.Models, provider.Models)))
			if provider.ReasoningEffort != "" {
				fmt.Fprintf(&b, "reasoning_effort = %q\n", cfg.rawOrResolvedString(rawProvider.ReasoningEffort, loadedProvider.ReasoningEffort, provider.ReasoningEffort))
			}
		}
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (c Config) hasStaticLLMConfig() bool {
	if !c.Models.IsZero() || !c.LLM.IsZero() {
		return true
	}
	model := c.Model
	return strings.TrimSpace(model.Provider) != "" ||
		strings.TrimSpace(model.BaseURL) != "" ||
		strings.TrimSpace(model.APIKey) != "" ||
		strings.TrimSpace(model.ModelID) != "" ||
		strings.TrimSpace(model.ReasoningEffort) != ""
}

func modelsProviderSectionName(section string) (string, bool) {
	const prefix = "models.providers."
	if !strings.HasPrefix(section, prefix) {
		return "", false
	}
	name := strings.TrimSpace(strings.TrimPrefix(section, prefix))
	if name == "" {
		return "", false
	}
	return name, true
}

func SingleProfileLLM(model ModelConfig) LLMConfig {
	model = model.Resolved()
	provider := ProviderConfig{
		BaseURL:         model.BaseURL,
		APIKey:          model.APIKey,
		ReasoningEffort: model.ReasoningEffort,
	}
	if model.ModelID != "" {
		provider.Models = []string{model.ModelID}
	}
	return LLMConfig{
		Default:        DefaultLLMProfile,
		Providers:      map[string]ProviderConfig{DefaultLLMProfile: provider.Resolved()},
		DefaultProfile: DefaultLLMProfile,
		Profiles:       map[string]ModelConfig{DefaultLLMProfile: model},
	}
}

func (c Config) effectiveLLMConfig() LLMConfig {
	switch {
	case !c.Models.IsZero():
		return c.Models.Normalized()
	case !c.LLM.IsZero():
		return c.LLM.Normalized()
	default:
		return SingleProfileLLM(c.Model).Normalized()
	}
}

func (c *Config) syncModelFromLLM() {
	if c == nil {
		return
	}

	llmCfg := c.effectiveLLMConfig()
	c.Models = llmCfg
	c.LLM = llmCfg

	name, model, err := llmCfg.Resolve("")
	if err != nil {
		c.Model = c.Model.Resolved()
		return
	}

	c.Models.Default = name
	c.Models.DefaultProfile = name
	c.LLM = c.Models
	c.Model = model.Resolved()
}

func newLLMConfig() LLMConfig {
	return LLMConfig{
		Providers: make(map[string]ProviderConfig),
		Profiles:  make(map[string]ModelConfig),
	}
}

func isLegacyConfigSection(section string) bool {
	section = strings.TrimSpace(section)
	switch {
	case section == "llm":
		return true
	case section == "model":
		return true
	case strings.HasPrefix(section, "llm.profiles."):
		return true
	default:
		return false
	}
}

func parseStringValue(raw string) string {
	return expandEnv(parseRawStringValue(raw))
}

func parseBoolValue(raw string) (bool, error) {
	value := strings.TrimSpace(expandEnv(parseRawStringValue(raw)))
	if value == "" {
		return false, nil
	}
	return strconv.ParseBool(value)
}

func parseRawStringValue(raw string) string {
	return strings.Trim(strings.TrimSpace(raw), `"`)
}

func parseStringArray(raw string) ([]string, error) {
	rawValues, err := parseRawStringArray(raw)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rawValues))
	for _, value := range rawValues {
		value = expandEnv(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out, nil
}

func parseRawStringArray(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if !strings.HasPrefix(raw, "[") || !strings.HasSuffix(raw, "]") {
		return nil, fmt.Errorf("expected TOML string array, got %q", raw)
	}
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]"))
	if inner == "" {
		return nil, nil
	}
	parts := strings.Split(inner, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = parseRawStringValue(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out, nil
}

func formatStringArray(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	if len(quoted) == 0 {
		return "[]"
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func normalizeSandboxProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case "":
		return ""
	default:
		return provider
	}
}

func sortedProviderNames(providers map[string]ProviderConfig) []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
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
	if len(out) == 0 {
		return nil
	}
	return out
}
func expandEnv(value string) string {
	return os.Expand(value, func(name string) string {
		return os.Getenv(name)
	})
}

func (c Config) rawOrResolvedString(raw, loaded, resolved string) string {
	raw = strings.TrimSpace(raw)
	if raw != "" && loaded == resolved {
		return raw
	}
	return resolved
}

func (c Config) rawOrResolvedSandboxProvider(raw, loaded, resolved string) string {
	if strings.TrimSpace(raw) == "" && strings.TrimSpace(loaded) == "" && resolved == defaultSandboxProvider() {
		return ""
	}
	return c.rawOrResolvedString(raw, loaded, resolved)
}

func (c Config) rawOrResolvedStringArray(raw, loaded, resolved []string) []string {
	if len(raw) > 0 && equalStringSlices(loaded, resolved) {
		return raw
	}
	return resolved
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (r rawConfigValues) resolvedOrZero() rawConfigValues {
	if r.resolved == nil {
		return rawConfigValues{
			models: make(map[string]rawProviderConfig),
		}
	}
	return *r.resolved
}

func (c Config) resolvedRawValues() *rawConfigValues {
	out := rawConfigValues{
		models: make(map[string]rawProviderConfig),
	}

	if c.raw.server.ListenAddr != "" {
		out.server.ListenAddr = c.Server.ListenAddr
	}
	if c.raw.server.AdvertiseBaseURL != "" {
		out.server.AdvertiseBaseURL = c.Server.AdvertiseBaseURL
	}
	if c.raw.server.AccessToken != "" {
		out.server.AccessToken = c.Server.AccessToken
	}
	if c.raw.bootstrap.DefaultManagerTemplate != "" {
		out.bootstrap.DefaultManagerTemplate = c.Bootstrap.DefaultManagerTemplate
	}
	if c.raw.bootstrap.DefaultWorkerTemplate != "" {
		out.bootstrap.DefaultWorkerTemplate = c.Bootstrap.DefaultWorkerTemplate
	}
	if c.raw.sandbox.Provider != "" {
		out.sandbox.Provider = c.Sandbox.Provider
	}
	if c.raw.sandbox.StoragePath != "" {
		out.sandbox.StoragePath = c.Sandbox.StoragePath
	}
	if c.raw.sandbox.DockerCLIPath != "" {
		out.sandbox.DockerCLIPath = c.Sandbox.DockerCLIPath
	}
	if len(c.raw.sandbox.DebianRegistriesOverride) > 0 {
		out.sandbox.DebianRegistriesOverride = append([]string(nil), c.Sandbox.DebianRegistriesOverride...)
	}
	if c.raw.hub.DefaultRegistry != "" {
		out.hub.DefaultRegistry = c.Hub.DefaultRegistry
	}
	if c.raw.hub.DefaultPublishRegistry != "" {
		out.hub.DefaultPublishRegistry = c.Hub.DefaultPublishRegistry
	}
	for i, rawRegistry := range c.raw.hub.Registries {
		if i >= len(c.Hub.Registries) {
			break
		}
		registry := c.Hub.Registries[i]
		loadedRegistry := rawHubRegistryConfig{
			EnabledSet: rawRegistry.EnabledSet,
		}
		if rawRegistry.Name != "" {
			loadedRegistry.Name = registry.Name
		}
		if rawRegistry.Kind != "" {
			loadedRegistry.Kind = registry.Kind
		}
		if rawRegistry.Path != "" {
			loadedRegistry.Path = registry.Path
		}
		if rawRegistry.URL != "" {
			loadedRegistry.URL = registry.URL
		}
		if rawRegistry.Token != "" {
			loadedRegistry.Token = registry.Token
		}
		out.hub.Registries = append(out.hub.Registries, loadedRegistry)
	}
	if c.raw.modelsDefault != "" {
		out.modelsDefault = c.Models.Default
	}

	for name, rawProvider := range c.raw.models {
		provider := c.Models.Providers[name].Resolved()
		loadedProvider := rawProviderConfig{}
		if rawProvider.BaseURL != "" {
			loadedProvider.BaseURL = provider.BaseURL
		}
		if rawProvider.APIKey != "" {
			loadedProvider.APIKey = provider.APIKey
		}
		if len(rawProvider.Models) > 0 {
			loadedProvider.Models = append([]string(nil), provider.Models...)
		}
		if rawProvider.ReasoningEffort != "" {
			loadedProvider.ReasoningEffort = provider.ReasoningEffort
		}
		out.models[name] = loadedProvider
	}

	return &out
}
