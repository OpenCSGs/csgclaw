package picoclawsandbox

import (
	"encoding/json"
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
