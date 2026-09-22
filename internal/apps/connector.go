package apps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	feishutransport "csgclaw/internal/channel/feishu/transport"
	"csgclaw/internal/mcpschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var quietLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

var errAuthentication = errors.New("App authorization is no longer valid")

type connection struct {
	httpAuth   *headerTransport
	tokens     feishutransport.TenantTokenSource
	ctx        context.Context
	session    *mcp.ClientSession
	cancel     context.CancelFunc
	tools      []*mcp.Tool
	toolNames  []string
	generation uint64
	refreshMu  chan struct{}
}

func (c *connection) close() {
	if c != nil {
		c.cancel()
		_ = c.session.Close()
	}
}

func normalizeConfig(c Config, appID string, supplied ...Credentials) Config {
	if c.Transport == "" {
		if c.Command != "" {
			c.Transport = "stdio"
		} else {
			c.Transport = "http"
		}
	}
	if c.PlatformCredentialSource == "" {
		c.PlatformCredentialSource = defaultPlatformCredentialSource(c)
		if len(supplied) > 0 && supplied[0].Token != "" && (c.AuthMode == "" || c.AuthMode == "bearer" || c.AuthMode == "feishu") {
			c.PlatformCredentialSource = "manual"
		}
	}
	if c.CredentialSource == "" {
		c.CredentialSource = "manual"
	}
	if (c.AuthMode == "oauth2" || c.AuthMode == "connector") && c.ConnectorID == "" && appID == "gitlab" {
		c.ConnectorID = "gitlab"
	}
	if (c.AuthMode == "oauth2" || c.AuthMode == "connector") && appID == "gitlab" {
		c.AuthMode = "connector"
		c.GitLabBaseURL = strings.TrimRight(strings.TrimSpace(c.GitLabBaseURL), "/")
	}
	if c.AuthMode == "" {
		c.AuthMode = "bearer"
	}
	if c.TokenHeader == "" {
		c.TokenHeader = "Authorization"
	}
	if c.TokenEnv == "" {
		c.TokenEnv = "MCP_ACCESS_TOKEN"
		if appID == "gitlab" {
			c.TokenEnv = "GITLAB_PERSONAL_ACCESS_TOKEN"
		}
	}
	if c.AppIDEnv == "" {
		c.AppIDEnv = "FEISHU_APP_ID"
	}
	if c.AppSecretEnv == "" {
		c.AppSecretEnv = "FEISHU_APP_SECRET"
	}
	if c.StartupTimeoutSec <= 0 {
		c.StartupTimeoutSec = 30
	}
	if c.ToolTimeoutSec <= 0 {
		c.ToolTimeoutSec = 60
	}
	return c
}

func validateConfig(c Config, appID string) error {
	if c.PlatformCredentialSource != "" && c.PlatformCredentialSource != "manual" && c.PlatformCredentialSource != "opencsg_login" {
		return fmt.Errorf("%w: unsupported platform credential source", ErrInvalid)
	}
	if c.PlatformCredentialSource == "opencsg_login" {
		if c.Transport != "http" || c.AuthMode == "env" || c.AuthMode == "oauth2" {
			return fmt.Errorf("%w: OpenCSG login requires HTTP authentication", ErrInvalid)
		}
		if c.AuthMode == "header" && strings.EqualFold(c.TokenHeader, "Authorization") {
			return errors.Join(ErrInvalid, connectionError("app_platform_header_conflict", "OpenCSG uses the Authorization header. Put the service API key in a separate header, such as PRIVATE-TOKEN or X-API-Key.", 0, false))
		}
	}
	if c.AuthMode == "feishu" && appID != "feishu" {
		return fmt.Errorf("%w: Feishu application authentication is only available for the Feishu App", ErrInvalid)
	}
	if c.AuthMode == "oauth2" || c.AuthMode == "connector" {
		if appID != "gitlab" || c.Transport != "http" || strings.TrimSpace(c.ConnectorID) == "" || strings.TrimSpace(c.GitLabBaseURL) == "" {
			return ErrUnsupportedOAuth
		}
		base, err := url.Parse(c.GitLabBaseURL)
		if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
			return fmt.Errorf("%w: GitLab base URL must be an absolute HTTP(S) URL", ErrInvalid)
		}
	}
	if c.StartupTimeoutSec > 120 || c.ToolTimeoutSec > 600 {
		return fmt.Errorf("%w: timeout is too large", ErrInvalid)
	}
	if c.CredentialSource != "manual" {
		return errors.Join(ErrInvalid, connectionError("app_global_credentials_required", "Configure credentials on the global connector. Agent channel credentials cannot be referenced.", 0, true))
	}

	switch c.AuthMode {
	case "none", "bearer", "header", "env", "feishu", "oauth2", "connector":
	default:
		return fmt.Errorf("%w: authentication mode is unsupported", ErrInvalid)
	}
	switch c.Transport {
	case "http":
		u, err := url.Parse(c.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" {
			return fmt.Errorf("%w: MCP URL must be an HTTP(S) URL without user information or fragment", ErrInvalid)
		}
		if c.Command != "" {
			return fmt.Errorf("%w: HTTP connection cannot include a command", ErrInvalid)
		}
		if c.AuthMode == "env" {
			return fmt.Errorf("%w: environment authentication requires stdio", ErrInvalid)
		}
	case "stdio":
		if strings.TrimSpace(c.Command) == "" || c.URL != "" {
			return fmt.Errorf("%w: stdio requires a command and no URL", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: transport must be http or stdio", ErrInvalid)
	}
	return nil
}

func (s *Service) resolve(ctx context.Context, agentID string, c Config, credentials Credentials) (Credentials, error) {

	if c.AuthMode != "none" && c.AuthMode != "feishu" && c.AuthMode != "oauth2" && c.AuthMode != "connector" &&
		(c.PlatformCredentialSource != "opencsg_login" || c.AuthMode == "header") && credentials.Token == "" {
		return Credentials{}, fmt.Errorf("%w: token is required", ErrInvalid)
	}
	if c.AuthMode == "feishu" && (credentials.AppID == "" || credentials.AppSecret == "") {
		return Credentials{}, fmt.Errorf("%w: App ID and App Secret are required", ErrInvalid)
	}
	clearReferencedPlatformCredentials(c, &credentials)
	return credentials, nil
}

func (s *Service) buildTransport(ctx context.Context, agentID, appID string, c Config, credentials Credentials, dirs processDirs, tokenSources ...feishutransport.TenantTokenSource) (mcp.Transport, error) {
	if c.Transport == "stdio" {
		vars := map[string]string{}
		// Deliberately inherit only process essentials, never the host's secrets.
		for _, key := range []string{"PATH", "LANG", "SYSTEMROOT", "WINDIR"} {
			if v, ok := os.LookupEnv(key); ok {
				vars[key] = v
			}
		}
		for k, v := range c.Env {
			vars[k] = v
		}
		for k, v := range credentials.Env {
			vars[k] = v
		}
		if credentials.Token != "" && c.AuthMode != "none" && c.AuthMode != "feishu" {
			vars[c.TokenEnv] = credentials.Token
		}
		if c.AuthMode == "feishu" {
			vars[c.AppIDEnv] = credentials.AppID
			vars[c.AppSecretEnv] = credentials.AppSecret
		}
		if dirs.Home == "" || dirs.Data == "" || dirs.Plugin == "" {
			return nil, fmt.Errorf("App process directories are unavailable")
		}
		vars["HOME"] = dirs.Home
		vars["USERPROFILE"] = dirs.Home
		vars["PLUGIN_DATA"] = dirs.Data
		vars["PLUGIN_ROOT"] = dirs.Plugin
		vars["TMPDIR"] = dirs.Temp
		vars["TMP"] = dirs.Temp
		vars["TEMP"] = dirs.Temp
		vars["XDG_CONFIG_HOME"] = filepath.Join(dirs.Home, ".config")
		vars["XDG_CACHE_HOME"] = filepath.Join(dirs.Home, ".cache")
		vars["XDG_DATA_HOME"] = filepath.Join(dirs.Home, ".local", "share")
		for k, v := range vars {
			if k == "" || strings.ContainsAny(k, "=\x00") || strings.ContainsRune(v, 0) {
				return nil, fmt.Errorf("%w: invalid environment variable", ErrInvalid)
			}
		}
		cmd := exec.Command(c.Command, c.Args...)
		cmd.Dir = c.CWD
		if cmd.Dir == "" {
			cmd.Dir = dirs.Data
		}
		cmd.Env = make([]string, 0, len(vars))
		for k, v := range vars {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		sort.Strings(cmd.Env)
		cmd.Stderr = io.Discard
		return &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil
	}
	if c.AuthMode == "connector" {
		var resolved ConnectorHTTPConfig
		if appID == "gitlab" && strings.TrimSpace(credentials.Token) != "" {
			resolved = ConnectorHTTPConfig{
				Endpoint: c.URL, Token: credentials.Token, TokenHeader: "PRIVATE-TOKEN",
				Headers: http.Header{"X-GitLab-Base-URL": []string{c.GitLabBaseURL}},
			}
		} else {
			if s.options.ResolveConnectorHTTP == nil {
				return nil, ErrUnsupportedOAuth
			}
			var err error
			resolved, err = s.options.ResolveConnectorHTTP(ctx, agentID, appID, c)
			if err != nil {
				return nil, err
			}
		}
		direct := c
		direct.URL = resolved.Endpoint
		direct.AuthMode = "none"
		direct.Headers = make(map[string]string, len(resolved.Headers)+len(credentials.Headers)+1)
		for key, values := range resolved.Headers {
			if len(values) > 0 {
				setHeaderValue(direct.Headers, key, values[0])
			}
		}
		if resolved.TokenHeader != "" {
			setHeaderValue(direct.Headers, resolved.TokenHeader, resolved.TokenPrefix+resolved.Token)
		}
		for key, value := range credentials.Headers {
			setHeaderValue(direct.Headers, key, value)
		}
		client, err := connectorHTTPClient(direct, Credentials{})
		if err != nil {
			return nil, err
		}
		return &mcp.StreamableClientTransport{Endpoint: resolved.Endpoint, HTTPClient: client, MaxRetries: -1}, nil
	}
	client, err := connectorHTTPClient(c, credentials, tokenSources...)
	if err != nil {
		return nil, err
	}
	return &mcp.StreamableClientTransport{Endpoint: c.URL, HTTPClient: client, MaxRetries: -1}, nil
}

func setHeaderValue(headers map[string]string, name, value string) {
	for key := range headers {
		if strings.EqualFold(key, name) {
			delete(headers, key)
		}
	}
	headers[name] = value
}

func connectorHTTPClient(c Config, credentials Credentials, tokenSources ...feishutransport.TenantTokenSource) (*http.Client, error) {
	headers := http.Header{}
	set := func(k, v string) error {
		if !mcpschema.ValidMCPHTTPHeaderName(k) || strings.ContainsAny(v, "\r\n") || reservedHeader(k) {
			return fmt.Errorf("%w: invalid or reserved authentication header", ErrInvalid)
		}
		headers.Set(k, v)
		return nil
	}
	for k, v := range c.Headers {
		if err := set(k, v); err != nil {
			return nil, err
		}
	}
	for k, v := range credentials.Headers {
		if err := set(k, v); err != nil {
			return nil, err
		}
	}
	switch c.AuthMode {
	case "bearer":
		if c.PlatformCredentialSource != "opencsg_login" {
			if err := set("Authorization", "Bearer "+credentials.Token); err != nil {
				return nil, err
			}
		}
	case "header":
		if err := set(c.TokenHeader, c.TokenPrefix+credentials.Token); err != nil {
			return nil, err
		}
	case "feishu":
		// This mode uses application credentials to obtain a TAT locally.
		// Only the optional platform token and generated TAT go upstream.
		headers.Del("Authorization")
		if credentials.Token != "" {
			if err := set("Authorization", "Bearer "+credentials.Token); err != nil {
				return nil, err
			}
		}
		headers.Del("lark-access-token")
		headers.Set("X-Lark-Token-Type", "tenant_access_token")

	}
	var tokens feishutransport.TenantTokenSource
	if c.AuthMode == "feishu" {
		if len(tokenSources) > 0 {
			tokens = tokenSources[0]
		} else {
			tokens = feishutransport.NewTenantTokenSource(credentials.AppID, credentials.AppSecret)
		}
		if tokens == nil {
			return nil, fmt.Errorf("Feishu token source is unavailable")
		}
	}
	u, _ := url.Parse(c.URL)
	client := &http.Client{Transport: &headerTransport{base: http.DefaultTransport, headers: headers, origin: u.Scheme + "://" + u.Host, tokens: tokens, manualToken: platformManualToken(c, credentials)}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return client, nil
}

func reservedHeader(name string) bool {
	n := strings.ToLower(name)
	return n == "host" || n == "content-type" || n == "accept" || strings.HasPrefix(n, "mcp-") || n == "content-length" || n == "connection" || n == "transfer-encoding"
}

type feishuCallTokenKey struct{}

type headerTransport struct {
	failureMu     sync.Mutex
	failure       *ConnectionError
	platformToken func(context.Context) (string, error)
	manualToken   string
	tokens        feishutransport.TenantTokenSource
	base          http.RoundTripper
	headers       http.Header
	origin        string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header = req.Header.Clone()
	if cloned.URL.Scheme+"://"+cloned.URL.Host != t.origin {
		return nil, fmt.Errorf("MCP request origin changed")
	}
	for k, v := range t.headers {
		cloned.Header[k] = append([]string(nil), v...)
	}
	if t.platformToken != nil {
		token, err := t.platformToken(req.Context())
		if err != nil {
			return nil, t.captureFailure(err)
		}
		cloned.Header.Set("Authorization", "Bearer "+token)
	} else if expiredPlatformToken(t.manualToken) {
		return nil, t.captureFailure(connectionError("app_platform_token_expired", "The configured platform access token has expired. Update it or reuse the current OpenCSG login.", 401, true))
	}
	if t.tokens != nil {
		token, _ := req.Context().Value(feishuCallTokenKey{}).(string)
		if token == "" {
			var err error
			token, err = t.tokens.Token(req.Context())
			if err != nil {
				return nil, t.captureFailure(connectionError("app_feishu_token_failed", "Cannot obtain a Feishu application token. Check the connector App ID/App Secret, application status and Feishu connectivity.", 0, true))
			}
		}
		cloned.Header.Set("lark-access-token", token)
		cloned.Header.Set("X-Lark-Token-Type", "tenant_access_token")
	}
	response, err := t.base.RoundTrip(cloned)
	if err == nil {
		if response.StatusCode == 401 || response.StatusCode == 403 || response.StatusCode >= 500 {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
			response.Body.Close()
			detail := upstreamHTTPError(response.StatusCode, body, t.platformToken != nil)
			t.rememberFailure(detail)
			return nil, detail
		}
		// GET/DELETE 405 and session 404 have defined MCP transport meanings.
		// Let the SDK handle them instead of converting them into network errors.
		if response.StatusCode == 404 || (response.StatusCode == 405 && req.Method == http.MethodPost) {
			t.rememberFailure(upstreamHTTPError(response.StatusCode, nil, t.platformToken != nil))
		} else if response.StatusCode < 400 {
			t.rememberFailure(nil)
		}
	} else if req.Context().Err() == nil {
		t.rememberFailure(connectionError("app_mcp_network_error", "Cannot reach the MCP service. Check its address, network connection and running state.", 0, false))
	}
	return response, err
}

func (s *Service) open(ctx context.Context, agentID, appID string, c Config, credentials Credentials, dirs processDirs, onToolsChanged func()) (*connection, *ProbeResult, error) {
	if c.PlatformCredentialSource == "opencsg_login" {
		if _, err := s.openCSGToken(ctx, c.URL); err != nil {
			logMCPConnectionFailure("platform_auth", agentID, appID, c, credentials, err, nil)
			return nil, nil, err
		}
	}
	if c.PlatformCredentialSource != "opencsg_login" && expiredPlatformToken(platformManualToken(c, credentials)) {
		return nil, nil, connectionError("app_platform_token_expired", "The configured platform access token has expired. Update it or reuse the current OpenCSG login.", 401, true)
	}
	resolved, err := s.resolve(ctx, agentID, c, credentials)
	if err != nil {
		logMCPConnectionFailure("resolve_credentials", agentID, appID, c, credentials, err, nil)
		return nil, nil, err
	}
	var tokens feishutransport.TenantTokenSource
	if c.Transport == "http" && c.AuthMode == "feishu" {
		factory := s.options.FeishuTokenSource
		if factory == nil {
			factory = feishutransport.NewTenantTokenSource
		}
		tokens = factory(resolved.AppID, resolved.AppSecret)
		if tokens == nil {
			return nil, nil, fmt.Errorf("Feishu token source is unavailable")
		}
		tokenCtx, tokenCancel := context.WithTimeout(ctx, time.Duration(c.StartupTimeoutSec)*time.Second)
		_, tokenErr := tokens.Token(tokenCtx)
		tokenCancel()
		if tokenErr != nil {
			return nil, nil, connectionError("app_feishu_token_failed", "Cannot obtain a Feishu application token. Check the connector App ID/App Secret, application status and Feishu connectivity.", 0, true)
		}
	}
	transport, err := s.buildTransport(ctx, agentID, appID, c, resolved, dirs, tokens)
	if err != nil {
		logMCPConnectionFailure("build_transport", agentID, appID, c, credentials, err, nil)
		return nil, nil, err
	}
	s.bindTransportPlatformLogin(c, transport)
	lifetime, cancel := context.WithCancel(s.ctx)
	stopRequest := context.AfterFunc(ctx, cancel)
	timer := time.AfterFunc(time.Duration(c.StartupTimeoutSec)*time.Second, cancel)
	client := mcp.NewClient(&mcp.Implementation{Name: "csgclaw-app", Version: "1.0.0"}, &mcp.ClientOptions{
		Logger: quietLogger, Capabilities: &mcp.ClientCapabilities{},
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) {
			if onToolsChanged != nil {
				onToolsChanged()
			}
		},
	})
	session, err := client.Connect(lifetime, transport, nil)
	timer.Stop()
	stopRequest()
	if err != nil {
		cancel()
		detail := transportFailure(transport, err)
		logMCPConnectionFailure("initialize", agentID, appID, c, credentials, err, detail)
		if detail != nil {
			return nil, nil, detail
		}
		if errors.Is(err, errAuthentication) {
			return nil, nil, errAuthentication
		}
		return nil, nil, fmt.Errorf("MCP connection failed; verify the service address and credentials")
	}
	conn := &connection{httpAuth: transportHTTPAuth(transport), ctx: lifetime, session: session, cancel: cancel, refreshMu: make(chan struct{}, 1), tokens: tokens}
	if lifetime.Err() != nil || ctx.Err() != nil {
		conn.close()
		return nil, nil, fmt.Errorf("MCP connection was cancelled or timed out")
	}

	listCtx, listCancel := context.WithTimeout(ctx, time.Duration(c.ToolTimeoutSec)*time.Second)
	defer listCancel()
	tools, err := listTools(listCtx, session)
	if err != nil {
		conn.close()
		detail := transportFailure(transport, err)
		logMCPConnectionFailure("tools_list", agentID, appID, c, credentials, err, detail)
		if detail != nil {
			return nil, nil, detail
		}
		return nil, nil, fmt.Errorf("MCP tool discovery failed; verify the service permissions")
	}
	conn.tools = tools
	result := &ProbeResult{Connected: true, Tools: tools}
	if init := session.InitializeResult(); init != nil {
		result.ServerInfo = init.ServerInfo
		result.ProtocolVersion = init.ProtocolVersion
	}
	return conn, result, nil
}

// logMCPConnectionFailure records enough context to diagnose connection stages
// without logging URLs with query strings, credentials, or header values.
func logMCPConnectionFailure(stage, agentID, appID string, c Config, credentials Credentials, err error, detail *ConnectionError) {
	host := ""
	if endpoint, parseErr := url.Parse(c.URL); parseErr == nil {
		host = endpoint.Hostname()
	}
	attrs := []any{
		"stage", stage,
		"agent_id", agentID,
		"app_id", appID,
		"mcp_host", host,
		"transport", c.Transport,
		"auth_mode", c.AuthMode,
		"platform_credential_source", c.PlatformCredentialSource,
		"error_type", fmt.Sprintf("%T", err),
		"credential_token_set", credentials.Token != "",
		"custom_header_count", len(credentials.Headers),
	}
	if detail != nil {
		attrs = append(attrs,
			"error_code", detail.Code,
			"upstream_status", detail.HTTPStatus,
			"authentication_error", detail.Authentication,
		)
	} else {
		// SDK/upstream errors are untrusted and may echo request headers. The
		// actual Connector and platform credentials are resolved below the
		// persisted Credentials object, so logging the raw error cannot be made
		// reliably safe here.
		attrs = append(attrs, "error", "MCP connection failed")
	}
	slog.Warn("Agent App MCP connection failed", attrs...)
}

func redactMCPLogMessage(message string, c Config, credentials Credentials) string {
	secrets := []string{credentials.Token, c.URL}
	for _, value := range credentials.Headers {
		secrets = append(secrets, value)
	}
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	return message
}

func listTools(ctx context.Context, session *mcp.ClientSession) ([]*mcp.Tool, error) {
	tools := make([]*mcp.Tool, 0)
	validator := mcp.NewServer(&mcp.Implementation{Name: "app-catalog-validation", Version: "1"}, &mcp.ServerOptions{Logger: quietLogger})
	names := map[string]bool{}
	if init := session.InitializeResult(); init != nil && (init.Capabilities == nil || init.Capabilities.Tools == nil) {
		return tools, nil
	}
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < 100; page++ {
		result, err := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, fmt.Errorf("empty tools response")
		}
		for _, tool := range result.Tools {
			if tool == nil || tool.Name == "" || tool.InputSchema == nil {
				return nil, fmt.Errorf("invalid tool definition")
			}
			var schema map[string]any
			data, err := json.Marshal(tool.InputSchema)
			if err != nil || json.Unmarshal(data, &schema) != nil || schema["type"] != "object" {
				return nil, fmt.Errorf("invalid tool input schema")
			}
			if names[tool.Name] {
				return nil, fmt.Errorf("duplicate tool name")
			}
			names[tool.Name] = true
			if err := validateTool(validator, tool); err != nil {
				return nil, err
			}
			tools = append(tools, tool)
			if len(tools) > 10000 {
				return nil, fmt.Errorf("tool catalog exceeds limit")
			}
		}
		cursor = result.NextCursor
		if cursor == "" {
			return tools, nil
		}
		if seen[cursor] {
			return nil, fmt.Errorf("repeated tools cursor")
		}
		seen[cursor] = true
	}
	return nil, fmt.Errorf("tool catalog exceeds page limit")
}

func validateTool(server *mcp.Server, tool *mcp.Tool) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("invalid upstream tool definition")
		}
	}()
	server.AddTool(tool, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) { return nil, nil })
	return nil
}

func platformManualToken(config Config, credentials Credentials) string {
	if config.PlatformCredentialSource == "opencsg_login" {
		return ""
	}
	if config.AuthMode == "bearer" || config.AuthMode == "feishu" || config.AuthMode == "header" {
		return credentials.Token
	}
	return ""
}

func (t *headerTransport) rememberFailure(err *ConnectionError) {
	t.failureMu.Lock()
	t.failure = err
	t.failureMu.Unlock()
}
func (t *headerTransport) latestFailure() *ConnectionError {
	if t == nil {
		return nil
	}
	t.failureMu.Lock()
	defer t.failureMu.Unlock()
	return t.failure
}
func transportHTTPAuth(transport mcp.Transport) *headerTransport {
	if t, ok := transport.(*mcp.StreamableClientTransport); ok {
		if auth, ok := t.HTTPClient.Transport.(*headerTransport); ok {
			return auth
		}
	}
	return nil
}
func transportFailure(transport mcp.Transport, err error) *ConnectionError {
	var detail *ConnectionError
	if errors.As(err, &detail) {
		return detail
	}
	return transportHTTPAuth(transport).latestFailure()
}

func (t *headerTransport) captureFailure(err error) error {
	var detail *ConnectionError
	if errors.As(err, &detail) {
		t.rememberFailure(detail)
	}
	return err
}
