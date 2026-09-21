package apps

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

//go:embed builtin/*/*.json
var builtins embed.FS

type pluginPackage struct {
	Manifest Manifest                `json:"manifest"`
	Apps     map[string]AppReference `json:"apps"`
}

func loadPackages() (map[string]pluginPackage, error) {
	result := make(map[string]pluginPackage)
	entries, err := fs.ReadDir(builtins, "builtin")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		root := path.Join("builtin", entry.Name())
		data, err := builtins.ReadFile(path.Join(root, "plugin.json"))
		if err != nil {
			return nil, err
		}
		var pkg pluginPackage
		if err := json.Unmarshal(data, &pkg.Manifest); err != nil {
			return nil, err
		}
		ref := strings.TrimPrefix(pkg.Manifest.Apps, "./")
		if !fs.ValidPath(ref) || pkg.Manifest.Name == "" {
			return nil, fmt.Errorf("invalid built-in App manifest")
		}
		data, err = builtins.ReadFile(path.Join(root, ref))
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &pkg); err != nil {
			return nil, err
		}
		if len(pkg.Apps) != 1 {
			return nil, fmt.Errorf("built-in App must reference one service")
		}
		for _, app := range pkg.Apps {
			if !app.Required || (app.ID != "gitlab" && app.ID != "feishu" && app.ID != "llm-wiki") {
				return nil, fmt.Errorf("unknown built-in App service")
			}
			if _, exists := result[app.ID]; exists {
				return nil, fmt.Errorf("duplicate built-in App service")
			}
			result[app.ID] = pkg
		}
	}
	return result, nil
}

func (s *Service) Catalog() []Definition {
	items := make([]Definition, 0, len(s.packages))
	for id := range s.packages {
		item, _ := s.Definition(id)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].AppID < items[j].AppID })
	return items
}

func (s *Service) Definition(appID string) (Definition, error) {
	pkg, ok := s.packages[appID]
	if !ok {
		return Definition{}, ErrNotFound
	}
	authMethods := []string{"none", "bearer", "header", "env"}
	if appID == "feishu" {
		authMethods = append(authMethods, "feishu")
	} else if appID == "gitlab" {
		authMethods = []string{"connector"}
	}
	return Definition{AppID: appID, Name: pkg.Manifest.Interface.DisplayName, Description: pkg.Manifest.Description,
		Version: pkg.Manifest.Version, Interface: pkg.Manifest.Interface, ConfigSchema: configSchema(appID),
		AuthMethods: authMethods, OAuthSupported: false}, nil
}

func configSchema(appID string) map[string]any {
	field := func(title string, secret bool) map[string]any {
		return map[string]any{"type": "string", "title": title, "writeOnly": secret}
	}
	config := normalizeConfig(Config{}, appID)
	if appID == "feishu" {
		config.AuthMode = "feishu"
	}
	props := map[string]any{
		"url": field("MCP URL", false), "token": field("Token / API Key", true),
		"connector_id": field("Connector ID", false),
		"transport":    map[string]any{"type": "string", "enum": []string{"http", "stdio"}, "default": "http"},
		"command":      field("Command", false), "cwd": field("Working directory", false),
	}
	// Editable installation defaults. Local knowledge bases may instead be selected
	// through the knowledge picker; no credentials are embedded in these defaults.
	props["url"].(map[string]any)["default"] = map[string]string{
		"feishu":   "https://u-ryandraco-lark-mcp-passthrough-15s.public.opencsg-stg.com/mcp",
		"gitlab":   "https://u-wanghj-gitlab-mcp-161.public.opencsg-stg.com/mcp",
		"llm-wiki": "http://127.0.0.1:19093/mcp",
	}[appID]
	for key, value := range map[string]string{
		"auth_mode": config.AuthMode, "token_header": config.TokenHeader,
		"token_prefix": "Bearer ", "token_env": config.TokenEnv,
		"app_id_env": config.AppIDEnv, "app_secret_env": config.AppSecretEnv,
	} {
		props[key] = map[string]any{"type": "string", "default": value}
	}
	props["startup_timeout_sec"] = map[string]any{"type": "integer", "default": config.StartupTimeoutSec}
	props["tool_timeout_sec"] = map[string]any{"type": "integer", "default": config.ToolTimeoutSec}
	if appID == "feishu" {
		props["token"] = field("Platform access token (optional)", true)
		props["app_id"] = field("App ID", true)
		props["app_secret"] = field("App Secret", true)
		props["credential_source"] = map[string]any{"type": "string", "enum": []string{"feishu_channel", "manual"}, "default": "feishu_channel"}
	}
	if appID == "gitlab" {
		props["gitlab_base_url"] = field("GitLab instance URL", false)
	}
	return map[string]any{"type": "object", "properties": props}
}
