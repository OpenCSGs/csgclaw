package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultProbeStartupTimeout = 30 * time.Second
	defaultProbeToolTimeout    = 60 * time.Second
	maximumProbeTimeout        = 2 * time.Minute
	maximumProbeTools          = 1000
	maximumProbePages          = 100
)

var ErrInvalidServerConfig = errors.New("invalid mcp server config")

type ProbeServerInfo struct {
	Name    string `json:"name,omitempty"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

type ProbeTool struct {
	Description  string `json:"description,omitempty"`
	InputSchema  any    `json:"input_schema,omitempty"`
	Name         string `json:"name"`
	OutputSchema any    `json:"output_schema,omitempty"`
	Title        string `json:"title,omitempty"`
}

type ProbeResult struct {
	Connected       bool             `json:"connected"`
	DurationMS      int64            `json:"duration_ms"`
	ProtocolVersion string           `json:"protocol_version,omitempty"`
	ServerInfo      *ProbeServerInfo `json:"server_info,omitempty"`
	Tools           []ProbeTool      `json:"tools"`
	ToolsSupported  bool             `json:"tools_supported"`
	Truncated       bool             `json:"truncated,omitempty"`
}

type ServerProber interface {
	Probe(ctx context.Context, name string, config map[string]any) (ProbeResult, error)
}

func (s *Service) ProbeServer(ctx context.Context, name string, config map[string]any) (ProbeResult, error) {
	name, config, err := normalizeServerInput(name, config)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("%w: %v", ErrInvalidServerConfig, err)
	}
	return s.serverProber().Probe(ctx, name, config)
}

func (s *Service) serverProber() ServerProber {
	if s == nil || s.prober == nil {
		return defaultServerProber{}
	}
	return s.prober
}

type defaultServerProber struct{}

func (defaultServerProber) Probe(ctx context.Context, name string, config map[string]any) (ProbeResult, error) {
	startedAt := time.Now()
	transport, err := probeTransport(config)
	if err != nil {
		return ProbeResult{}, err
	}

	client := mcpsdk.NewClient(
		&mcpsdk.Implementation{Name: "csgclaw-mcp-probe", Version: "1.0.0"},
		&mcpsdk.ClientOptions{Capabilities: &mcpsdk.ClientCapabilities{}},
	)
	connectCtx, cancelConnect := context.WithCancelCause(ctx)
	connectTimerDone := make(chan struct{})
	connectTimer := time.AfterFunc(
		probeTimeout(config, "startup_timeout_sec", defaultProbeStartupTimeout),
		func() {
			cancelConnect(context.DeadlineExceeded)
			close(connectTimerDone)
		},
	)
	session, err := client.Connect(connectCtx, transport, nil)
	if !connectTimer.Stop() {
		<-connectTimerDone
	}
	if err != nil {
		if cause := context.Cause(connectCtx); cause != nil {
			err = cause
		}
		cancelConnect(err)
		return ProbeResult{}, fmt.Errorf("connect to MCP server %q: %w", name, err)
	}
	if cause := context.Cause(connectCtx); cause != nil {
		_ = session.Close()
		cancelConnect(cause)
		return ProbeResult{}, fmt.Errorf("connect to MCP server %q: %w", name, cause)
	}
	defer cancelConnect(context.Canceled)
	defer session.Close()

	result := ProbeResult{
		Connected:  true,
		DurationMS: time.Since(startedAt).Milliseconds(),
		Tools:      []ProbeTool{},
	}
	initialized := session.InitializeResult()
	if initialized != nil {
		result.ProtocolVersion = strings.TrimSpace(initialized.ProtocolVersion)
		if initialized.ServerInfo != nil {
			result.ServerInfo = &ProbeServerInfo{
				Name:    strings.TrimSpace(initialized.ServerInfo.Name),
				Title:   strings.TrimSpace(initialized.ServerInfo.Title),
				Version: strings.TrimSpace(initialized.ServerInfo.Version),
			}
		}
		result.ToolsSupported = initialized.Capabilities != nil && initialized.Capabilities.Tools != nil
	}
	if !result.ToolsSupported {
		result.DurationMS = time.Since(startedAt).Milliseconds()
		return result, nil
	}

	listCtx, cancelList := context.WithTimeout(ctx, probeTimeout(config, "tool_timeout_sec", defaultProbeToolTimeout))
	defer cancelList()
	tools, truncated, err := listProbeTools(listCtx, session)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("list tools from MCP server %q: %w", name, err)
	}
	result.Tools = tools
	result.Truncated = truncated
	result.DurationMS = time.Since(startedAt).Milliseconds()
	return result, nil
}

func listProbeTools(ctx context.Context, session *mcpsdk.ClientSession) ([]ProbeTool, bool, error) {
	tools := make([]ProbeTool, 0)
	cursor := ""
	seenCursors := map[string]struct{}{}
	for page := 0; page < maximumProbePages; page++ {
		response, err := session.ListTools(ctx, &mcpsdk.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, false, err
		}
		if response == nil {
			return nil, false, errors.New("tools/list returned an empty response")
		}
		for _, tool := range response.Tools {
			if tool == nil || strings.TrimSpace(tool.Name) == "" {
				continue
			}
			title := strings.TrimSpace(tool.Title)
			if title == "" && tool.Annotations != nil {
				title = strings.TrimSpace(tool.Annotations.Title)
			}
			tools = append(tools, ProbeTool{
				Description:  strings.TrimSpace(tool.Description),
				InputSchema:  tool.InputSchema,
				Name:         strings.TrimSpace(tool.Name),
				OutputSchema: tool.OutputSchema,
				Title:        title,
			})
			if len(tools) >= maximumProbeTools {
				sort.SliceStable(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
				return tools, true, nil
			}
		}
		nextCursor := strings.TrimSpace(response.NextCursor)
		if nextCursor == "" {
			sort.SliceStable(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
			return tools, false, nil
		}
		if _, exists := seenCursors[nextCursor]; exists {
			return nil, false, fmt.Errorf("tools/list returned a repeated cursor %q", nextCursor)
		}
		seenCursors[nextCursor] = struct{}{}
		cursor = nextCursor
	}
	sort.SliceStable(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, true, nil
}

func probeTransport(config map[string]any) (mcpsdk.Transport, error) {
	command := stringField(config, "command")
	endpoint := stringField(config, "url")
	transportName := normalizeTransportName(stringField(config, "transport"))
	if transportName == "" {
		transportName = normalizeTransportName(stringField(config, "type"))
	}
	if transportName == "" {
		if endpoint != "" && command == "" {
			transportName = "streamable_http"
		} else if command != "" && endpoint == "" {
			transportName = "stdio"
		}
	}

	switch transportName {
	case "stdio":
		if command == "" {
			return nil, fmt.Errorf("%w: stdio transport requires command", ErrInvalidServerConfig)
		}
		cmd := exec.Command(command, stringSliceField(config, "args")...)
		if cwd := stringField(config, "cwd"); cwd != "" {
			cmd.Dir = cwd
		}
		cmd.Env = append(os.Environ(), stringMapEnv(config["env"])...)
		return &mcpsdk.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil
	case "sse":
		if err := validateProbeEndpoint(endpoint); err != nil {
			return nil, err
		}
		return &mcpsdk.SSEClientTransport{Endpoint: endpoint, HTTPClient: probeHTTPClient(config, endpoint)}, nil
	case "http", "remote", "streamable", "streamable_http", "streamablehttp":
		if err := validateProbeEndpoint(endpoint); err != nil {
			return nil, err
		}
		return &mcpsdk.StreamableClientTransport{
			Endpoint:             endpoint,
			HTTPClient:           probeHTTPClient(config, endpoint),
			MaxRetries:           -1,
			DisableStandaloneSSE: true,
		}, nil
	case "":
		return nil, fmt.Errorf("%w: config must select a single command or URL transport", ErrInvalidServerConfig)
	default:
		return nil, fmt.Errorf("%w: unsupported transport %q", ErrInvalidServerConfig, transportName)
	}
}

func validateProbeEndpoint(endpoint string) error {
	if endpoint == "" {
		return fmt.Errorf("%w: HTTP transport requires url", ErrInvalidServerConfig)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%w: url must use http or https", ErrInvalidServerConfig)
	}
	return nil
}

func probeHTTPClient(config map[string]any, endpoint string) *http.Client {
	headers := http.Header{}
	if rawHeaders, ok := config["headers"].(map[string]any); ok {
		for name, rawValue := range rawHeaders {
			if value, ok := rawValue.(string); ok {
				headers.Set(name, value)
			}
		}
	}
	parsed, _ := url.Parse(endpoint)
	return &http.Client{Transport: probeHeaderTransport{
		base:          http.DefaultTransport,
		headers:       headers,
		allowedScheme: strings.ToLower(parsed.Scheme),
		allowedHost:   strings.ToLower(parsed.Host),
	}}
}

type probeHeaderTransport struct {
	base          http.RoundTripper
	headers       http.Header
	allowedScheme string
	allowedHost   string
}

func (t probeHeaderTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	cloned.Header = request.Header.Clone()
	if strings.EqualFold(request.URL.Scheme, t.allowedScheme) && strings.EqualFold(request.URL.Host, t.allowedHost) {
		for name, values := range t.headers {
			if reservedProbeHeader(name) {
				continue
			}
			cloned.Header.Del(name)
			for _, value := range values {
				cloned.Header.Add(name, value)
			}
		}
	}
	return t.base.RoundTrip(cloned)
}

func reservedProbeHeader(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	return normalized == "accept" || normalized == "content-type" || normalized == "host" ||
		strings.HasPrefix(normalized, "mcp-")
}

func probeTimeout(config map[string]any, key string, fallback time.Duration) time.Duration {
	seconds, ok := positiveInteger(config[key])
	if !ok {
		return fallback
	}
	if seconds > int64(maximumProbeTimeout/time.Second) {
		return maximumProbeTimeout
	}
	duration := time.Duration(seconds) * time.Second
	return duration
}

func positiveInteger(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), typed > 0
	case int64:
		return typed, typed > 0
	case float64:
		integer := int64(typed)
		return integer, typed > 0 && float64(integer) == typed
	default:
		return 0, false
	}
}

func normalizeTransportName(value string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_")
}

func stringField(config map[string]any, key string) string {
	value, _ := config[key].(string)
	return strings.TrimSpace(value)
}

func stringSliceField(config map[string]any, key string) []string {
	switch values := config[key].(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func stringMapEnv(value any) []string {
	items := make([]string, 0)
	switch values := value.(type) {
	case map[string]any:
		for name, rawValue := range values {
			if text, ok := rawValue.(string); ok {
				items = append(items, name+"="+text)
			}
		}
	case map[string]string:
		for name, text := range values {
			items = append(items, name+"="+text)
		}
	}
	sort.Strings(items)
	return items
}
