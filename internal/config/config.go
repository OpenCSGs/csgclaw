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
	Channels  ChannelsConfig

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
	ManagerImage string
}

type SandboxConfig struct {
	Provider         string
	HomeDirName      string
	BoxLiteCLIPath   string
	DebianRegistries []string
}

func (c SandboxConfig) Resolved() SandboxConfig {
	if strings.TrimSpace(c.Provider) == "" {
		c.Provider = DefaultSandboxProvider
	}
	if strings.TrimSpace(c.HomeDirName) == "" {
		c.HomeDirName = DefaultSandboxHomeDirName
	}
	if strings.TrimSpace(c.BoxLiteCLIPath) == "" {
		c.BoxLiteCLIPath = DefaultBoxLiteCLIPath
	}
	c.DebianRegistries = normalizeStringList(c.DebianRegistries)
	if len(c.DebianRegistries) == 0 {
		c.DebianRegistries = append([]string(nil), DefaultDebianRegistries...)
	}
	return c
}

type ChannelsConfig struct {
	FeishuAdminOpenID string
	Feishu            map[string]FeishuConfig
}

type FeishuConfig struct {
	AppID     string
	AppSecret string
}

type rawConfigValues struct {
	server        ServerConfig
	bootstrap     BootstrapConfig
	sandbox       SandboxConfig
	modelsDefault string
	models        map[string]rawProviderConfig
	channels      rawChannelsConfig
	resolved      *rawConfigValues
}

type rawChannelsConfig struct {
	FeishuAdminOpenID string
	Feishu            map[string]FeishuConfig
}

type rawProviderConfig struct {
	BaseURL         string
	APIKey          string
	Models          []string
	ReasoningEffort string
}

const (
	AppDirName      = ".csgclaw"
	ConfigFileName  = "config.toml"
	StateFileName   = "state.json"
	AgentsDirName   = "agents"
	IMDirName       = "im"
	ChannelsDirName = "channels"

	DefaultHTTPPort           = apiclient.DefaultHTTPPort
	DefaultAccessToken        = "your_access_token"
	DefaultManagerImage       = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.4.24.0"
	CSGHubProvider            = "csghub"
	BoxLiteSDKProvider        = "boxlite-sdk"
	BoxLiteCLIProvider        = "boxlite-cli"
	DefaultBoxLiteCLIPath     = "boxlite"
	DefaultSandboxHomeDirName = "boxlite"
	RuntimeHomeDirName        = DefaultSandboxHomeDirName
)

// DefaultDebianRegistries is the default BoxLite Debian registry lookup order when
// [sandbox].debian_registries is unset or empty after normalization.
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
			return Config{}, fmt.Errorf("config not found at %s; run `csgclaw onboard` first", path)
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	modelsCfg := newLLMConfig()
	cfg := Config{
		Models: modelsCfg,
		LLM:    newLLMConfig(),
		raw: rawConfigValues{
			models: make(map[string]rawProviderConfig),
			channels: rawChannelsConfig{
				Feishu: make(map[string]FeishuConfig),
			},
		},
	}

	section := ""
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[]")
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
			case "manager_image":
				cfg.raw.bootstrap.ManagerImage = parseRawStringValue(rawValue)
				cfg.Bootstrap.ManagerImage = value
			}
		case section == "sandbox":
			switch key {
			case "provider":
				cfg.raw.sandbox.Provider = parseRawStringValue(rawValue)
				cfg.Sandbox.Provider = value
			case "home_dir_name":
				cfg.raw.sandbox.HomeDirName = parseRawStringValue(rawValue)
				cfg.Sandbox.HomeDirName = value
			case "boxlite_cli_path":
				cfg.raw.sandbox.BoxLiteCLIPath = parseRawStringValue(rawValue)
				cfg.Sandbox.BoxLiteCLIPath = value
			case "debian_registries":
				registries, parseErr := parseStringArray(rawValue)
				if parseErr != nil {
					return Config{}, fmt.Errorf("parse sandbox.debian_registries: %w", parseErr)
				}
				cfg.Sandbox.DebianRegistries = registries
			}
		case section == "channels.feishu":
			switch key {
			case "admin_open_id":
				cfg.raw.channels.FeishuAdminOpenID = parseRawStringValue(rawValue)
				cfg.Channels.FeishuAdminOpenID = value
			}
		case strings.HasPrefix(section, "channels.feishu."):
			name := strings.TrimPrefix(section, "channels.feishu.")
			if name == "" {
				return Config{}, fmt.Errorf("invalid feishu channel section: %q", section)
			}
			if cfg.Channels.Feishu == nil {
				cfg.Channels.Feishu = make(map[string]FeishuConfig)
			}
			feishu := cfg.Channels.Feishu[name]
			rawFeishu := cfg.raw.channels.Feishu[name]
			switch key {
			case "app_id":
				rawFeishu.AppID = parseRawStringValue(rawValue)
				feishu.AppID = value
			case "app_secret":
				rawFeishu.AppSecret = parseRawStringValue(rawValue)
				feishu.AppSecret = value
			}
			cfg.Channels.Feishu[name] = feishu
			cfg.raw.channels.Feishu[name] = rawFeishu
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
	if cfg.Bootstrap.ManagerImage == "" {
		cfg.Bootstrap.ManagerImage = DefaultManagerImage
	}
	if cfg.Server.AccessToken == "" {
		cfg.Server.AccessToken = DefaultAccessToken
	}
	cfg.Sandbox = cfg.Sandbox.Resolved()

	if !modelsCfg.IsZero() {
		cfg.Models = modelsCfg.Normalized()
	} else {
		cfg.Models = SingleProfileLLM(ModelConfig{})
	}
	cfg.LLM = cfg.Models
	cfg.syncModelFromLLM()
	cfg.raw.resolved = cfg.resolvedRawValues()
	return cfg, nil
}

func (c Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	cfg := c
	cfg.syncModelFromLLM()
	llmCfg := cfg.effectiveLLMConfig()
	defaultSelector := llmCfg.DefaultSelector()
	resolvedSandbox := cfg.Sandbox.Resolved()
	loadedRaw := cfg.raw.resolvedOrZero()

	var b strings.Builder
	fmt.Fprintf(&b, `# Generated by csgclaw onboard.

[server]
listen_addr = %q
advertise_base_url = %q
access_token = %q
no_auth = %t

[bootstrap]
manager_image = %q
`, cfg.rawOrResolvedString(cfg.raw.server.ListenAddr, loadedRaw.server.ListenAddr, cfg.Server.ListenAddr), cfg.rawOrResolvedString(cfg.raw.server.AdvertiseBaseURL, loadedRaw.server.AdvertiseBaseURL, cfg.Server.AdvertiseBaseURL), cfg.rawOrResolvedString(cfg.raw.server.AccessToken, loadedRaw.server.AccessToken, cfg.Server.AccessToken), cfg.Server.NoAuth, cfg.rawOrResolvedString(cfg.raw.bootstrap.ManagerImage, loadedRaw.bootstrap.ManagerImage, cfg.Bootstrap.ManagerImage))
	sandboxSection := fmt.Sprintf(`

[sandbox]
provider = %q
home_dir_name = %q
boxlite_cli_path = %q
`, cfg.rawOrResolvedString(cfg.raw.sandbox.Provider, loadedRaw.sandbox.Provider, resolvedSandbox.Provider), cfg.rawOrResolvedString(cfg.raw.sandbox.HomeDirName, loadedRaw.sandbox.HomeDirName, resolvedSandbox.HomeDirName), cfg.rawOrResolvedString(cfg.raw.sandbox.BoxLiteCLIPath, loadedRaw.sandbox.BoxLiteCLIPath, resolvedSandbox.BoxLiteCLIPath))
	if len(resolvedSandbox.DebianRegistries) > 0 {
		sandboxSection = strings.Replace(sandboxSection, "[sandbox]\n", fmt.Sprintf("[sandbox]\ndebian_registries = %s\n", formatStringArray(resolvedSandbox.DebianRegistries)), 1)
	}
	b.WriteString(sandboxSection)
	fmt.Fprintf(&b, `[models]
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

	if strings.TrimSpace(c.Channels.FeishuAdminOpenID) != "" {
		fmt.Fprintf(&b, `
[channels.feishu]
admin_open_id = %q
`, cfg.rawOrResolvedString(cfg.raw.channels.FeishuAdminOpenID, loadedRaw.channels.FeishuAdminOpenID, c.Channels.FeishuAdminOpenID))
	}

	if len(c.Channels.Feishu) > 0 {
		names := make([]string, 0, len(c.Channels.Feishu))
		for name := range c.Channels.Feishu {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			feishu := c.Channels.Feishu[name]
			rawFeishu := cfg.raw.channels.Feishu[name]
			loadedFeishu := loadedRaw.channels.Feishu[name]
			fmt.Fprintf(&b, `
[channels.feishu.%s]
app_id = %q
app_secret = %q
`, name, cfg.rawOrResolvedString(rawFeishu.AppID, loadedFeishu.AppID, feishu.AppID), cfg.rawOrResolvedString(rawFeishu.AppSecret, loadedFeishu.AppSecret, feishu.AppSecret))
		}
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
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
			channels: rawChannelsConfig{
				Feishu: make(map[string]FeishuConfig),
			},
		}
	}
	return *r.resolved
}

func (c Config) resolvedRawValues() *rawConfigValues {
	out := rawConfigValues{
		models: make(map[string]rawProviderConfig),
		channels: rawChannelsConfig{
			Feishu: make(map[string]FeishuConfig),
		},
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
	if c.raw.bootstrap.ManagerImage != "" {
		out.bootstrap.ManagerImage = c.Bootstrap.ManagerImage
	}
	if c.raw.sandbox.Provider != "" {
		out.sandbox.Provider = c.Sandbox.Provider
	}
	if c.raw.sandbox.HomeDirName != "" {
		out.sandbox.HomeDirName = c.Sandbox.HomeDirName
	}
	if c.raw.sandbox.BoxLiteCLIPath != "" {
		out.sandbox.BoxLiteCLIPath = c.Sandbox.BoxLiteCLIPath
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

	if c.raw.channels.FeishuAdminOpenID != "" {
		out.channels.FeishuAdminOpenID = c.Channels.FeishuAdminOpenID
	}
	for name, rawFeishu := range c.raw.channels.Feishu {
		feishu := c.Channels.Feishu[name]
		loadedFeishu := FeishuConfig{}
		if rawFeishu.AppID != "" {
			loadedFeishu.AppID = feishu.AppID
		}
		if rawFeishu.AppSecret != "" {
			loadedFeishu.AppSecret = feishu.AppSecret
		}
		out.channels.Feishu[name] = loadedFeishu
	}

	return &out
}
