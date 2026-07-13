package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	agentruntime "csgclaw/internal/runtime"
)

type mcpServersViewTestRuntime struct {
	fakeAgentRuntime
	list func(context.Context, agentruntime.Handle, agentruntime.MCPServersSnapshot) (agentruntime.MCPServersSnapshot, error)
}

func (r mcpServersViewTestRuntime) ListMCPServers(ctx context.Context, h agentruntime.Handle, current agentruntime.MCPServersSnapshot) (agentruntime.MCPServersSnapshot, error) {
	if r.list == nil {
		return agentruntime.MCPServersSnapshot{}, nil
	}
	return r.list(ctx, h, current)
}

func TestMCPServersViewPreservesNilAndExplicitEmptyMaps(t *testing.T) {
	tests := []struct {
		name    string
		servers map[string]any
		want    string
	}{
		{name: "unmanaged", servers: nil, want: "null"},
		{name: "managed empty", servers: map[string]any{}, want: "{}"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc := &Service{
				agents: map[string]Agent{
					"u-mcp": {
						ID:          "u-mcp",
						RuntimeKind: RuntimeKindCodex,
						MCPServers:  test.servers,
					},
				},
				runtimeRegistry: map[string]agentruntime.Runtime{
					RuntimeKindCodex: mcpServersViewTestRuntime{
						fakeAgentRuntime: fakeAgentRuntime{kind: RuntimeKindCodex},
						list: func(_ context.Context, _ agentruntime.Handle, _ agentruntime.MCPServersSnapshot) (agentruntime.MCPServersSnapshot, error) {
							return agentruntime.MCPServersSnapshot{Servers: test.servers}, nil
						},
					},
				},
			}

			view, err := svc.MCPServersView(context.Background(), "u-mcp")
			if err != nil {
				t.Fatalf("MCPServersView() error = %v", err)
			}
			data, err := json.Marshal(view)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if got := string(fields["servers"]); got != test.want {
				t.Fatalf("servers = %s, want %s", got, test.want)
			}
		})
	}
}

func TestMCPServersViewReadsRuntimeServersBeforeFirstManagement(t *testing.T) {
	svc := &Service{
		agents: map[string]Agent{
			"u-mcp": {
				ID:          "u-mcp",
				RuntimeKind: RuntimeKindCodex,
			},
		},
		runtimeRegistry: map[string]agentruntime.Runtime{
			RuntimeKindCodex: mcpServersViewTestRuntime{
				fakeAgentRuntime: fakeAgentRuntime{kind: RuntimeKindCodex},
				list: func(context.Context, agentruntime.Handle, agentruntime.MCPServersSnapshot) (agentruntime.MCPServersSnapshot, error) {
					return agentruntime.MCPServersSnapshot{Servers: map[string]any{
						"manual": map[string]any{"command": "uvx"},
					}}, nil
				},
			},
		},
	}

	view, err := svc.MCPServersView(context.Background(), "u-mcp")
	if err != nil {
		t.Fatalf("MCPServersView() error = %v", err)
	}
	server, ok := view.Servers["manual"].(map[string]any)
	if !ok || server["command"] != "uvx" {
		t.Fatalf("Servers = %#v, want runtime server", view.Servers)
	}
	if got, ok := svc.Agent("u-mcp"); !ok || got.MCPServers != nil {
		t.Fatalf("Agent().MCPServers = %#v, want unmanaged state preserved on read", got.MCPServers)
	}
}

func TestMCPServersViewKeepsPersistedServersWhenRuntimeReadFails(t *testing.T) {
	readErr := errors.New("native config is unreadable")
	desired := map[string]any{
		"context7": map[string]any{
			"command": "uvx",
			"env":     map[string]any{"CONTEXT7_API_KEY": "secret"},
		},
	}
	svc := &Service{
		agents: map[string]Agent{
			"u-mcp": {
				ID:          "u-mcp",
				RuntimeKind: RuntimeKindCodex,
				MCPServers:  desired,
			},
		},
		runtimeRegistry: map[string]agentruntime.Runtime{
			RuntimeKindCodex: mcpServersViewTestRuntime{
				fakeAgentRuntime: fakeAgentRuntime{kind: RuntimeKindCodex},
				list: func(context.Context, agentruntime.Handle, agentruntime.MCPServersSnapshot) (agentruntime.MCPServersSnapshot, error) {
					return agentruntime.MCPServersSnapshot{}, readErr
				},
			},
		},
	}

	view, err := svc.MCPServersView(context.Background(), "u-mcp")
	if err != nil {
		t.Fatalf("MCPServersView() error = %v, want desired state to remain readable", err)
	}
	server, ok := view.Servers["context7"].(map[string]any)
	if !ok || server["command"] != "uvx" {
		t.Fatalf("Servers = %#v, want raw persisted server", view.Servers)
	}
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := string(fields["servers"]); got == "null" {
		t.Fatalf("servers = %s, want persisted server map", got)
	}
}
