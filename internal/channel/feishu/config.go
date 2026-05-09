package feishu

import (
	"errors"
	"sort"
	"strings"
	"sync"

	"csgclaw/internal/config"
)

type Config struct {
	mu         sync.RWMutex
	configPath string
}

type Update struct {
	BotID       string
	AppID       string
	AppSecret   string
	AdminOpenID string
}

type Entry struct {
	BotID       string
	Configured  bool
	AppID       string
	HasSecret   bool
	AdminOpenID string
}

type AppConfig struct {
	AppID       string
	AppSecret   string
	AdminOpenID string
}

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

func IsValidationError(err error) bool {
	var validationErr ValidationError
	return errors.As(err, &validationErr)
}

func NewConfig(configPath string) *Config {
	return &Config{
		configPath: strings.TrimSpace(configPath),
	}
}

func (c *Config) SetPath(path string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.configPath = strings.TrimSpace(path)
}

func (c *Config) Get(botID string) (Entry, error) {
	botID, err := normalizeConfigBotID(botID)
	if err != nil {
		return Entry{}, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	cfg, err := c.loadConfigWithChannelFiles()
	if err != nil {
		return Entry{}, err
	}
	app, ok := cfg.Channels.Feishu[botID]
	return MaskConfig(botID, app, ok, cfg.Channels.FeishuAdminOpenID), nil
}

func (c *Config) Update(req Update) (Entry, error) {
	botID, appID, appSecret, adminOpenID, err := normalizeConfigUpdate(req)
	if err != nil {
		return Entry{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	channels, err := c.loadStandaloneFeishuChannelConfig()
	if err != nil {
		return Entry{}, err
	}
	if channels.Feishu == nil {
		channels.Feishu = make(map[string]config.FeishuConfig)
	}
	if adminOpenID != "" {
		channels.FeishuAdminOpenID = adminOpenID
	}
	channels.Feishu[botID] = config.FeishuConfig{AppID: appID, AppSecret: appSecret}

	feishuPath, err := config.FeishuChannelConfigPath(c.configPath)
	if err != nil {
		return Entry{}, err
	}
	if err := config.SaveFeishuChannelConfig(feishuPath, channels); err != nil {
		return Entry{}, err
	}
	return MaskConfig(botID, channels.Feishu[botID], true, channels.FeishuAdminOpenID), nil
}

func (c *Config) Load() (config.Config, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.loadConfigWithChannelFiles()
}

func normalizeConfigUpdate(req Update) (string, string, string, string, error) {
	botID, err := normalizeConfigBotID(req.BotID)
	if err != nil {
		return "", "", "", "", err
	}
	appID := strings.TrimSpace(req.AppID)
	if appID == "" {
		return "", "", "", "", ValidationError{Message: "app_id is required"}
	}
	appSecret := strings.TrimSpace(req.AppSecret)
	if appSecret == "" {
		return "", "", "", "", ValidationError{Message: "app_secret is required"}
	}
	return botID, appID, appSecret, strings.TrimSpace(req.AdminOpenID), nil
}

func normalizeConfigBotID(botID string) (string, error) {
	botID = strings.TrimSpace(botID)
	if err := config.ValidateFeishuChannelBotID(botID); err != nil {
		return "", ValidationError{Message: err.Error()}
	}
	return botID, nil
}

func (c *Config) loadConfigWithChannelFiles() (config.Config, error) {
	if strings.TrimSpace(c.configPath) == "" {
		return config.LoadDefaultWithChannelFiles()
	}
	return config.LoadWithChannelFiles(c.configPath)
}

func (c *Config) loadStandaloneFeishuChannelConfig() (config.ChannelsConfig, error) {
	path, err := config.FeishuChannelConfigPath(c.configPath)
	if err != nil {
		return config.ChannelsConfig{}, err
	}
	channels, ok, err := config.LoadFeishuChannelConfigIfExists(path)
	if err != nil {
		return config.ChannelsConfig{}, err
	}
	if !ok {
		return config.ChannelsConfig{}, nil
	}
	return channels, nil
}

func MaskConfig(botID string, app config.FeishuConfig, configured bool, adminOpenID string) Entry {
	view := Entry{
		BotID:       botID,
		Configured:  configured,
		AdminOpenID: strings.TrimSpace(adminOpenID),
	}
	if configured {
		view.AppID = strings.TrimSpace(app.AppID)
		view.HasSecret = strings.TrimSpace(app.AppSecret) != ""
	}
	return view
}

func AppsFromChannels(cfg config.ChannelsConfig) map[string]AppConfig {
	apps := make(map[string]AppConfig, len(cfg.Feishu))
	for name, app := range cfg.Feishu {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		apps[name] = AppConfig{
			AppID:       app.AppID,
			AppSecret:   app.AppSecret,
			AdminOpenID: cfg.FeishuAdminOpenID,
		}
	}
	return apps
}

func SortedBotIDs(channels config.ChannelsConfig) []string {
	ids := make([]string, 0, len(channels.Feishu))
	for id := range channels.Feishu {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
