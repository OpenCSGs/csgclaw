package config

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"csgclaw/internal/apiclient"
)

type Config struct {
	Server    ServerConfig
	Models    LLMConfig
	LLM       LLMConfig
	Model     ModelConfig
	Bootstrap BootstrapConfig
	Channels  ChannelsConfig
}

type ServerConfig struct {
	ListenAddr       string
	AdvertiseBaseURL string
	AccessToken      string
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

type ChannelsConfig struct {
	FeishuAdminOpenID string
	Feishu            map[string]FeishuConfig
}

type FeishuConfig struct {
	AppID     string
	AppSecret string
}

const (
	AppDirName         = ".csgclaw"
	RuntimeHomeDirName = "boxlite"
	ConfigFileName     = "config.toml"
	StateFileName      = "state.json"
	AgentsDirName      = "agents"
	IMDirName          = "im"
	ChannelsDirName    = "channels"

	DefaultHTTPPort     = apiclient.DefaultHTTPPort
	DefaultAccessToken  = "your_access_token"
	DefaultManagerImage = "ghcr.io/russellluo/picoclaw:2026.4.8.1"
)

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
				cfg.Server.ListenAddr = value
			case "advertise_base_url":
				cfg.Server.AdvertiseBaseURL = strings.TrimRight(value, "/")
			case "access_token":
				cfg.Server.AccessToken = value
			}
		case section == "models":
			switch key {
			case "default":
				modelsCfg.Default = value
			}
		case section == "bootstrap":
			switch key {
			case "manager_image":
				cfg.Bootstrap.ManagerImage = value
			}
		case section == "channels.feishu":
			switch key {
			case "admin_open_id":
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
			switch key {
			case "app_id":
				feishu.AppID = value
			case "app_secret":
				feishu.AppSecret = value
			}
			cfg.Channels.Feishu[name] = feishu
		default:
			if name, ok := modelsProviderSectionName(section); ok {
				provider := modelsCfg.Providers[name]
				switch key {
				case "base_url":
					provider.BaseURL = value
				case "api_key":
					provider.APIKey = value
				case "models":
					models, parseErr := parseStringArray(rawValue)
					if parseErr != nil {
						return Config{}, fmt.Errorf("parse models.providers.%s.models: %w", name, parseErr)
					}
					provider.Models = models
				case "reasoning_effort":
					provider.ReasoningEffort = value
				}
				modelsCfg.Providers[name] = provider
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

	if !modelsCfg.IsZero() {
		cfg.Models = modelsCfg.Normalized()
	} else {
		cfg.Models = SingleProfileLLM(ModelConfig{})
	}
	cfg.LLM = cfg.Models
	cfg.syncModelFromLLM()
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

	var b strings.Builder
	fmt.Fprintf(&b, `# Generated by csgclaw onboard.

[server]
listen_addr = %q
advertise_base_url = %q
access_token = %q

[bootstrap]
manager_image = %q

[models]
default = %q
`, cfg.Server.ListenAddr, cfg.Server.AdvertiseBaseURL, cfg.Server.AccessToken, cfg.Bootstrap.ManagerImage, defaultSelector)

	for _, name := range sortedProviderNames(llmCfg.Providers) {
		provider := llmCfg.Providers[name].Resolved()
		fmt.Fprintf(&b, `
[models.providers.%s]
base_url = %q
api_key = %q
models = %s
`, name, provider.BaseURL, provider.APIKey, formatStringArray(provider.Models))
		if provider.ReasoningEffort != "" {
			fmt.Fprintf(&b, "reasoning_effort = %q\n", provider.ReasoningEffort)
		}
	}

	if strings.TrimSpace(c.Channels.FeishuAdminOpenID) != "" {
		fmt.Fprintf(&b, `
[channels.feishu]
admin_open_id = %q
`, c.Channels.FeishuAdminOpenID)
	}

	if len(c.Channels.Feishu) > 0 {
		names := make([]string, 0, len(c.Channels.Feishu))
		for name := range c.Channels.Feishu {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			feishu := c.Channels.Feishu[name]
			fmt.Fprintf(&b, `
[channels.feishu.%s]
app_id = %q
app_secret = %q
`, name, feishu.AppID, feishu.AppSecret)
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
	return strings.Trim(strings.TrimSpace(raw), `"`)
}

func parseStringArray(raw string) ([]string, error) {
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
		part = parseStringValue(part)
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
