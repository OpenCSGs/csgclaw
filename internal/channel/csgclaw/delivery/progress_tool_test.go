package delivery

import (
	"encoding/json"
	"testing"

	"csgclaw/internal/agentengine"
)

func TestProgressToolProjectsSummaryAndLegacyMetadata(t *testing.T) {
	started := progressTool(agentengine.ToolActivity{
		Kind: "exec_command",
		Payload: map[string]any{
			"command": "cat src/README.md",
			"workdir": "/workspace/project",
			"commandActions": []any{
				map[string]any{"type": "read", "name": "README.md", "path": "src/README.md"},
			},
		},
	}, nil)
	completed := progressTool(agentengine.ToolActivity{
		Kind:   "exec_command",
		Status: "completed",
		Payload: map[string]any{
			"exit_code": json.Number("0"),
			"duration":  map[string]any{"secs": json.Number("0"), "nanos": json.Number("120000000")},
		},
	}, started)
	if completed.Command != "cat src/README.md" || completed.Summary != "README.md" ||
		completed.Cwd != "/workspace/project" || len(completed.Actions) != 1 || completed.Actions[0] != "read" ||
		completed.ExitCode == nil || *completed.ExitCode != 0 || completed.DurationMS == nil || *completed.DurationMS != 120 {
		t.Fatalf("unexpected command details: %+v", completed)
	}

	mcp := progressTool(agentengine.ToolActivity{
		Kind:    "mcp_tool_call",
		Payload: map[string]any{"arguments": map[string]any{"q": "上海天气"}},
	}, nil)
	if mcp.Summary != "上海天气" || mcp.Cwd != "" || mcp.ExitCode != nil {
		t.Fatalf("unexpected MCP details: %+v", mcp)
	}
}

func TestProgressToolPreservesWindowsWorkingDirectory(t *testing.T) {
	for _, path := range []string{`C:\Users\Alice\My Project`, `\\server\share\Project`} {
		started := progressTool(agentengine.ToolActivity{
			Kind:    "exec_command",
			Payload: map[string]any{"workdir": path},
		}, nil)
		completed := progressTool(agentengine.ToolActivity{Kind: "exec_command", Status: "completed"}, started)
		if completed.Cwd != path {
			t.Errorf("working directory = %q, want %q", completed.Cwd, path)
		}
	}
}
