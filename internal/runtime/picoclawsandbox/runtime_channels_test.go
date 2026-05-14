package picoclawsandbox

import (
	"testing"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
)

func TestRuntimeSetFeishuProviderUpdatesGatewayCreateSpecEnv(t *testing.T) {
	rt := New(Dependencies{
		ModelFallback: "model-1",
		Server: config.ServerConfig{
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "token",
		},
		FeishuProvider: feishuProviderStub{
			apps: map[string]feishu.AppConfig{
				"u-dev": {AppID: "old-app", AppSecret: "old-secret"},
			},
		},
		ResolveBaseURL: func(server config.ServerConfig) string {
			return server.AdvertiseBaseURL
		},
		EnsureGatewayConfig: func(_, _ string, _ agentruntime.Profile) error { return nil },
		EnsureWorkspace: func(_, _ string) (string, error) {
			return t.TempDir(), nil
		},
		WorkspaceTemplate: func(_, _ string) (string, error) { return "", nil },
		EnsureProjectsRoot: func() (string, error) {
			return t.TempDir(), nil
		},
		BuildRuntimeEnv: func(_, _, botID, _, _ string, provider feishu.BotCredentialProvider) map[string]string {
			env := map[string]string{}
			if app, ok := provider.BotConfig(botID); ok {
				env["APP_ID"] = app.AppID
				env["APP_SECRET"] = app.AppSecret
			}
			return env
		},
		AddProfileEnv: func(envVars map[string]string, profileEnv map[string]string) {},
	})

	rt.SetFeishuProvider(feishuProviderStub{
		apps: map[string]feishu.AppConfig{
			"u-dev": {AppID: "new-app", AppSecret: "new-secret"},
		},
	})

	spec, err := rt.GatewayCreateSpec("image", "dev", "u-dev", agentruntime.Profile{})
	if err != nil {
		t.Fatalf("GatewayCreateSpec() error = %v", err)
	}
	if got, want := spec.Env["APP_ID"], "new-app"; got != want {
		t.Fatalf("APP_ID = %q, want %q", got, want)
	}
	if got, want := spec.Env["APP_SECRET"], "new-secret"; got != want {
		t.Fatalf("APP_SECRET = %q, want %q", got, want)
	}
}

type feishuProviderStub struct {
	apps map[string]feishu.AppConfig
}

func (p feishuProviderStub) BotConfig(botID string) (feishu.AppConfig, bool) {
	app, ok := p.apps[botID]
	return app, ok
}
