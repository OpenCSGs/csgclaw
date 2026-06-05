package runtimewiring

import (
	"testing"

	"csgclaw/internal/channel/feishu"
)

func TestPicoClawRuntimeEnvVarsUseParticipantIDForCSGClawChannel(t *testing.T) {
	env := picoClawRuntimeEnvVars(
		"http://10.0.0.8:18080",
		"shared-token",
		"manager",
		"u-manager",
		"http://10.0.0.8:18080/api/v1/agents/u-manager/llm",
		"minimax-m2.7",
		nil,
	)

	if got, want := env["PICOCLAW_CHANNELS_CSGCLAW_PARTICIPANT_ID"], "manager"; got != want {
		t.Fatalf("PICOCLAW_CHANNELS_CSGCLAW_PARTICIPANT_ID = %q, want %q", got, want)
	}
	if _, ok := env["PICOCLAW_CHANNELS_CSGCLAW_BOT_ID"]; ok {
		t.Fatalf("PICOCLAW_CHANNELS_CSGCLAW_BOT_ID should not be emitted")
	}
	if got, want := env["PICOCLAW_CHANNELS_CSGCLAW_ENABLED"], "true"; got != want {
		t.Fatalf("PICOCLAW_CHANNELS_CSGCLAW_ENABLED = %q, want %q", got, want)
	}
}

func TestPicoClawRuntimeEnvVarsEnableFeishuOnlyForConfiguredBot(t *testing.T) {
	env := picoClawRuntimeEnvVars(
		"http://10.0.0.8:18080",
		"shared-token",
		"u-missing",
		"u-missing",
		"http://10.0.0.8:18080/api/v1/agents/u-missing/llm",
		"minimax-m2.7",
		staticFeishuProvider{apps: map[string]feishu.AppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
		}},
	)
	for _, key := range []string{
		"PICOCLAW_CHANNELS_FEISHU_ENABLED",
		"PICOCLAW_CHANNELS_FEISHU_APP_ID",
		"PICOCLAW_CHANNELS_FEISHU_APP_SECRET",
	} {
		if _, ok := env[key]; ok {
			t.Fatalf("%s should not be emitted for an unconfigured Feishu bot", key)
		}
	}

	env = picoClawRuntimeEnvVars(
		"http://10.0.0.8:18080",
		"shared-token",
		"manager",
		"u-manager",
		"http://10.0.0.8:18080/api/v1/agents/u-manager/llm",
		"minimax-m2.7",
		staticFeishuProvider{apps: map[string]feishu.AppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
		}},
	)
	if got, want := env["PICOCLAW_CHANNELS_FEISHU_ENABLED"], "true"; got != want {
		t.Fatalf("PICOCLAW_CHANNELS_FEISHU_ENABLED = %q, want %q", got, want)
	}
	if got, want := env["PICOCLAW_CHANNELS_CSGCLAW_PARTICIPANT_ID"], "manager"; got != want {
		t.Fatalf("PICOCLAW_CHANNELS_CSGCLAW_PARTICIPANT_ID = %q, want %q", got, want)
	}
	if _, ok := env["PICOCLAW_CHANNELS_CSGCLAW_BOT_ID"]; ok {
		t.Fatalf("PICOCLAW_CHANNELS_CSGCLAW_BOT_ID should not be emitted")
	}
	if got, want := env["PICOCLAW_CHANNELS_FEISHU_APP_ID"], "cli_manager"; got != want {
		t.Fatalf("PICOCLAW_CHANNELS_FEISHU_APP_ID = %q, want %q", got, want)
	}
	if got, want := env["PICOCLAW_CHANNELS_FEISHU_APP_SECRET"], "manager-secret"; got != want {
		t.Fatalf("PICOCLAW_CHANNELS_FEISHU_APP_SECRET = %q, want %q", got, want)
	}
}

type staticFeishuProvider struct {
	apps map[string]feishu.AppConfig
}

func (p staticFeishuProvider) BotConfig(botID string) (feishu.AppConfig, bool) {
	app, ok := p.apps[botID]
	return app, ok
}
