package knowledgebase

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"csgclaw/internal/auth"
)

var ErrAuthenticationRequired = errors.New("OpenCSG sign-in is required to use knowledge base MCP servers")

// Connection identifies the current user and the trusted endpoints used by
// AgenticHub knowledge-base MCP servers.
type Connection struct {
	CSGHubBaseURL     string
	AIGatewayBaseURL  string
	CSGHubAccessToken string
}

// LoadConnection prefers an interactive OpenCSG login and falls back to the
// identity injected into a managed community-template runner.
func LoadConnection(ctx context.Context) (Connection, error) {
	interactive, authenticated, interactiveErr := LoadInteractiveConnection(ctx)
	if authenticated {
		return interactive, nil
	}
	if managed, ok := ManagedConnection(); ok {
		return managed, nil
	}
	if interactiveErr != nil {
		return Connection{}, interactiveErr
	}
	return Connection{}, ErrAuthenticationRequired
}

// LoadInteractiveConnection resolves the current desktop login. The boolean
// is false when no authenticated identity is stored.
func LoadInteractiveConnection(ctx context.Context) (Connection, bool, error) {
	return loadInteractiveConnection(ctx)
}

var loadInteractiveConnection = func(ctx context.Context) (Connection, bool, error) {
	baseURL, token, ok, err := auth.Default().Store.Credentials()
	if err != nil {
		return Connection{}, false, fmt.Errorf("read OpenCSG authentication: %w", err)
	}
	status, err := auth.Default().Status(ctx)
	if err != nil {
		return Connection{}, false, fmt.Errorf("read OpenCSG authentication status: %w", err)
	}
	if !ok || !status.Authenticated || strings.TrimSpace(token) == "" {
		return Connection{}, false, nil
	}
	return Connection{
		CSGHubBaseURL:     strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		AIGatewayBaseURL:  strings.TrimRight(strings.TrimSpace(status.AIGatewayBaseURL), "/"),
		CSGHubAccessToken: strings.TrimSpace(token),
	}, true, nil
}

// ManagedConnection resolves the current user identity supplied to a hosted
// community-template runner.
func ManagedConnection() (Connection, bool) {
	token := managedCSGHubAccessToken()
	baseURL := trustedCSGHubAPIBaseURL()
	if strings.TrimSpace(os.Getenv("CSGHUB_API_BASE_URL")) == "" {
		baseURL = auth.DefaultCSGHubBaseURL
	}
	if token == "" || baseURL == "" {
		return Connection{}, false
	}
	return Connection{
		CSGHubBaseURL:     baseURL,
		AIGatewayBaseURL:  managedAIGatewayBaseURL(baseURL),
		CSGHubAccessToken: token,
	}, true
}

func managedCSGHubAccessToken() string {
	for _, name := range []string{"CSGHUB_ACCESS_TOKEN", "CSGHUB_USER_TOKEN"} {
		if token := strings.TrimSpace(os.Getenv(name)); token != "" {
			return token
		}
	}
	return ""
}

func trustedCSGHubAPIBaseURL() string {
	raw := strings.TrimRight(strings.TrimSpace(os.Getenv("CSGHUB_API_BASE_URL")), "/")
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	return raw
}

func managedAIGatewayBaseURL(csgHubBaseURL string) string {
	for _, name := range []string{"CSGHUB_AIGATEWAY_BASE_URL", "CSGHUB_AIGATEWAY_URL", "CSGCLAW_LLM_BASE_URL"} {
		if baseURL := normalizeAIGatewayBaseURL(os.Getenv(name)); baseURL != "" {
			return baseURL
		}
	}
	switch strings.TrimRight(strings.TrimSpace(csgHubBaseURL), "/") {
	case auth.DefaultCSGHubBaseURL:
		return auth.DefaultAIGatewayBaseURL
	case auth.StageCSGHubBaseURL:
		return auth.StageAIGatewayBaseURL
	default:
		return normalizeAIGatewayBaseURL(csgHubBaseURL)
	}
}

func normalizeAIGatewayBaseURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	if !strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/v1") {
		parsed.Path = strings.TrimRight(parsed.Path, "/") + "/v1"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}
