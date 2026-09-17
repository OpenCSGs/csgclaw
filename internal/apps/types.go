// Package apps manages per-agent App installations and their MCP connections.
// Plugin manifests are internal packaging metadata, not another public API.
package apps

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	ErrNotFound         = errors.New("app not found")
	ErrInvalid          = errors.New("invalid app configuration")
	ErrConflict         = errors.New("app name already exists")
	ErrUnsupportedOAuth = errors.New("OAuth2 authorization is not supported in this version")
)

type Interface struct {
	DisplayName       string          `json:"displayName,omitempty"`
	ShortDescription  string          `json:"shortDescription,omitempty"`
	LongDescription   string          `json:"longDescription,omitempty"`
	DeveloperName     string          `json:"developerName,omitempty"`
	Category          string          `json:"category,omitempty"`
	Capabilities      []string        `json:"capabilities,omitempty"`
	WebsiteURL        string          `json:"websiteURL,omitempty"`
	PrivacyPolicyURL  string          `json:"privacyPolicyURL,omitempty"`
	TermsOfServiceURL string          `json:"termsOfServiceURL,omitempty"`
	DefaultPrompt     json.RawMessage `json:"defaultPrompt,omitempty"`
	BrandColor        string          `json:"brandColor,omitempty"`
	ComposerIcon      string          `json:"composerIcon,omitempty"`
	Logo              string          `json:"logo,omitempty"`
	Screenshots       []string        `json:"screenshots,omitempty"`
}

type Manifest struct {
	Name        string            `json:"name"`
	Version     string            `json:"version,omitempty"`
	Description string            `json:"description,omitempty"`
	Author      map[string]string `json:"author,omitempty"`
	Homepage    string            `json:"homepage,omitempty"`
	Repository  string            `json:"repository,omitempty"`
	License     string            `json:"license,omitempty"`
	Keywords    []string          `json:"keywords,omitempty"`
	Apps        string            `json:"apps,omitempty"`
	Interface   Interface         `json:"interface"`
	MCPServers  json.RawMessage   `json:"mcpServers,omitempty"`
	Skills      json.RawMessage   `json:"skills,omitempty"`
	Hooks       json.RawMessage   `json:"hooks,omitempty"`
}

type AppReference struct {
	ID       string `json:"id"`
	Required bool   `json:"required"`
}

type Definition struct {
	AppID          string         `json:"app_id"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Version        string         `json:"version"`
	Interface      Interface      `json:"interface"`
	ConfigSchema   map[string]any `json:"config_schema"`
	AuthMethods    []string       `json:"auth_methods"`
	OAuthSupported bool           `json:"oauth_supported"`
}

// Config contains only displayable configuration. Secret headers and environment
// values belong in Credentials and are never returned by the service.
type Config struct {
	Transport         string            `json:"transport,omitempty"`
	URL               string            `json:"url,omitempty"`
	Command           string            `json:"command,omitempty"`
	Args              []string          `json:"args,omitempty"`
	CWD               string            `json:"cwd,omitempty"`
	Env               map[string]string `json:"env,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	AuthMode          string            `json:"auth_mode,omitempty"`
	CredentialSource  string            `json:"credential_source,omitempty"`
	TokenHeader       string            `json:"token_header,omitempty"`
	TokenPrefix       string            `json:"token_prefix,omitempty"`
	TokenEnv          string            `json:"token_env,omitempty"`
	AppIDHeader       string            `json:"app_id_header,omitempty"`
	AppSecretHeader   string            `json:"app_secret_header,omitempty"`
	AppIDEnv          string            `json:"app_id_env,omitempty"`
	AppSecretEnv      string            `json:"app_secret_env,omitempty"`
	StartupTimeoutSec int               `json:"startup_timeout_sec,omitempty"`
	ToolTimeoutSec    int               `json:"tool_timeout_sec,omitempty"`
}

type Credentials struct {
	Token     string            `json:"token,omitempty"`
	AppID     string            `json:"app_id,omitempty"`
	AppSecret string            `json:"app_secret,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

func (Credentials) String() string { return "[redacted]" }

type Installation struct {
	InstallationID string          `json:"installation_id"`
	AgentID        string          `json:"agent_id"`
	AppID          string          `json:"app_id"`
	Name           string          `json:"name"`
	Enabled        bool            `json:"enabled"`
	Disconnected   bool            `json:"disconnected"`
	Status         string          `json:"status"`
	LastError      string          `json:"last_error,omitempty"`
	Config         Config          `json:"config"`
	CredentialsSet map[string]bool `json:"credentials_set"`
	Tools          []*mcp.Tool     `json:"tools"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type CreateRequest struct {
	AppID       string      `json:"app_id"`
	Name        string      `json:"name"`
	Config      Config      `json:"config"`
	Credentials Credentials `json:"credentials"`
	Connect     bool        `json:"connect"`
}

type UpdateRequest struct {
	Name        *string      `json:"name,omitempty"`
	Enabled     *bool        `json:"enabled,omitempty"`
	Config      *Config      `json:"config,omitempty"`
	Credentials *Credentials `json:"credentials,omitempty"`
}

type ProbeRequest struct {
	AppID          string      `json:"app_id"`
	InstallationID string      `json:"installation_id,omitempty"`
	Config         Config      `json:"config"`
	Credentials    Credentials `json:"credentials"`
}

type ProbeResult struct {
	Connected       bool                `json:"connected"`
	Tools           []*mcp.Tool         `json:"tools"`
	ServerInfo      *mcp.Implementation `json:"server_info,omitempty"`
	ProtocolVersion string              `json:"protocol_version,omitempty"`
}

type FeishuCredentials struct{ AppID, AppSecret string }

func (FeishuCredentials) String() string { return "[redacted]" }

type Options struct {
	ReadOnly         func(string) bool
	ResolveFeishu    func(context.Context, string) (FeishuCredentials, error)
	OnCatalogChanged func(string, uint64)
}
