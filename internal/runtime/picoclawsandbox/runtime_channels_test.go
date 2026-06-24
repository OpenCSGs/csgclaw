package picoclawsandbox

import (
	"context"
	"testing"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	"csgclaw/internal/hub/templates"
	agentruntime "csgclaw/internal/runtime"
)

func TestRuntimeSetFeishuProviderUpdatesGatewayCreateSpecEnv(t *testing.T) {
	rt := New(Dependencies{
		FeishuProvider: feishuProviderStub{
			apps: map[string]feishu.AppConfig{
				"u-dev": {AppID: "old-app", AppSecret: "old-secret"},
			},
		},
		BuildRuntimeEnv: func(_, _, participantID, agentID, _, _ string, provider feishu.AgentCredentialProvider) map[string]string {
			env := map[string]string{"PARTICIPANT_ID": participantID}
			if _, app, ok := provider.BotConfigForAgent(agentID); ok {
				env["APP_ID"] = app.AppID
				env["APP_SECRET"] = app.AppSecret
			}
			return env
		},
		AddProfileEnv: func(envVars map[string]string, profileEnv map[string]string) {},
	})
	if err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
		RuntimeID:     "rt-u-dev",
		AgentID:       "u-dev",
		ParticipantID: "dev",
		AgentName:     "dev",
		Gateway: &agentruntime.GatewayProvision{
			ModelFallback:     "model-1",
			Server:            config.ServerConfig{AdvertiseBaseURL: "http://127.0.0.1:18080", AccessToken: "token"},
			ManagerBaseURL:    "http://127.0.0.1:18080",
			AgentHome:         t.TempDir(),
			ProjectsRoot:      t.TempDir(),
			WorkspaceTemplate: templates.PicoClawWorkerRoot,
		},
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

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
	if got, want := spec.Env["PARTICIPANT_ID"], "dev"; got != want {
		t.Fatalf("PARTICIPANT_ID = %q, want %q", got, want)
	}
}

type feishuProviderStub struct {
	apps map[string]feishu.AppConfig
}

func (p feishuProviderStub) BotConfig(botID string) (feishu.AppConfig, bool) {
	app, ok := p.apps[botID]
	return app, ok
}

func (p feishuProviderStub) BotConfigForAgent(agentID string) (string, feishu.AppConfig, bool) {
	app, ok := p.apps[agentID]
	return agentID, app, ok
}
