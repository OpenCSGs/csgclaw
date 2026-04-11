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
	DefaultLLMProfile   = "default"
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

	cfg := Config{
		LLM: LLMConfig{
			Profiles: make(map[string]ModelConfig),
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
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("invalid line: %q", line)
		}

		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"`)
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
		case section == "llm":
			switch key {
			case "default_profile":
				cfg.LLM.DefaultProfile = value
			}
		case section == "model":
			switch key {
			case "provider":
				cfg.Model.Provider = value
			case "base_url":
				cfg.Model.BaseURL = value
			case "api_key":
				cfg.Model.APIKey = value
			case "model_id":
				cfg.Model.ModelID = value
			case "reasoning_effort":
				cfg.Model.ReasoningEffort = value
			}
		default:
			if name, ok := llmProfileSectionName(section); ok {
				profile := cfg.LLM.Profiles[name]
				switch key {
				case "provider":
					profile.Provider = value
				case "base_url":
					profile.BaseURL = value
				case "api_key":
					profile.APIKey = value
				case "model_id":
					profile.ModelID = value
				case "reasoning_effort":
					profile.ReasoningEffort = value
				}
				cfg.LLM.Profiles[name] = profile
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
	cfg.Model = cfg.Model.Resolved()
	if len(cfg.LLM.Profiles) == 0 {
		cfg.LLM = SingleProfileLLM(cfg.Model)
	} else {
		cfg.LLM = cfg.LLM.Normalized()
	}
	cfg.syncModelFromLLM()
	return cfg, nil
}

func (c Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	cfg := c
	cfg.syncModelFromLLM()
	llmCfg := cfg.LLM.Normalized()
	if len(llmCfg.Profiles) == 0 {
		llmCfg = SingleProfileLLM(cfg.Model)
	}
	defaultProfile := llmCfg.EffectiveDefaultProfile()
	if defaultProfile == "" {
		defaultProfile = DefaultLLMProfile
	}

	var b strings.Builder
	fmt.Fprintf(&b, `# Generated by csgclaw onboard.

[server]
listen_addr = %q
advertise_base_url = %q
access_token = %q

[llm]
default_profile = %q

[bootstrap]
manager_image = %q
`, cfg.Server.ListenAddr, cfg.Server.AdvertiseBaseURL, cfg.Server.AccessToken, defaultProfile, cfg.Bootstrap.ManagerImage)

	for _, name := range sortedProfileNames(llmCfg.Profiles) {
		profile := llmCfg.Profiles[name].Resolved()
		fmt.Fprintf(&b, `
[llm.profiles.%s]
provider = %q
base_url = %q
api_key = %q
model_id = %q
reasoning_effort = %q
`, name, profile.EffectiveProvider(), profile.BaseURL, profile.APIKey, profile.ModelID, profile.ReasoningEffort)
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

func llmProfileSectionName(section string) (string, bool) {
	const prefix = "llm.profiles."
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
	return LLMConfig{
		DefaultProfile: DefaultLLMProfile,
		Profiles: map[string]ModelConfig{
			DefaultLLMProfile: model.Resolved(),
		},
	}
}

func (c *Config) syncModelFromLLM() {
	if c == nil {
		return
	}
	c.LLM = c.LLM.Normalized()
	if len(c.LLM.Profiles) == 0 {
		c.LLM = SingleProfileLLM(c.Model)
	}
	name, model, err := c.LLM.Resolve("")
	if err != nil {
		c.Model = c.Model.Resolved()
		return
	}
	c.LLM.DefaultProfile = name
	c.Model = model.Resolved()
}

func sortedProfileNames(profiles map[string]ModelConfig) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
