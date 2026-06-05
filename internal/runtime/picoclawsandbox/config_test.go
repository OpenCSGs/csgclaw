package picoclawsandbox

import (
	"encoding/json"
	"testing"

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
