package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListToolDefinitionsWithoutRedirects returns complete MCP tool definitions so
// callers acting as a protocol proxy do not discard annotations or metadata.
func ListToolDefinitionsWithoutRedirects(ctx context.Context, serverName string, config map[string]any) ([]*mcpsdk.Tool, error) {
	checkRedirect := func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	session, closeSession, err := connectToolSession(ctx, serverName, config, checkRedirect)
	if err != nil {
		return nil, err
	}
	defer closeSession()
	listCtx, cancelList := context.WithTimeout(ctx, probeTimeout(config, "tool_timeout_sec", defaultProbeToolTimeout))
	defer cancelList()
	tools, _, err := listSDKTools(listCtx, session)
	if err != nil {
		return nil, fmt.Errorf("list tools from MCP server %q: %w", strings.TrimSpace(serverName), err)
	}
	return tools, nil
}

// CallAdvertisedToolWithoutRedirects verifies and invokes a tool in one MCP
// session, preserving the complete protocol result for transparent proxying.
func CallAdvertisedToolWithoutRedirects(ctx context.Context, serverName string, config map[string]any, toolName string, arguments map[string]any) (*mcpsdk.CallToolResult, error) {
	checkRedirect := func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	session, closeSession, err := connectToolSession(ctx, serverName, config, checkRedirect)
	if err != nil {
		return nil, err
	}
	defer closeSession()

	callCtx, cancelCall := context.WithTimeout(ctx, probeTimeout(config, "tool_timeout_sec", 2*time.Minute))
	defer cancelCall()
	tools, _, err := listSDKTools(callCtx, session)
	if err != nil {
		return nil, fmt.Errorf("list tools from MCP server %q: %w", strings.TrimSpace(serverName), err)
	}
	toolName = strings.TrimSpace(toolName)
	advertised := false
	for _, tool := range tools {
		if tool != nil && strings.TrimSpace(tool.Name) == toolName {
			advertised = true
			break
		}
	}
	if !advertised {
		return nil, fmt.Errorf("tool %q is not advertised by the MCP server", toolName)
	}
	result, err := session.CallTool(callCtx, &mcpsdk.CallToolParams{Name: toolName, Arguments: arguments})
	if err != nil {
		return nil, fmt.Errorf("call MCP tool %q: %w", toolName, err)
	}
	if result == nil {
		return nil, errors.New("MCP tool returned an empty response")
	}
	return result, nil
}

func connectToolSession(ctx context.Context, serverName string, config map[string]any, checkRedirect func(*http.Request, []*http.Request) error) (*mcpsdk.ClientSession, func(), error) {
	transport, err := probeTransportWithRedirectPolicy(config, checkRedirect)
	if err != nil {
		return nil, nil, err
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "csgclaw-host-tool", Version: "1.0.0"}, &mcpsdk.ClientOptions{Capabilities: &mcpsdk.ClientCapabilities{}})
	connectCtx, cancelConnect := context.WithCancelCause(ctx)
	connectTimer := time.AfterFunc(probeTimeout(config, "startup_timeout_sec", defaultProbeStartupTimeout), func() {
		cancelConnect(context.DeadlineExceeded)
	})
	session, err := client.Connect(connectCtx, transport, nil)
	connectTimer.Stop()
	if err != nil {
		cancelConnect(err)
		return nil, nil, fmt.Errorf("connect to MCP server %q: %w", strings.TrimSpace(serverName), err)
	}
	return session, func() { _ = session.Close(); cancelConnect(context.Canceled) }, nil
}

func listSDKTools(ctx context.Context, session *mcpsdk.ClientSession) ([]*mcpsdk.Tool, bool, error) {
	tools := make([]*mcpsdk.Tool, 0)
	cursor := ""
	seen := map[string]struct{}{}
	for page := 0; page < maximumProbePages; page++ {
		response, err := session.ListTools(ctx, &mcpsdk.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, false, err
		}
		if response == nil {
			return nil, false, errors.New("tools/list returned an empty response")
		}
		for _, tool := range response.Tools {
			if tool != nil && strings.TrimSpace(tool.Name) != "" {
				tools = append(tools, tool)
				if len(tools) >= maximumProbeTools {
					return tools, true, nil
				}
			}
		}
		next := strings.TrimSpace(response.NextCursor)
		if next == "" {
			return tools, false, nil
		}
		if _, ok := seen[next]; ok {
			return nil, false, fmt.Errorf("tools/list returned a repeated cursor %q", next)
		}
		seen[next] = struct{}{}
		cursor = next
	}
	return tools, true, nil
}
