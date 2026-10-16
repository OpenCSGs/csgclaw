package apps

import (
	"embed"
	"sort"
)

//go:embed builtin
var builtins embed.FS

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
	if pkg.Manifest.Kind == KindCLI {
		methods := []string{"none"}
		if len(pkg.CLI.Auth) > 0 {
			methods = []string{"command"}
		}

		return Definition{
			AppID: appID, Kind: KindCLI, Name: pkg.Manifest.Interface.DisplayName,
			Description: pkg.Manifest.Description, Interface: pkg.Manifest.Interface,
			Version: pkg.Manifest.Version, AuthMethods: methods, OAuthSupported: len(pkg.CLI.AuthorizationDomains) > 0 && len(pkg.CLI.Auth) > 0,
			ConfigSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		}, nil
	}
	authMethods := pkg.MCP.AuthMethods
	return Definition{Kind: KindMCP, AppID: appID, Name: pkg.Manifest.Interface.DisplayName, Description: pkg.Manifest.Description,
		Version: pkg.Manifest.Version, Interface: pkg.Manifest.Interface, ConfigSchema: configSchema(appID, pkg.MCP.config()),
		AuthMethods: authMethods, OAuthSupported: false}, nil
}

func configSchema(appID string, defaults Config) map[string]any {
	field := func(title string, secret bool) map[string]any {
		return map[string]any{"type": "string", "title": title, "writeOnly": secret}
	}
	config := normalizeConfig(defaults, appID)
	props := map[string]any{
		"url": field("MCP URL", false), "token": field("Token / API Key", true),
		"connector_id": field("Connector ID", false),
		"transport":    map[string]any{"type": "string", "enum": []string{"http", "stdio"}, "default": "http"},
		"command":      field("Command", false), "cwd": field("Working directory", false),
	}
	// Editable installation defaults. Local knowledge bases may instead be selected
	// through the knowledge picker; no credentials are embedded in these defaults.
	props["url"].(map[string]any)["default"] = defaults.URL
	props["command"].(map[string]any)["default"] = defaults.Command
	props["cwd"].(map[string]any)["default"] = defaults.CWD
	props["args"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "default": defaults.Args}
	props["transport"].(map[string]any)["default"] = config.Transport
	if defaults.AuthMode != "" {
		config.AuthMode = defaults.AuthMode
	}

	for key, value := range map[string]string{
		"auth_mode": config.AuthMode, "token_header": config.TokenHeader,
		"token_prefix": config.TokenPrefix, "token_env": config.TokenEnv,
		"app_id_env": config.AppIDEnv, "app_secret_env": config.AppSecretEnv,
	} {
		props[key] = map[string]any{"type": "string", "default": value}
	}
	props["startup_timeout_sec"] = map[string]any{"type": "integer", "default": config.StartupTimeoutSec}
	props["tool_timeout_sec"] = map[string]any{"type": "integer", "default": config.ToolTimeoutSec}
	if appID == "gitlab" {
		props["gitlab_base_url"] = field("GitLab instance URL", false)
	}
	return map[string]any{"type": "object", "properties": props}
}
