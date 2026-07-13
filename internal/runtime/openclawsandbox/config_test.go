package openclawsandbox

import (
	"encoding/json"
	"reflect"
	"runtime"
	"strings"
	"testing"

	feishuchannel "csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
)

func TestRenderAgentOpenClawConfigUsesBridgeForMinimaxBaseURL(t *testing.T) {
	data, err := renderConfig("u-manager", "u-manager", config.ServerConfig{
		ListenAddr:       "127.0.0.1:18080",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
		AccessToken:      "gateway-shared-token",
	}, config.ModelConfig{
		BaseURL: "https://api.minimaxi.com/v1",
		APIKey:  "sk-minimax-test",
		ModelID: "MiniMax-M2.7",
	}, testBaseURLResolver, nil)
	if err != nil {
		t.Fatalf("renderAgentOpenClawConfig() error = %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	models := cfg["models"].(map[string]any)
	providers := models["providers"].(map[string]any)
	if _, ok := providers["csgclaw-minimax"]; ok {
		t.Fatalf("csgclaw-minimax provider should not be used for OpenAI-compatible MiniMax config")
	}
	llm := providers["csgclaw-llm"].(map[string]any)
	if got, want := llm["baseUrl"], "http://127.0.0.1:18080/api/v1/agents/u-manager/llm"; got != want {
		t.Fatalf("csgclaw-llm baseUrl = %v, want %v", got, want)
	}
	if got, want := llm["api"], "openai-completions"; got != want {
		t.Fatalf("csgclaw-llm api = %v, want %v", got, want)
	}
	if got, want := llm["apiKey"], "gateway-shared-token"; got != want {
		t.Fatalf("csgclaw-llm apiKey = %v, want %v", got, want)
	}
	if got, want := llm["authHeader"], true; got != want {
		t.Fatalf("csgclaw-llm authHeader = %v, want %v", got, want)
	}
	modelList := llm["models"].([]any)
	entry := modelList[0].(map[string]any)
	if _, ok := entry["api"]; ok {
		t.Fatalf("model api should not be set for OpenAI-compatible bridge model: %#v", entry["api"])
	}
	if got, want := entry["reasoning"], false; got != want {
		t.Fatalf("model reasoning = %v, want %v", got, want)
	}
	if got, want := entry["input"], []any{"text"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("model input = %#v, want %#v", got, want)
	}
	compat := entry["compat"].(map[string]any)
	if got, want := compat["supportsReasoningEffort"], false; got != want {
		t.Fatalf("model compat.supportsReasoningEffort = %v, want %v", got, want)
	}
	if got, want := compat["supportedReasoningEfforts"], []any{}; !reflect.DeepEqual(got, want) {
		t.Fatalf("model compat.supportedReasoningEfforts = %#v, want %#v", got, want)
	}
	wantReasoningMap := map[string]any{}
	if got := compat["reasoningEffortMap"]; !reflect.DeepEqual(got, wantReasoningMap) {
		t.Fatalf("model compat.reasoningEffortMap = %#v, want %#v", got, wantReasoningMap)
	}
	if got, want := compat["supportsUsageInStreaming"], false; got != want {
		t.Fatalf("model compat.supportsUsageInStreaming = %v, want %v", got, want)
	}
	agents := cfg["agents"].(map[string]any)
	defaults := agents["defaults"].(map[string]any)
	model := defaults["model"].(map[string]any)
	if got, want := model["primary"], "csgclaw-llm/MiniMax-M2.7"; got != want {
		t.Fatalf("primary model = %v, want %v", got, want)
	}
	if _, ok := defaults["thinkingDefault"]; ok {
		t.Fatalf("thinkingDefault should be omitted for non-reasoning OpenAI-compatible bridge model: %#v", defaults["thinkingDefault"])
	}
	if got, want := defaults["verboseDefault"], "on"; got != want {
		t.Fatalf("verboseDefault = %v, want %v", got, want)
	}
	if runtime.GOOS == "windows" {
		if got, want := defaults["workspace"], BoxWindowsWorkspaceDir; got != want {
			t.Fatalf("workspace = %v, want %v", got, want)
		}
	}
	tools := cfg["tools"].(map[string]any)
	if got, want := tools["deny"], []any{"image"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tools.deny = %#v, want %#v", got, want)
	}
	text := string(data)
	if strings.Contains(text, "https://api.minimaxi.com/v1") || strings.Contains(text, "sk-minimax-test") {
		t.Fatalf("rendered OpenClaw config should use CSGClaw bridge, not upstream credentials:\n%s", text)
	}
}

func TestRenderAgentOpenClawConfigUsesBridgeForInfiniMaaS(t *testing.T) {
	data, err := renderConfig("u-manager", "u-manager", config.ServerConfig{
		ListenAddr:       "127.0.0.1:18080",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
		AccessToken:      "gateway-shared-token",
	}, config.ModelConfig{
		BaseURL: "https://cloud.infini-ai.com/maas/v1",
		APIKey:  "sk-infini-test",
		ModelID: "minimax-m2.5",
	}, testBaseURLResolver, nil)
	if err != nil {
		t.Fatalf("renderAgentOpenClawConfig() error = %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	models := cfg["models"].(map[string]any)
	providers := models["providers"].(map[string]any)
	if _, ok := providers["csgclaw-minimax"]; ok {
		t.Fatalf("OpenClaw MiniMax provider should not be used for Infini MaaS (OpenAI-compatible; model may contain 'minimax')")
	}
	llm := providers["csgclaw-llm"].(map[string]any)
	if got, want := llm["baseUrl"], "http://127.0.0.1:18080/api/v1/agents/u-manager/llm"; got != want {
		t.Fatalf("csgclaw-llm baseUrl = %v, want %v", got, want)
	}
	if got, want := llm["apiKey"], "gateway-shared-token"; got != want {
		t.Fatalf("csgclaw-llm apiKey = %v, want %v", got, want)
	}
	if got, want := llm["api"], "openai-completions"; got != want {
		t.Fatalf("csgclaw-llm api = %v, want %v", got, want)
	}
	if got, want := llm["authHeader"], true; got != want {
		t.Fatalf("csgclaw-llm authHeader = %v, want %v", got, want)
	}
	if got, want := llm["auth"], "token"; got != want {
		t.Fatalf("csgclaw-llm auth = %v, want %v", got, want)
	}
	agents := cfg["agents"].(map[string]any)
	defaults := agents["defaults"].(map[string]any)
	model := defaults["model"].(map[string]any)
	if got, want := model["primary"], "csgclaw-llm/minimax-m2.5"; got != want {
		t.Fatalf("primary model = %v, want %v", got, want)
	}
	if got, want := defaults["verboseDefault"], "on"; got != want {
		t.Fatalf("verboseDefault = %v, want %v", got, want)
	}
	text := string(data)
	if strings.Contains(text, "https://cloud.infini-ai.com/maas/v1") || strings.Contains(text, "sk-infini-test") {
		t.Fatalf("rendered OpenClaw config should use CSGClaw bridge, not upstream credentials:\n%s", text)
	}
}

func TestRenderAgentOpenClawConfigUsesBridgeWhenBaseURLEmpty(t *testing.T) {
	data, err := renderConfig("u-manager", "u-manager", config.ServerConfig{
		ListenAddr:       "127.0.0.1:18080",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
		AccessToken:      "shared-token",
	}, config.ModelConfig{
		ModelID: "MiniMax-M2.7",
	}, testBaseURLResolver, nil)
	if err != nil {
		t.Fatalf("renderAgentOpenClawConfig() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `http://127.0.0.1:18080/api/v1/agents/u-manager/llm`) {
		t.Fatalf("expected CSGClaw LLM bridge URL in config:\n%s", text)
	}
	for _, placeholder := range []string{
		"example.invalid",
		"REPLACE_WITH_MODEL_ID",
		"REPLACE_WITH_BOT_ID",
		"REPLACE_WITH_LLM_API_KEY",
		"REPLACE_WITH_CSGCLAW_ACCESS_TOKEN",
	} {
		if strings.Contains(text, placeholder) {
			t.Fatalf("rendered OpenClaw config leaked template placeholder %q:\n%s", placeholder, text)
		}
	}
}

func TestRenderAgentOpenClawConfigRendersMCPServers(t *testing.T) {
	data, err := renderConfigWithMCPServers("u-worker-1", "u-worker-1", config.ServerConfig{
		ListenAddr:       "127.0.0.1:18080",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
		AccessToken:      "shared-token",
	}, config.ModelConfig{
		ModelID: "MiniMax-M2.7",
	}, map[string]any{
		"context7": map[string]any{
			"command":             "uvx",
			"args":                []any{"context7-mcp"},
			"startup_timeout_sec": float64(90),
			"tool_timeout_sec":    120,
			"env": map[string]any{
				"CONTEXT7_API_KEY": "secret",
			},
		},
		"filesystem": map[string]any{
			"command": "npx",
			"args": []any{
				"-y",
				"@modelcontextprotocol/server-filesystem",
				"/home/user/workspace",
				"/home/user/workspace/nested",
				"${workspace}",
				"${workspace}/from-placeholder",
				"--root=/home/user/workspace",
			},
		},
		"remote-search": map[string]any{
			"command":   nil,
			"url":       "https://mcp.example.com/mcp",
			"transport": "streamable-http",
			"headers": map[string]any{
				"Authorization": "Bearer secret",
			},
		},
	}, testBaseURLResolver, nil)
	if err != nil {
		t.Fatalf("renderConfigWithMCPServers() error = %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	mcp := cfg["mcp"].(map[string]any)
	servers := mcp["servers"].(map[string]any)
	context7 := servers["context7"].(map[string]any)
	if got, want := context7["command"], "uvx"; got != want {
		t.Fatalf("context7 command = %#v, want %q", got, want)
	}
	if got, want := context7["args"], []any{"context7-mcp"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("context7 args = %#v, want %#v", got, want)
	}
	if got, want := context7["startup_timeout_sec"], float64(90); got != want {
		t.Fatalf("context7 startup_timeout_sec = %#v, want %#v", got, want)
	}
	if got, want := context7["tool_timeout_sec"], float64(120); got != want {
		t.Fatalf("context7 tool_timeout_sec = %#v, want %#v", got, want)
	}
	env := context7["env"].(map[string]any)
	if got, want := env["CONTEXT7_API_KEY"], "secret"; got != want {
		t.Fatalf("context7 env key = %#v, want %q", got, want)
	}
	filesystem := servers["filesystem"].(map[string]any)
	workspace := workspaceGuestPathForGOOS(runtime.GOOS)
	if got, want := filesystem["args"], []any{
		"-y",
		"@modelcontextprotocol/server-filesystem",
		"/home/user/workspace",
		"/home/user/workspace/nested",
		workspace,
		strings.TrimRight(workspace, "/") + "/from-placeholder",
		"--root=/home/user/workspace",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("filesystem args = %#v, want %#v", got, want)
	}
	remote := servers["remote-search"].(map[string]any)
	if got, want := remote["url"], "https://mcp.example.com/mcp"; got != want {
		t.Fatalf("remote-search url = %#v, want %q", got, want)
	}
	if _, ok := remote["command"]; ok {
		t.Fatalf("remote-search command is present, want nil optional command omitted: %#v", remote)
	}
	if got, want := remote["transport"], "streamable-http"; got != want {
		t.Fatalf("remote-search transport = %#v, want %q", got, want)
	}
	headers := remote["headers"].(map[string]any)
	if got, want := headers["Authorization"], "Bearer secret"; got != want {
		t.Fatalf("remote-search Authorization = %#v, want %q", got, want)
	}
}

func TestRenderAgentOpenClawConfigMCPEmptyAndClearSemantics(t *testing.T) {
	baseServer := config.ServerConfig{
		ListenAddr:       "127.0.0.1:18080",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
		AccessToken:      "shared-token",
	}
	baseModel := config.ModelConfig{ModelID: "MiniMax-M2.7"}
	for _, tc := range []struct {
		name        string
		mcpServers  map[string]any
		wantMCP     bool
		wantServers bool
	}{
		{
			name: "absent does not write mcp",
		},
		{
			name:        "empty object writes empty servers",
			mcpServers:  map[string]any{},
			wantMCP:     true,
			wantServers: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := renderConfigWithMCPServers("u-worker-1", "u-worker-1", baseServer, baseModel, tc.mcpServers, testBaseURLResolver, nil)
			if err != nil {
				t.Fatalf("renderConfigWithMCPServers() error = %v", err)
			}
			var cfg map[string]any
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			mcp, hasMCP := cfg["mcp"].(map[string]any)
			if hasMCP != tc.wantMCP {
				t.Fatalf("mcp present = %v, want %v; config:\n%s", hasMCP, tc.wantMCP, data)
			}
			if !hasMCP {
				return
			}
			servers, hasServers := mcp["servers"].(map[string]any)
			if hasServers != tc.wantServers {
				t.Fatalf("mcp.servers present = %v, want %v; mcp=%#v", hasServers, tc.wantServers, mcp)
			}
			if hasServers && len(servers) != 0 {
				t.Fatalf("mcp.servers = %#v, want empty", servers)
			}
		})
	}
}

func TestRenderAgentOpenClawConfigRejectsInvalidMCPServer(t *testing.T) {
	baseServer := config.ServerConfig{
		ListenAddr:       "127.0.0.1:18080",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
		AccessToken:      "shared-token",
	}
	baseModel := config.ModelConfig{ModelID: "MiniMax-M2.7"}
	for _, tc := range []struct {
		name string
		mcp  map[string]any
		want string
	}{
		{
			name: "server entry must be object",
			mcp:  map[string]any{"broken": "invalid"},
			want: "mcpServers.broken must be an object",
		},
		{
			name: "server requires command or url",
			mcp:  map[string]any{"broken": map[string]any{"args": []any{"missing-command-or-url"}}},
			want: "must declare command or url",
		},
		{
			name: "blank command is invalid even with url",
			mcp:  map[string]any{"broken": map[string]any{"command": " ", "url": "https://mcp.example.com/mcp"}},
			want: "command must not be blank",
		},
		{
			name: "blank url is invalid even with command",
			mcp:  map[string]any{"broken": map[string]any{"command": "uvx", "url": " "}},
			want: "url must not be blank",
		},
		{
			name: "args must contain strings",
			mcp:  map[string]any{"broken": map[string]any{"command": "uvx", "args": []any{1}}},
			want: "args must be an array of strings",
		},
		{
			name: "env values must be strings",
			mcp:  map[string]any{"broken": map[string]any{"command": "uvx", "env": map[string]any{"TOKEN": 1}}},
			want: "env must be an object with string values",
		},
		{
			name: "trimmed server names must be unique",
			mcp: map[string]any{
				"same":  map[string]any{"command": "uvx"},
				" same": map[string]any{"command": "uvx"},
			},
			want: `duplicate server name "same"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := renderConfigWithMCPServers("u-worker-1", "u-worker-1", baseServer, baseModel, tc.mcp, testBaseURLResolver, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("renderConfigWithMCPServers() error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestRenderAgentOpenClawConfigUsesCodexResponsesModelMetadata(t *testing.T) {
	data, err := renderConfig("u-manager", "u-manager", config.ServerConfig{
		ListenAddr:       "127.0.0.1:18080",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
		AccessToken:      "shared-token",
	}, config.ModelConfig{
		Provider:        "codex",
		ModelID:         "gpt-5.5",
		ReasoningEffort: "high",
	}, testBaseURLResolver, nil)
	if err != nil {
		t.Fatalf("renderAgentOpenClawConfig() error = %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	models := cfg["models"].(map[string]any)
	providers := models["providers"].(map[string]any)
	llm := providers["csgclaw-llm"].(map[string]any)
	if got, want := llm["api"], "openai-codex-responses"; got != want {
		t.Fatalf("csgclaw-llm api = %v, want %v", got, want)
	}
	modelList := llm["models"].([]any)
	entry := modelList[0].(map[string]any)
	if got, want := entry["api"], "openai-codex-responses"; got != want {
		t.Fatalf("model api = %v, want %v", got, want)
	}
	if got, want := entry["reasoning"], true; got != want {
		t.Fatalf("model reasoning = %v, want %v", got, want)
	}
	if got, want := entry["input"], []any{"text", "image"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("model input = %#v, want %#v", got, want)
	}
	compat := entry["compat"].(map[string]any)
	if got, want := compat["supportsReasoningEffort"], true; got != want {
		t.Fatalf("model compat.supportsReasoningEffort = %v, want %v", got, want)
	}
	if got, want := compat["supportedReasoningEfforts"], []any{"low", "medium", "high", "xhigh"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("model compat.supportedReasoningEfforts = %#v, want %#v", got, want)
	}
	wantReasoningMap := map[string]any{
		"minimal": "low",
		"low":     "low",
		"medium":  "medium",
		"high":    "high",
		"xhigh":   "xhigh",
	}
	if got := compat["reasoningEffortMap"]; !reflect.DeepEqual(got, wantReasoningMap) {
		t.Fatalf("model compat.reasoningEffortMap = %#v, want %#v", got, wantReasoningMap)
	}
	if got, want := compat["supportsUsageInStreaming"], true; got != want {
		t.Fatalf("model compat.supportsUsageInStreaming = %v, want %v", got, want)
	}
	agents := cfg["agents"].(map[string]any)
	defaults := agents["defaults"].(map[string]any)
	if got, want := defaults["thinkingDefault"], "high"; got != want {
		t.Fatalf("thinkingDefault = %v, want %v", got, want)
	}
}

func TestRenderAgentOpenClawConfigSplitsParticipantAndAgentID(t *testing.T) {
	data, err := renderConfig("manager", "u-manager", config.ServerConfig{
		ListenAddr:       "127.0.0.1:18080",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
		AccessToken:      "shared-token",
	}, config.ModelConfig{
		ModelID: "MiniMax-M2.7",
	}, testBaseURLResolver, nil)
	if err != nil {
		t.Fatalf("renderAgentOpenClawConfig() error = %v", err)
	}
	text := string(data)
	for _, want := range []string{
		`"botId": "manager"`,
		`"baseUrl": "http://127.0.0.1:18080/api/v1/agents/u-manager/llm"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered OpenClaw config missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, `/api/v1/agents/manager/llm`) {
		t.Fatalf("rendered OpenClaw config used participant ID for LLM bridge:\n%s", text)
	}
}

func TestRenderAgentOpenClawConfigDisablesStartupUpdateCheck(t *testing.T) {
	data, err := renderConfig("u-manager", "u-manager", config.ServerConfig{
		ListenAddr:       "127.0.0.1:18080",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
		AccessToken:      "shared-token",
	}, config.ModelConfig{
		ModelID: "MiniMax-M2.7",
	}, testBaseURLResolver, nil)
	if err != nil {
		t.Fatalf("renderAgentOpenClawConfig() error = %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	update := cfg["update"].(map[string]any)
	if got, want := update["checkOnStart"], false; got != want {
		t.Fatalf("update.checkOnStart = %v, want %v", got, want)
	}
}

func TestRenderAgentOpenClawConfigDefaultsCsgclawGroupsToMentionOnly(t *testing.T) {
	data, err := renderConfig("u-manager", "u-manager", config.ServerConfig{
		ListenAddr:       "127.0.0.1:18080",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
		AccessToken:      "shared-token",
	}, config.ModelConfig{
		ModelID: "MiniMax-M2.7",
	}, testBaseURLResolver, nil)
	if err != nil {
		t.Fatalf("renderAgentOpenClawConfig() error = %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	channels := cfg["channels"].(map[string]any)
	csgclaw := channels["csgclaw"].(map[string]any)
	groupTrigger := csgclaw["groupTrigger"].(map[string]any)
	if got, want := groupTrigger["mentionOnly"], true; got != want {
		t.Fatalf("groupTrigger.mentionOnly = %v, want %v", got, want)
	}
	groups := csgclaw["groups"].(map[string]any)
	defaultGroup := groups["*"].(map[string]any)
	if got, want := defaultGroup["requireMention"], true; got != want {
		t.Fatalf("groups.*.requireMention = %v, want %v", got, want)
	}
	if _, ok := channels["feishu"]; ok {
		t.Fatalf("feishu channel should not be rendered without bot credentials")
	}
}

func TestRenderAgentOpenClawConfigAddsFeishuChannelWhenConfigured(t *testing.T) {
	data, err := renderConfig("u-manager", "u-manager", config.ServerConfig{
		ListenAddr:       "127.0.0.1:18080",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
		AccessToken:      "shared-token",
	}, config.ModelConfig{
		ModelID: "MiniMax-M2.7",
	}, testBaseURLResolver, staticFeishuProvider{
		bots: map[string]feishuchannel.AppConfig{
			"manager": {
				AppID:     "cli_a_test",
				AppSecret: "secret-test",
			},
		},
	})
	if err != nil {
		t.Fatalf("renderAgentOpenClawConfig() error = %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	channels := cfg["channels"].(map[string]any)
	feishuCfg := channels["feishu"].(map[string]any)
	if got, want := feishuCfg["enabled"], true; got != want {
		t.Fatalf("feishu.enabled = %v, want %v", got, want)
	}
	if got, want := feishuCfg["connectionMode"], "websocket"; got != want {
		t.Fatalf("feishu.connectionMode = %v, want %v", got, want)
	}
	if got, want := feishuCfg["defaultAccount"], "manager"; got != want {
		t.Fatalf("feishu.defaultAccount = %v, want %v", got, want)
	}
	if got, want := feishuCfg["requireMention"], true; got != want {
		t.Fatalf("feishu.requireMention = %v, want %v", got, want)
	}
	accounts := feishuCfg["accounts"].(map[string]any)
	account := accounts["manager"].(map[string]any)
	if got, want := account["appId"], "cli_a_test"; got != want {
		t.Fatalf("feishu account appId = %v, want %v", got, want)
	}
	if got, want := account["appSecret"], "secret-test"; got != want {
		t.Fatalf("feishu account appSecret = %v, want %v", got, want)
	}
	plugins := cfg["plugins"].(map[string]any)
	entries := plugins["entries"].(map[string]any)
	feishuPlugin := entries["feishu"].(map[string]any)
	if got, want := feishuPlugin["enabled"], true; got != want {
		t.Fatalf("plugins.entries.feishu.enabled = %v, want %v", got, want)
	}
}

func TestRenderAgentOpenClawConfigPassesThroughDockerHostAlias(t *testing.T) {
	data, err := renderConfig("u-manager", "u-manager", config.ServerConfig{
		ListenAddr:       "0.0.0.0:18080",
		AdvertiseBaseURL: "http://host.docker.internal:18080",
		AccessToken:      "shared-token",
	}, config.ModelConfig{
		BaseURL: "https://api.minimaxi.com/v1",
		APIKey:  "sk-minimax-test",
		ModelID: "MiniMax-M2.7",
	}, testBaseURLResolver, nil)
	if err != nil {
		t.Fatalf("renderAgentOpenClawConfig() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"baseUrl": "http://host.docker.internal:18080"`) {
		t.Fatalf("expected CSGClaw channel base URL from advertise_base_url in config:\n%s", text)
	}
	if !strings.Contains(text, `"primary": "csgclaw-llm/MiniMax-M2.7"`) {
		t.Fatalf("expected OpenAI-compatible primary model:\n%s", text)
	}
}

func testBaseURLResolver(server config.ServerConfig) string {
	return strings.TrimRight(server.AdvertiseBaseURL, "/")
}

type staticFeishuProvider struct {
	bots map[string]feishuchannel.AppConfig
}

func (p staticFeishuProvider) BotConfig(botID string) (feishuchannel.AppConfig, bool) {
	app, ok := p.bots[botID]
	return app, ok
}

func (p staticFeishuProvider) BotConfigForAgent(agentID string) (string, feishuchannel.AppConfig, bool) {
	participantID := strings.TrimPrefix(strings.TrimSpace(agentID), "u-")
	app, ok := p.bots[participantID]
	return participantID, app, ok
}
