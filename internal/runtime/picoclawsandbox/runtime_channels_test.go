package picoclawsandbox

import (
	"testing"

	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
)

func TestRuntimeSetChannelsUpdatesGatewayCreateSpecEnv(t *testing.T) {
	rt := New(Dependencies{
		ModelFallback: "model-1",
		Server: config.ServerConfig{
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "token",
		},
		Channels: config.ChannelsConfig{
			Feishu: map[string]config.FeishuConfig{
				"u-dev": {AppID: "old-app", AppSecret: "old-secret"},
			},
		},
		ResolveBaseURL: func(server config.ServerConfig) string {
			return server.AdvertiseBaseURL
		},
		EnsureGatewayConfig: func(_, _, _ string) error { return nil },
		EnsureWorkspace: func(_, _ string) (string, error) {
			return t.TempDir(), nil
		},
		WorkspaceTemplate: func(_, _ string) string { return "" },
		EnsureProjectsRoot: func() (string, error) {
			return t.TempDir(), nil
		},
		BuildRuntimeEnv: func(_, _, botID, _, _ string, channels config.ChannelsConfig) map[string]string {
			env := map[string]string{}
			if feishu, ok := channels.Feishu[botID]; ok {
				env["APP_ID"] = feishu.AppID
				env["APP_SECRET"] = feishu.AppSecret
			}
			return env
		},
		AddProfileEnv: func(envVars map[string]string, profileEnv map[string]string) {},
	})

	rt.SetChannels(config.ChannelsConfig{
		Feishu: map[string]config.FeishuConfig{
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
