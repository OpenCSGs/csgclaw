package picoclawsandbox

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
)

func TestRenderConfigDisablesUnconfiguredFeishuChannel(t *testing.T) {
	data, err := RenderConfig("u-manager", "u-manager", config.ServerConfig{
		AccessToken: "shared-token",
	}, config.ModelConfig{
		ModelID: "gpt-5.5",
	}, fixedBaseURL("http://127.0.0.1:18080"))
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	var rendered struct {
		Channels map[string]map[string]any `json:"channels"`
	}
	if err := json.Unmarshal(data, &rendered); err != nil {
		t.Fatalf("RenderConfig() produced invalid JSON: %v", err)
	}
	feishu, ok := rendered.Channels["feishu"]
	if !ok {
		t.Fatalf("RenderConfig() missing channels.feishu in:\n%s", data)
	}
	if got, want := feishu["enabled"], false; got != want {
		t.Fatalf("channels.feishu.enabled = %v, want %v in:\n%s", got, want, data)
	}
	if got, want := feishu["app_id"], ""; got != want {
		t.Fatalf("channels.feishu.app_id = %q, want empty in:\n%s", got, data)
	}
	if got, want := feishu["app_secret"], ""; got != want {
		t.Fatalf("channels.feishu.app_secret = %q, want empty in:\n%s", got, data)
	}
}

func TestRenderConfigWithMCPServersWritesPicoClawToolsMCP(t *testing.T) {
	baseServer := config.ServerConfig{AccessToken: "shared-token"}
	baseModel := config.ModelConfig{ModelID: "gpt-5.5"}
	resolver := fixedBaseURL("http://127.0.0.1:18080")

	t.Run("absent disables mcp", func(t *testing.T) {
		data, err := RenderConfigWithMCPServers("manager", "u-manager", baseServer, baseModel, nil, resolver)
		if err != nil {
			t.Fatalf("RenderConfigWithMCPServers() error = %v", err)
		}
		mcpRoot := renderedToolsMCP(t, data)
		if got, want := mcpRoot["enabled"], false; got != want {
			t.Fatalf("tools.mcp.enabled = %v, want %v in:\n%s", got, want, data)
		}
		if _, ok := mcpRoot["servers"]; ok {
			t.Fatalf("tools.mcp.servers should be absent for unmanaged config: %#v", mcpRoot["servers"])
		}
	})

	t.Run("empty managed config enables empty servers", func(t *testing.T) {
		data, err := RenderConfigWithMCPServers("manager", "u-manager", baseServer, baseModel, map[string]any{}, resolver)
		if err != nil {
			t.Fatalf("RenderConfigWithMCPServers() error = %v", err)
		}
		mcpRoot := renderedToolsMCP(t, data)
		if got, want := mcpRoot["enabled"], true; got != want {
			t.Fatalf("tools.mcp.enabled = %v, want %v in:\n%s", got, want, data)
		}
		servers, ok := mcpRoot["servers"].(map[string]any)
		if !ok {
			t.Fatalf("tools.mcp.servers = %#v, want object", mcpRoot["servers"])
		}
		if len(servers) != 0 {
			t.Fatalf("tools.mcp.servers = %#v, want empty", servers)
		}
	})

	t.Run("server config is preserved", func(t *testing.T) {
		data, err := RenderConfigWithMCPServers("manager", "u-manager", baseServer, baseModel, map[string]any{
			"context7": map[string]any{
				"command":             "uvx",
				"args":                []any{"context7-mcp"},
				"startup_timeout_sec": float64(90),
			},
			"filesystem": map[string]any{
				"command": "npx",
				"args": []any{
					"-y",
					"@modelcontextprotocol/server-filesystem",
					"/home/user/workspace",
					"${workspace}",
					"${workspace}/from-placeholder",
				},
			},
		}, resolver)
		if err != nil {
			t.Fatalf("RenderConfigWithMCPServers() error = %v", err)
		}
		mcpRoot := renderedToolsMCP(t, data)
		if got, want := mcpRoot["enabled"], true; got != want {
			t.Fatalf("tools.mcp.enabled = %v, want %v in:\n%s", got, want, data)
		}
		servers := mcpRoot["servers"].(map[string]any)
		context7 := servers["context7"].(map[string]any)
		if got, want := context7["command"], "uvx"; got != want {
			t.Fatalf("context7.command = %#v, want %q", got, want)
		}
		args := context7["args"].([]any)
		if got, want := args[0], "context7-mcp"; got != want {
			t.Fatalf("context7.args[0] = %#v, want %q", got, want)
		}
		if got, want := context7["startup_timeout_sec"], float64(90); got != want {
			t.Fatalf("context7.startup_timeout_sec = %#v, want %#v", got, want)
		}
		filesystem := servers["filesystem"].(map[string]any)
		if got, want := filesystem["args"], []any{
			"-y",
			"@modelcontextprotocol/server-filesystem",
			"/home/user/workspace",
			BoxWorkspaceDir,
			BoxWorkspaceDir + "/from-placeholder",
		}; !reflect.DeepEqual(got, want) {
			t.Fatalf("filesystem.args = %#v, want %#v", got, want)
		}
	})
}

func TestRenderConfigWithMCPServersRejectsInvalidPicoClawMCP(t *testing.T) {
	_, err := RenderConfigWithMCPServers("manager", "u-manager", config.ServerConfig{}, config.ModelConfig{ModelID: "gpt-5.5"}, map[string]any{
		"broken": []any{},
	}, fixedBaseURL("http://127.0.0.1:18080"))
	if err == nil || !strings.Contains(err.Error(), "mcpServers.broken must be an object") {
		t.Fatalf("RenderConfigWithMCPServers() error = %v, want invalid server error", err)
	}
}

func renderedToolsMCP(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var rendered struct {
		Tools map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(data, &rendered); err != nil {
		t.Fatalf("RenderConfigWithMCPServers() produced invalid JSON: %v", err)
	}
	mcpRoot, ok := rendered.Tools["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("RenderConfigWithMCPServers() missing tools.mcp in:\n%s", data)
	}
	return mcpRoot
}

func TestRenderConfigEnablesFeishuChannelWhenParticipantConfigured(t *testing.T) {
	data, err := RenderConfig("manager", "u-manager", config.ServerConfig{
		AccessToken: "shared-token",
	}, config.ModelConfig{
		ModelID: "gpt-5.5",
	}, fixedBaseURL("http://127.0.0.1:18080"), staticFeishuProvider{
		participantID: "manager",
		app: feishu.AppConfig{
			AppID:     "cli_manager",
			AppSecret: "manager-secret",
		},
	})
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	var rendered struct {
		Channels map[string]map[string]any `json:"channels"`
	}
	if err := json.Unmarshal(data, &rendered); err != nil {
		t.Fatalf("RenderConfig() produced invalid JSON: %v", err)
	}
	feishuCfg := rendered.Channels["feishu"]
	if got, want := feishuCfg["enabled"], true; got != want {
		t.Fatalf("channels.feishu.enabled = %v, want %v in:\n%s", got, want, data)
	}
	if got, want := feishuCfg["app_id"], "cli_manager"; got != want {
		t.Fatalf("channels.feishu.app_id = %q, want %q in:\n%s", got, want, data)
	}
	if got, want := feishuCfg["app_secret"], "manager-secret"; got != want {
		t.Fatalf("channels.feishu.app_secret = %q, want %q in:\n%s", got, want, data)
	}
}

type staticFeishuProvider struct {
	participantID string
	app           feishu.AppConfig
}

func (p staticFeishuProvider) BotConfig(_ string) (feishu.AppConfig, bool) {
	return feishu.AppConfig{}, false
}

func (p staticFeishuProvider) BotConfigForAgent(agentID string) (string, feishu.AppConfig, bool) {
	if agentID != "u-manager" {
		return "", feishu.AppConfig{}, false
	}
	return p.participantID, p.app, true
}
