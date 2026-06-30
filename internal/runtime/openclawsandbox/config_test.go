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
