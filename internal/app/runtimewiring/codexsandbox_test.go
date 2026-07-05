package runtimewiring

import (
	"strings"
	"testing"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/runtime/codexsandbox"
)

func TestCodexSandboxRuntimeEnvVarsExposeCSGClawAndFeishuTenantContract(t *testing.T) {
	env := codexSandboxRuntimeEnvVars(
		"http://10.0.0.8:18080",
		"shared-token",
		"pt-dev-local",
		"u-dev",
		"http://10.0.0.8:18080/api/v1/agents/u-dev/llm",
		"gpt-5.5",
		staticFeishuProvider{apps: map[string]feishu.AppConfig{
			"dev": {AppID: "cli_dev", AppSecret: "dev-secret"},
		}},
	)

	want := map[string]string{
		"CSGCLAW_BASE_URL":                "http://10.0.0.8:18080",
		"CSGCLAW_ACCESS_TOKEN":            "shared-token",
		"CSGCLAW_PARTICIPANT_ID":          "pt-dev-local",
		"CSGCLAW_AGENT_ID":                "u-dev",
		"CSGCLAW_LLM_BASE_URL":            "http://10.0.0.8:18080/api/v1/agents/u-dev/llm",
		"CSGCLAW_LLM_API_KEY":             "shared-token",
		"CSGCLAW_LLM_MODEL_ID":            "gpt-5.5",
		"OPENAI_BASE_URL":                 "http://10.0.0.8:18080/api/v1/agents/u-dev/llm",
		"OPENAI_API_KEY":                  "shared-token",
		"OPENAI_MODEL":                    "gpt-5.5",
		"LARK_CHANNEL_HOME":               codexsandbox.BoxDir,
		"LARK_CHANNEL_PROFILE":            codexsandbox.ProfileName,
		"LARK_CHANNEL_CONFIG":             codexsandbox.BoxConfigPath,
		"LARK_CHANNEL_CODEX_BIN":          "/usr/local/bin/codex",
		"CODEX_HOME":                      codexsandbox.BoxCodexHomeDir,
		"CSGCLAW_CODEX_GATEWAY_WORKSPACE": codexsandbox.BoxProjectsDir,
		"LARK_CHANNEL_APP_ID":             "cli_dev",
		"LARK_CHANNEL_PARTICIPANT_ID":     "dev",
		"LARK_CHANNEL_TENANT":             "feishu",
		"APP_SECRET":                      "dev-secret",
	}
	for key, wantValue := range want {
		if got := env[key]; got != wantValue {
			t.Errorf("%s = %q, want %q", key, got, wantValue)
		}
	}
	for key, value := range env {
		if key == "APP_SECRET" {
			continue
		}
		if strings.Contains(value, "dev-secret") {
			t.Errorf("%s leaked app secret outside APP_SECRET", key)
		}
	}
}

func TestCodexSandboxRuntimeEnvVarsEnableFeishuOnlyForConfiguredAgent(t *testing.T) {
	env := codexSandboxRuntimeEnvVars(
		"http://10.0.0.8:18080",
		"shared-token",
		"missing",
		"u-missing",
		"http://10.0.0.8:18080/api/v1/agents/u-missing/llm",
		"gpt-5.5",
		staticFeishuProvider{apps: map[string]feishu.AppConfig{
			"dev": {AppID: "cli_dev", AppSecret: "dev-secret"},
		}},
	)
	for _, key := range []string{
		"LARK_CHANNEL_APP_ID",
		"LARK_CHANNEL_PARTICIPANT_ID",
		"APP_SECRET",
	} {
		if _, ok := env[key]; ok {
			t.Fatalf("%s should not be emitted for an unconfigured Feishu bot", key)
		}
	}

	env = codexSandboxRuntimeEnvVars(
		"http://10.0.0.8:18080",
		"shared-token",
		"dev",
		"u-dev",
		"http://10.0.0.8:18080/api/v1/agents/u-dev/llm",
		"gpt-5.5",
		staticFeishuProvider{apps: map[string]feishu.AppConfig{
			"dev": {AppID: "cli_dev", AppSecret: "dev-secret"},
		}},
	)
	if got, want := env["LARK_CHANNEL_APP_ID"], "cli_dev"; got != want {
		t.Fatalf("LARK_CHANNEL_APP_ID = %q, want %q", got, want)
	}
	if got, want := env["LARK_CHANNEL_PARTICIPANT_ID"], "dev"; got != want {
		t.Fatalf("LARK_CHANNEL_PARTICIPANT_ID = %q, want %q", got, want)
	}
	if got, want := env["APP_SECRET"], "dev-secret"; got != want {
		t.Fatalf("APP_SECRET = %q, want %q", got, want)
	}
	if got, want := env["OPENAI_BASE_URL"], "http://10.0.0.8:18080/api/v1/agents/u-dev/llm"; got != want {
		t.Fatalf("OPENAI_BASE_URL = %q, want %q", got, want)
	}
	if got, want := env["CODEX_HOME"], codexsandbox.BoxCodexHomeDir; got != want {
		t.Fatalf("CODEX_HOME = %q, want %q", got, want)
	}
}
