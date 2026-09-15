package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// CallTool connects to one normalized MCP server configuration and executes a
// tool without exposing its arguments to the model transcript.
func CallTool(ctx context.Context, serverName string, config map[string]any, toolName string, arguments map[string]any) (any, error) {
	transport, err := probeTransport(config)
	if err != nil {
		return nil, err
	}
	client := mcpsdk.NewClient(
		&mcpsdk.Implementation{Name: "csgclaw-host-tool", Version: "1.0.0"},
		&mcpsdk.ClientOptions{Capabilities: &mcpsdk.ClientCapabilities{}},
	)
	connectCtx, cancelConnect := context.WithCancelCause(ctx)
	connectTimer := time.AfterFunc(probeTimeout(config, "startup_timeout_sec", defaultProbeStartupTimeout), func() {
		cancelConnect(context.DeadlineExceeded)
	})
	session, err := client.Connect(connectCtx, transport, nil)
	connectTimer.Stop()
	if err != nil {
		cancelConnect(err)
		return nil, fmt.Errorf("connect to MCP server %q: %w", strings.TrimSpace(serverName), err)
	}
	defer cancelConnect(context.Canceled)
	defer session.Close()

	callCtx, cancelCall := context.WithTimeout(ctx, probeTimeout(config, "tool_timeout_sec", 2*time.Minute))
	defer cancelCall()
	result, err := session.CallTool(callCtx, &mcpsdk.CallToolParams{Name: strings.TrimSpace(toolName), Arguments: arguments})
	if err != nil {
		return nil, fmt.Errorf("call MCP tool %q: %w", strings.TrimSpace(toolName), err)
	}
	if result == nil {
		return nil, errors.New("MCP tool returned an empty response")
	}
	if result.IsError {
		return nil, fmt.Errorf("MCP tool %q failed: %v", strings.TrimSpace(toolName), result.Content)
	}
	if result.StructuredContent != nil {
		return result.StructuredContent, nil
	}
	return result.Content, nil
}
