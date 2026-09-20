package apps

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"csgclaw/internal/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type OpenCSGCredentials struct{ BaseURL, Token string }

func (OpenCSGCredentials) String() string { return "[redacted]" }

func loadOpenCSGCredentials(ctx context.Context) (OpenCSGCredentials, error) {
	if err := ctx.Err(); err != nil {
		return OpenCSGCredentials{}, err
	}
	store, err := auth.DefaultStore()
	if err != nil {
		return OpenCSGCredentials{}, connectionError("app_opencsg_login_required", "Cannot read the OpenCSG login. Sign in from Settings and reconnect the App.", 0, true)
	}
	record, ok, err := store.Load()
	if err != nil || !ok || !record.Status().Authenticated {
		return OpenCSGCredentials{}, connectionError("app_opencsg_login_required", "OpenCSG is not signed in or its login has expired. Sign in from Settings and reconnect the App.", 0, true)
	}
	return OpenCSGCredentials{BaseURL: record.Account.OpenCSGBaseURL, Token: record.Tokens.AccessToken}, nil
}

// OpenCSG platform credentials are only eligible for first-party HTTPS ingress.
func defaultPlatformCredentialSource(config Config) string {
	if config.Transport == "stdio" || config.AuthMode == "env" || config.AuthMode == "oauth2" {
		return "manual"
	}
	u, err := url.Parse(config.URL)
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return "manual"
	}
	host := strings.ToLower(u.Hostname())
	for _, base := range []string{"opencsg.com", "opencsg-stg.com"} {
		if host == base || strings.HasSuffix(host, ".public."+base) {
			return "opencsg_login"
		}
	}
	return "manual"
}

func (s *Service) openCSGToken(ctx context.Context, endpoint string) (string, error) {
	target, err := url.Parse(endpoint)
	if err != nil || target.Scheme != "https" || target.User != nil || (target.Port() != "" && target.Port() != "443") {
		return "", connectionError("app_opencsg_endpoint_untrusted", "OpenCSG login credentials can only be used with HTTPS OpenCSG service addresses.", 0, false)
	}
	load := s.options.OpenCSGCredentials
	if load == nil {
		load = loadOpenCSGCredentials
	}
	credentials, err := load(ctx)
	if err != nil {
		return "", err
	}
	if credentials.Token == "" || expiredPlatformToken(credentials.Token) {
		return "", connectionError("app_opencsg_login_required", "OpenCSG is not signed in or its login has expired. Sign in from Settings and reconnect the App.", 0, true)
	}
	environment, err := url.Parse(credentials.BaseURL)
	if err != nil || environment.Scheme != "https" || (environment.Hostname() != "opencsg.com" && environment.Hostname() != "opencsg-stg.com") {
		return "", connectionError("app_opencsg_environment_mismatch", "The current OpenCSG login environment does not match this MCP service. Select the matching environment in Settings.", 0, false)
	}
	host := strings.ToLower(target.Hostname())
	base := environment.Hostname()
	if host != base && !strings.HasSuffix(host, ".public."+base) {
		return "", connectionError("app_opencsg_environment_mismatch", "The current OpenCSG login environment does not match this MCP service. Select the matching environment in Settings or use a manual token.", 0, false)
	}
	return credentials.Token, nil
}

func (s *Service) bindPlatformLogin(config Config, client *http.Client) {
	if config.PlatformCredentialSource == "opencsg_login" {
		client.Transport.(*headerTransport).platformToken = func(ctx context.Context) (string, error) { return s.openCSGToken(ctx, config.URL) }
	}
}
func (s *Service) bindTransportPlatformLogin(config Config, transport mcp.Transport) {
	if httpTransport, ok := transport.(*mcp.StreamableClientTransport); ok {
		s.bindPlatformLogin(config, httpTransport.HTTPClient)
	}
}

func clearReferencedPlatformCredentials(config Config, credentials *Credentials) {
	if config.PlatformCredentialSource != "opencsg_login" {
		return
	}
	// Connector tokens are business credentials (for example a GitLab PAT),
	// not a copied OpenCSG platform token. They must remain available while
	// Authorization is supplied independently from the current platform login.
	if config.AuthMode != "header" && config.AuthMode != "connector" {
		credentials.Token = ""
	}
	for key := range credentials.Headers {
		if strings.EqualFold(key, "Authorization") {
			delete(credentials.Headers, key)
		}
	}
}

// Authentication callbacks refresh only opted-in Apps. Explicitly disconnected
// or disabled instances are never revived by a global account change.
func (s *Service) RefreshPlatformCredentials(ctx context.Context) error {
	s.mu.Lock()
	var refs [][2]string
	for id, e := range s.entries {
		if e.record.Config.PlatformCredentialSource == "opencsg_login" && e.record.active() && !e.record.Disconnected && e.record.ConnectRequested {
			refs = append(refs, [2]string{e.record.AgentID, id})
		}
	}
	s.mu.Unlock()
	var failed bool
	for _, ref := range refs {
		if _, err := s.connect(ctx, ref[0], ref[1], false); err != nil {
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("some Apps require OpenCSG sign-in or service access")
	}
	return nil
}
