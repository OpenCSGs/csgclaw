package openclawsandbox

import (
	"encoding/json"
	"strings"
	"testing"

	"csgclaw/internal/config"
)

func TestRenderAgentOpenClawConfigUsesOpenAICompatForMinimaxBaseURL(t *testing.T) {
	data, err := renderConfig("u-manager", config.ServerConfig{
		ListenAddr:       "127.0.0.1:18080",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
		AccessToken:      "gateway-shared-token",
	}, config.ModelConfig{
		BaseURL: "https://api.minimaxi.com/v1",
		APIKey:  "sk-minimax-test",
		ModelID: "MiniMax-M2.7",
	}, testBaseURLResolver)
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
	if got, want := llm["baseUrl"], "https://api.minimaxi.com/v1"; got != want {
		t.Fatalf("csgclaw-llm baseUrl = %v, want %v", got, want)
	}
	if got, want := llm["api"], "openai-completions"; got != want {
		t.Fatalf("csgclaw-llm api = %v, want %v", got, want)
	}
	if got, want := llm["apiKey"], "sk-minimax-test"; got != want {
		t.Fatalf("csgclaw-llm apiKey = %v, want %v", got, want)
	}
	if got, want := llm["authHeader"], true; got != want {
		t.Fatalf("csgclaw-llm authHeader = %v, want %v", got, want)
	}
	agents := cfg["agents"].(map[string]any)
	defaults := agents["defaults"].(map[string]any)
	model := defaults["model"].(map[string]any)
	if got, want := model["primary"], "csgclaw-llm/MiniMax-M2.7"; got != want {
		t.Fatalf("primary model = %v, want %v", got, want)
	}
	if got, want := defaults["verboseDefault"], "on"; got != want {
		t.Fatalf("verboseDefault = %v, want %v", got, want)
	}
}

func TestRenderAgentOpenClawConfigUsesOpenAICompatForInfiniMaaS(t *testing.T) {
	data, err := renderConfig("u-manager", config.ServerConfig{
		ListenAddr:       "127.0.0.1:18080",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
		AccessToken:      "gateway-shared-token",
	}, config.ModelConfig{
		BaseURL: "https://cloud.infini-ai.com/maas/v1",
		APIKey:  "sk-infini-test",
		ModelID: "minimax-m2.5",
	}, testBaseURLResolver)
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
	if got, want := llm["baseUrl"], "https://cloud.infini-ai.com/maas/v1"; got != want {
		t.Fatalf("csgclaw-llm baseUrl = %v, want %v", got, want)
	}
	if got, want := llm["apiKey"], "sk-infini-test"; got != want {
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
}

func TestRenderAgentOpenClawConfigUsesBridgeWhenBaseURLEmpty(t *testing.T) {
	data, err := renderConfig("u-manager", config.ServerConfig{
		ListenAddr:       "127.0.0.1:18080",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
		AccessToken:      "shared-token",
	}, config.ModelConfig{
		ModelID: "MiniMax-M2.7",
	}, testBaseURLResolver)
	if err != nil {
		t.Fatalf("renderAgentOpenClawConfig() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `http://127.0.0.1:18080/api/bots/u-manager/llm`) {
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

func TestRenderAgentOpenClawConfigDefaultsCsgclawGroupsToMentionOnly(t *testing.T) {
	data, err := renderConfig("u-manager", config.ServerConfig{
		ListenAddr:       "127.0.0.1:18080",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
		AccessToken:      "shared-token",
	}, config.ModelConfig{
		ModelID: "MiniMax-M2.7",
	}, testBaseURLResolver)
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
}

func TestRenderAgentOpenClawConfigPassesThroughDockerHostAlias(t *testing.T) {
	data, err := renderConfig("u-manager", config.ServerConfig{
		ListenAddr:       "0.0.0.0:18080",
		AdvertiseBaseURL: "http://host.docker.internal:18080",
		AccessToken:      "shared-token",
	}, config.ModelConfig{
		BaseURL: "https://api.minimaxi.com/v1",
		APIKey:  "sk-minimax-test",
		ModelID: "MiniMax-M2.7",
	}, testBaseURLResolver)
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
