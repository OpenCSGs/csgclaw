package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Config struct {
	Server    ServerConfig
	Models    LLMConfig
	LLM       LLMConfig
	Model     ModelConfig
	Bootstrap BootstrapConfig
	Sandbox   SandboxConfig
	Hub       HubConfig
	Skill     SkillConfig

	raw rawConfigValues
}

type ServerConfig struct {
	ListenAddr       string
	AdvertiseBaseURL string
	AccessToken      string
	NoAuth           bool
	ShowUpgrade      bool
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
	if template := normalizeManagerBootstrapTemplateRef(c.DefaultManagerTemplate); template != "" {
		return template
	}
	return DefaultBootstrapManagerTemplate
}

func (c BootstrapConfig) ResolvedDefaultWorkerTemplate() string {
	if template := normalizeBootstrapTemplateRef(c.DefaultWorkerTemplate); template != "" {
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

	configured := make([]HubRegistryConfig, 0, len(c.Registries))
	for _, registry := range c.Registries {
		configured = append(configured, normalizeHubRegistry(registry))
	}
	c.Registries = mergeHubRegistries(defaultHubRegistries(), configured)
	return c
}

func normalizeHubRegistry(registry HubRegistryConfig) HubRegistryConfig {
	registry.Name = strings.TrimSpace(registry.Name)
	registry.Kind = strings.TrimSpace(registry.Kind)
	registry.Path = strings.TrimSpace(registry.Path)
	registry.URL = strings.TrimSpace(strings.TrimRight(registry.URL, "/"))
	registry.Token = strings.TrimSpace(registry.Token)
	return registry
}

func needsRemoteHubRegistryURLRewrite(registry HubRegistryConfig) bool {
	return false
}

func mergeHubRegistries(defaults, configured []HubRegistryConfig) []HubRegistryConfig {
	configuredByName := make(map[string]HubRegistryConfig, len(configured))
	for _, registry := range configured {
		configuredByName[registry.Name] = registry
	}

	out := make([]HubRegistryConfig, 0, len(defaults)+len(configured))
	seen := make(map[string]struct{}, len(defaults)+len(configured))
	for _, registry := range defaults {
		if override, ok := configuredByName[registry.Name]; ok {
			out = append(out, override)
		} else {
			out = append(out, registry)
		}
		seen[registry.Name] = struct{}{}
	}
	for _, registry := range configured {
		if _, ok := seen[registry.Name]; ok {
			continue
		}
		out = append(out, registry)
		seen[registry.Name] = struct{}{}
	}
	return out
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

func defaultOfficialRemoteHubRegistry() HubRegistryConfig {
	return HubRegistryConfig{
		Name:    DefaultOfficialHubRegistryName,
		Kind:    HubRegistryKindRemote,
		URL:     DefaultOfficialHubRegistryURL,
		Enabled: true,
	}
}

func defaultHubRegistries() []HubRegistryConfig {
	return []HubRegistryConfig{
		defaultBuiltinHubRegistry(),
		defaultLocalHubRegistry(),
		defaultOfficialRemoteHubRegistry(),
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

func hasRemoteHubRegistryURLRewrite(registries []rawHubRegistryConfig) bool {
	for _, registry := range registries {
		if registry.RewriteURL {
			return true
		}
	}
	return false
}

func shouldWriteHubRegistry(registry HubRegistryConfig, rawRegistry, loadedRegistry rawHubRegistryConfig) bool {
	if strings.TrimSpace(registry.Name) != DefaultOfficialHubRegistryName ||
		strings.TrimSpace(registry.Kind) != HubRegistryKindRemote {
		return true
	}
	if rawRegistry.Name != "" || loadedRegistry.Name != "" {
		return true
	}
	defaultRegistry := defaultOfficialRemoteHubRegistry()
	return strings.TrimRight(strings.TrimSpace(registry.URL), "/") != defaultRegistry.URL ||
		strings.TrimSpace(registry.Token) != "" ||
		registry.Enabled != defaultRegistry.Enabled
}

func (c Config) HasExplicitOfficialHubRegistry() bool {
	for _, registry := range c.raw.hub.Registries {
		if parseRawStringValue(registry.Name) == DefaultOfficialHubRegistryName &&
			parseRawStringValue(registry.Kind) == HubRegistryKindRemote {
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
	bootstrapMeta rawBootstrapConfigMeta
	sandbox       SandboxConfig
	hub           rawHubConfig
	skill         rawSkillConfig
	modelsDefault string
	models        map[string]rawProviderConfig
	modelsPath    string
	resolved      *rawConfigValues
}

type rawBootstrapConfigMeta struct {
	LegacyManagerTemplateSlash bool
	LegacyWorkerTemplateSlash  bool
}

type rawProviderConfig struct {
	DisplayName     string
	Preset          string
	BaseURL         string
	APIKey          string
	HeadersJSON     string
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
	RewriteURL bool
}

const (
	AppDirName      = ".csgclaw"
	ConfigFileName  = "config.toml"
	StateFileName   = "state.json"
	AgentsDirName   = "agents"
	HubDirName      = "hub"
	IMDirName       = "im"
	ChannelsDirName = "channels"

	DefaultHTTPHost                 = "127.0.0.1"
	DefaultHTTPPort                 = "18080"
	DefaultAccessToken              = "your_access_token"
	CSGHubProvider                  = "csghub"
	DockerProvider                  = "docker"
	BoxLiteProvider                 = "boxlite"
	DefaultHubRegistry              = "builtin"
	DefaultHubPublishRegistry       = "local"
	DefaultOfficialHubRegistryName  = "official"
	DefaultOfficialHubRegistryURL   = "https://hub.opencsg.com"
	DefaultBootstrapManagerTemplate = "builtin.manager-codex"
	DefaultBootstrapWorkerTemplate  = "builtin.picoclaw-worker"
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
	return "http://" + net.JoinHostPort(DefaultHTTPHost, DefaultHTTPPort)
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

// ResolveAdvertiseBaseURL resolves server.advertise_base_url, falling back to a URL derived from listen_addr.
func ResolveAdvertiseBaseURL(server ServerConfig) string {
	if u := strings.TrimRight(strings.TrimSpace(server.AdvertiseBaseURL), "/"); u != "" {
		return u
	}

	host := DefaultHTTPHost
	if listenHost, _, err := net.SplitHostPort(server.ListenAddr); err == nil {
		if listenHost != "" && listenHost != "0.0.0.0" && listenHost != "::" {
			host = listenHost
		}
	}
	return "http://" + net.JoinHostPort(host, ListenPort(server.ListenAddr))
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

func DefaultStatePath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, StateFileName), nil
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

func DefaultTeamsPath() (string, error) {
	return DefaultStatePath()
}

func DefaultTasksDir() (string, error) {
	return DefaultDomainDir("tasks")
}

func DefaultScheduledTasksDir() (string, error) {
	return DefaultDomainDir("scheduled-tasks")
}

func DefaultHubRegistryPath() string {
	dir, err := DefaultDomainDir(HubDirName)
	if err != nil {
		return filepath.Join(string(filepath.Separator), AppDirName, HubDirName)
	}
	return dir
}

func DefaultAgentsPath() (string, error) {
	return DefaultStatePath()
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
		Server: ServerConfig{
			ShowUpgrade: true,
		},
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
			case "show_upgrade":
				showUpgrade, err := parseBoolValue(rawValue)
				if err != nil {
					return Config{}, fmt.Errorf("parse server.show_upgrade: %w", err)
				}
				cfg.Server.ShowUpgrade = showUpgrade
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
				raw := parseRawStringValue(rawValue)
				cfg.raw.bootstrap.DefaultManagerTemplate = normalizeManagerBootstrapTemplateRef(raw)
				cfg.raw.bootstrapMeta.LegacyManagerTemplateSlash = bootstrapTemplateRefUsesLegacySlash(raw)
				cfg.Bootstrap.DefaultManagerTemplate = normalizeManagerBootstrapTemplateRef(value)
			case "default_worker_template":
				raw := parseRawStringValue(rawValue)
				cfg.raw.bootstrap.DefaultWorkerTemplate = normalizeBootstrapTemplateRef(raw)
				cfg.raw.bootstrapMeta.LegacyWorkerTemplateSlash = bootstrapTemplateRefUsesLegacySlash(raw)
				cfg.Bootstrap.DefaultWorkerTemplate = normalizeBootstrapTemplateRef(value)
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
		case section == "skill", section == "clawhub":
			switch key {
			case "base_url":
				cfg.raw.skill.BaseURL = parseRawStringValue(rawValue)
				cfg.Skill.BaseURL = strings.TrimRight(value, "/")
			case "official_base_url":
				cfg.raw.skill.OfficialBaseURLSet = true
				cfg.raw.skill.OfficialBaseURL = parseRawStringValue(rawValue)
				cfg.Skill.OfficialBaseURLSet = true
				cfg.Skill.OfficialBaseURL = strings.TrimRight(value, "/")
			case "token":
				cfg.raw.skill.Token = parseRawStringValue(rawValue)
				cfg.Skill.Token = value
			case "non_suspicious_only":
				enabled, err := parseBoolValue(rawValue)
				if err != nil {
					return Config{}, fmt.Errorf("parse %s.non_suspicious_only: %w", section, err)
				}
				cfg.raw.skill.NonSuspiciousOnlySet = true
				cfg.raw.skill.NonSuspiciousOnly = enabled
				cfg.Skill.NonSuspiciousOnly = enabled
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
			rawRegistry.RewriteURL = needsRemoteHubRegistryURLRewrite(registry)
			cfg.Hub.Registries[hubRegistryIndex] = registry
			cfg.raw.hub.Registries[hubRegistryIndex] = rawRegistry
		default:
			if name, ok := modelsProviderSectionName(section); ok {
				provider := modelsCfg.Providers[name]
				rawProvider := cfg.raw.models[name]
				switch key {
				case "display_name":
					rawProvider.DisplayName = parseRawStringValue(rawValue)
					provider.DisplayName = value
				case "preset":
					rawProvider.Preset = parseRawStringValue(rawValue)
					provider.Preset = value
				case "base_url":
					rawProvider.BaseURL = parseRawStringValue(rawValue)
					provider.BaseURL = value
				case "api_key":
					rawProvider.APIKey = parseRawStringValue(rawValue)
					provider.APIKey = value
				case "headers_json":
					headersJSON := parseQuotedStringValue(rawValue)
					rawProvider.HeadersJSON = headersJSON
					headers, parseErr := parseHeadersJSON(headersJSON)
					if parseErr != nil {
						return Config{}, fmt.Errorf("parse models.providers.%s.headers_json: %w", name, parseErr)
					}
					provider.Headers = headers
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
	cfg.Skill = cfg.Skill.Resolved()
	if !cfg.raw.skill.NonSuspiciousOnlySet {
		cfg.Skill.NonSuspiciousOnly = true
	}

	if !modelsCfg.IsZero() {
		cfg.Models = modelsCfg.Normalized()
		cfg.LLM = cfg.Models
		cfg.syncModelFromLLM()
	}
	modelsPath, err := ModelsPathForConfigPath(path)
	if err != nil {
		return Config{}, err
	}
	cfg.raw.modelsPath = modelsPath
	if modelsFileCfg, ok, err := LoadModels(modelsPath); err != nil {
		return Config{}, err
	} else if ok {
		cfg.Models = modelsFileCfg.Normalized()
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
	modelsPath, err := ModelsPathForConfigPath(path)
	if err != nil {
		return err
	}
	llmForStorage := cfg.llmConfigForStorage()
	if !llmForStorage.IsZero() {
		cfg.Models = llmForStorage.Normalized()
		cfg.LLM = cfg.Models
		cfg.syncModelFromLLM()
	}
	if err := SaveModels(modelsPath, llmForStorage); err != nil {
		return err
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
show_upgrade = %t

[bootstrap]
`, cfg.rawOrResolvedString(cfg.raw.server.ListenAddr, loadedRaw.server.ListenAddr, cfg.Server.ListenAddr), cfg.rawOrResolvedString(cfg.raw.server.AdvertiseBaseURL, loadedRaw.server.AdvertiseBaseURL, cfg.Server.AdvertiseBaseURL), cfg.rawOrResolvedString(cfg.raw.server.AccessToken, loadedRaw.server.AccessToken, cfg.Server.AccessToken), cfg.Server.NoAuth, cfg.Server.ShowUpgrade)
	fmt.Fprintf(&b, "default_manager_template = %q\n", cfg.rawOrResolvedString(cfg.raw.bootstrap.DefaultManagerTemplate, loadedRaw.bootstrap.DefaultManagerTemplate, cfg.Bootstrap.ResolvedDefaultManagerTemplate()))
	fmt.Fprintf(&b, "default_worker_template = %q\n", cfg.rawOrResolvedString(cfg.raw.bootstrap.DefaultWorkerTemplate, loadedRaw.bootstrap.DefaultWorkerTemplate, cfg.Bootstrap.ResolvedDefaultWorkerTemplate()))
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
	for _, registry := range resolvedHub.Registries {
		rawRegistry := findRawHubRegistry(cfg.raw.hub.Registries, registry.Name)
		loadedRegistry := findRawHubRegistry(loadedRaw.hub.Registries, registry.Name)
		if !shouldWriteHubRegistry(registry, rawRegistry, loadedRegistry) {
			continue
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
			registryURL := cfg.rawOrResolvedString(rawRegistry.URL, loadedRegistry.URL, registry.URL)
			if rawRegistry.RewriteURL {
				registryURL = registry.URL
			}
			fmt.Fprintf(&b, "url = %q\n", registryURL)
		}
		if registry.Token != "" {
			fmt.Fprintf(&b, "token = %q\n", cfg.rawOrResolvedString(rawRegistry.Token, loadedRegistry.Token, registry.Token))
		}
		fmt.Fprintf(&b, "enabled = %t\n", registry.Enabled)
	}
	resolvedSkill := cfg.Skill.Resolved()
	if cfg.raw.skill.BaseURL != "" || cfg.raw.skill.OfficialBaseURLSet || cfg.raw.skill.Token != "" || cfg.raw.skill.NonSuspiciousOnlySet {
		fmt.Fprintf(&b, `
[skill]
base_url = %q
`, cfg.rawOrResolvedString(cfg.raw.skill.BaseURL, loadedRaw.skill.BaseURL, resolvedSkill.BaseURL))
		if cfg.raw.skill.OfficialBaseURLSet {
			fmt.Fprintf(&b, "official_base_url = %q\n", cfg.rawOrResolvedString(cfg.raw.skill.OfficialBaseURL, loadedRaw.skill.OfficialBaseURL, resolvedSkill.OfficialBaseURL))
		}
		if cfg.raw.skill.Token != "" || loadedRaw.skill.Token != "" {
			fmt.Fprintf(&b, "token = %q\n", cfg.rawOrResolvedString(cfg.raw.skill.Token, loadedRaw.skill.Token, resolvedSkill.Token))
		}
		if cfg.raw.skill.NonSuspiciousOnlySet {
			fmt.Fprintf(&b, "non_suspicious_only = %t\n", resolvedSkill.NonSuspiciousOnly)
		}
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (c Config) NeedsMigrationRewrite() bool {
	// Transitional compatibility for legacy slash-separated bootstrap template refs.
	// After old configs are no longer supported in the field, remove this rewrite
	// trigger together with the slash-to-dot normalization path.
	return c.raw.bootstrapMeta.LegacyManagerTemplateSlash ||
		c.raw.bootstrapMeta.LegacyWorkerTemplateSlash ||
		hasRemoteHubRegistryURLRewrite(c.raw.hub.Registries) ||
		strings.TrimSpace(c.raw.modelsDefault) != "" ||
		len(c.raw.models) > 0
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

func (c Config) llmConfigForStorage() LLMConfig {
	switch {
	case !c.Models.IsZero():
		return c.Models.Normalized()
	case !c.LLM.IsZero():
		return c.LLM.Normalized()
	case c.hasStaticLLMConfig():
		return SingleProfileLLM(c.Model).Normalized()
	default:
		return newLLMConfig()
	}
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
	raw = strings.TrimSpace(raw)
	// Preserve escaped TOML string content such as Windows paths so later
	// save cycles do not introduce double-escaping.
	if value, err := strconv.Unquote(raw); err == nil {
		return value
	}
	return strings.Trim(raw, `"`)
}

func parseQuotedStringValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if value, err := strconv.Unquote(raw); err == nil {
		return expandEnv(value)
	}
	return parseStringValue(raw)
}

func findRawHubRegistry(registries []rawHubRegistryConfig, name string) rawHubRegistryConfig {
	name = strings.TrimSpace(name)
	for _, registry := range registries {
		if parseRawStringValue(registry.Name) == name {
			return registry
		}
	}
	return rawHubRegistryConfig{}
}

func findResolvedHubRegistry(registries []HubRegistryConfig, name string) (HubRegistryConfig, bool) {
	name = strings.TrimSpace(name)
	for _, registry := range registries {
		if registry.Name == name {
			return registry, true
		}
	}
	return HubRegistryConfig{}, false
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

func parseHeadersJSON(value string) (map[string]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return nil, err
	}
	return normalizeHeaderMap(decoded), nil
}

func formatHeadersJSON(values map[string]string) string {
	values = normalizeHeaderMap(values)
	if len(values) == 0 {
		return ""
	}
	data, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(data)
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

func normalizeHeaderMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" || strings.EqualFold(key, "authorization") || strings.EqualFold(key, "content-type") {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[key] = value
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
		out.bootstrap.DefaultManagerTemplate = normalizeManagerBootstrapTemplateRef(c.Bootstrap.DefaultManagerTemplate)
	}
	if c.raw.bootstrap.DefaultWorkerTemplate != "" {
		out.bootstrap.DefaultWorkerTemplate = normalizeBootstrapTemplateRef(c.Bootstrap.DefaultWorkerTemplate)
	}
	out.bootstrapMeta = c.raw.bootstrapMeta
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
	resolvedHub := c.Hub.Resolved()
	for _, rawRegistry := range c.raw.hub.Registries {
		registry, ok := findResolvedHubRegistry(resolvedHub.Registries, parseRawStringValue(rawRegistry.Name))
		if !ok {
			continue
		}
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
	if c.raw.skill.BaseURL != "" {
		out.skill.BaseURL = c.Skill.BaseURL
	}
	if c.raw.skill.OfficialBaseURLSet {
		out.skill.OfficialBaseURL = c.Skill.OfficialBaseURL
		out.skill.OfficialBaseURLSet = true
	}
	if c.raw.skill.Token != "" {
		out.skill.Token = c.Skill.Token
	}
	if c.raw.skill.NonSuspiciousOnlySet {
		out.skill.NonSuspiciousOnly = c.Skill.NonSuspiciousOnly
		out.skill.NonSuspiciousOnlySet = true
	}
	if c.raw.modelsDefault != "" {
		out.modelsDefault = c.Models.Default
	}

	for name, rawProvider := range c.raw.models {
		provider := c.Models.Providers[name].Resolved()
		loadedProvider := rawProviderConfig{}
		if rawProvider.DisplayName != "" {
			loadedProvider.DisplayName = provider.DisplayName
		}
		if rawProvider.Preset != "" {
			loadedProvider.Preset = provider.Preset
		}
		if rawProvider.BaseURL != "" {
			loadedProvider.BaseURL = provider.BaseURL
		}
		if rawProvider.APIKey != "" {
			loadedProvider.APIKey = provider.APIKey
		}
		if rawProvider.HeadersJSON != "" {
			loadedProvider.HeadersJSON = formatHeadersJSON(provider.Headers)
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

func normalizeBootstrapTemplateRef(value string) string {
	value = strings.TrimSpace(value)
	if !bootstrapTemplateRefUsesLegacySlash(value) {
		return value
	}
	left, right, _ := strings.Cut(value, "/")
	return strings.TrimSpace(left) + "." + strings.TrimSpace(right)
}

func normalizeManagerBootstrapTemplateRef(value string) string {
	value = normalizeBootstrapTemplateRef(value)
	switch value {
	case "builtin.picoclaw-manager", "builtin.openclaw-manager":
		return DefaultBootstrapManagerTemplate
	default:
		return value
	}
}

func bootstrapTemplateRefUsesLegacySlash(value string) bool {
	value = strings.TrimSpace(value)
	left, right, ok := strings.Cut(value, "/")
	if !ok {
		return false
	}
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	return !strings.Contains(left, "/") && !strings.Contains(right, "/")
}
