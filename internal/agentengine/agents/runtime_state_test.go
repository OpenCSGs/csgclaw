package agents

import (
	"testing"

	"csgclaw/internal/config"
	"csgclaw/internal/runtimeassets"
	"csgclaw/internal/sandbox"
)

func TestRuntimeProfileForAgentUsesBridgeForCodex(t *testing.T) {
	svc, err := NewController(
		config.ModelConfig{},
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		}, "manager-image:test", "",
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	profile := svc.runtimeProfileForAgent(Agent{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeKind: RuntimeKindCodex,
		AgentProfile: AgentProfile{
			Name:     "alice",
			Provider: ProviderCodex,
			ModelID:  "gpt-5.5",
			BaseURL:  "https://upstream.example/v1",
			APIKey:   "upstream-key",
			Env: map[string]string{
				" EXTRA_FLAG ": " 1 ",
			},
		},
	})

	if got, want := profile.BaseURL, "http://127.0.0.1:18080/api/v1/agents/u-alice/llm"; got != want {
		t.Fatalf("runtimeProfileForAgent().BaseURL = %q, want %q", got, want)
	}
	if got, want := profile.APIKey, svc.agentAccessToken("u-alice"); got != want {
		t.Fatalf("runtimeProfileForAgent().APIKey = %q, want %q", got, want)
	}
	if got, want := profile.ModelID, "gpt-5.5"; got != want {
		t.Fatalf("runtimeProfileForAgent().ModelID = %q, want %q", got, want)
	}
	if got, want := profile.Env["EXTRA_FLAG"], "1"; got != want {
		t.Fatalf("runtimeProfileForAgent().Env[EXTRA_FLAG] = %q, want %q", got, want)
	}
	if got, want := profile.Env["CSGCLAW_BASE_URL"], "http://127.0.0.1:18080"; got != want {
		t.Fatalf("runtimeProfileForAgent().Env[CSGCLAW_BASE_URL] = %q, want %q", got, want)
	}
	if got, want := profile.Env["CSGCLAW_ACCESS_TOKEN"], svc.agentAccessToken("u-alice"); got != want {
		t.Fatalf("runtimeProfileForAgent().Env[CSGCLAW_ACCESS_TOKEN] = %q, want %q", got, want)
	}
	if got := profile.Env[ConnectorCapabilityEnv]; got != "" {
		t.Fatalf("worker runtime connector capability = %q, want empty", got)
	}
}

func TestRuntimeProfileForAgentIgnoresPublicAdvertiseURLForCodex(t *testing.T) {
	svc, err := NewController(
		config.ModelConfig{},
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "https://public.example.test/csgclaw",
			AccessToken:      "shared-token",
		}, "manager-image:test", "",
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	profile := svc.runtimeProfileForAgent(Agent{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeKind: RuntimeKindCodex,
		AgentProfile: AgentProfile{
			Provider: ProviderCodex,
			ModelID:  "gpt-5.5",
		},
	})

	if got, want := profile.BaseURL, "http://127.0.0.1:18080/api/v1/agents/u-alice/llm"; got != want {
		t.Fatalf("runtimeProfileForAgent().BaseURL = %q, want local bridge %q", got, want)
	}
	if got, want := profile.Env["CSGCLAW_BASE_URL"], "http://127.0.0.1:18080"; got != want {
		t.Fatalf("runtimeProfileForAgent().Env[CSGCLAW_BASE_URL] = %q, want %q", got, want)
	}
}

func TestRuntimeProfileUsesScopedTokensWithoutGlobalConnectorCapability(t *testing.T) {
	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{AdvertiseBaseURL: "http://127.0.0.1:18080", AccessToken: "shared-token"}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	manager := svc.runtimeProfileForKind(RuntimeKindCodex, ManagerUserID, ManagerName, "", AgentProfile{Env: map[string]string{"CSGCLAW_CLI": "/stale/cli"}})
	if got, want := manager.Env["CSGCLAW_CLI"], runtimeassets.HostCLIPath(); got == "" || got != want {
		t.Fatalf("manager companion CLI = %q, want %q", got, want)
	}
	if manager.Env["CSGCLAW_CALLER_AGENT_ID"] != ManagerUserID {
		t.Fatal("manager CLI caller identity missing")
	}
	if got := manager.Env[ConnectorCapabilityEnv]; got != "" {
		t.Fatal("manager runtime must not receive the global connector capability")
	}
	worker := svc.runtimeProfileForKind(RuntimeKindCodex, "agent-worker", "worker", "", AgentProfile{})
	if worker.Env["CSGCLAW_CLI"] != manager.Env["CSGCLAW_CLI"] {
		t.Fatal("worker must use the same server companion CLI")
	}
	if got := worker.Env[ConnectorCapabilityEnv]; got != "" {
		t.Fatalf("worker runtime connector capability = %q, want empty", got)
	}
}

func TestRuntimeProfileForKindUsesBridgeForCodexRuntime(t *testing.T) {
	svc, err := NewController(
		config.ModelConfig{},
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		}, "manager-image:test", "",
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	profile := svc.runtimeProfileForKind(RuntimeKindCodex, "u-alice", "alice", "", AgentProfile{
		Name:     "alice",
		Provider: ProviderAPI,
		ModelID:  "gpt-4.1",
		BaseURL:  "https://api.example/v1",
		APIKey:   "api-key",
		Env: map[string]string{
			" FEATURE_FLAG ": " on ",
		},
	})

	if got, want := profile.BaseURL, "http://127.0.0.1:18080/api/v1/agents/u-alice/llm"; got != want {
		t.Fatalf("runtimeProfileForKind().BaseURL = %q, want %q", got, want)
	}
	if got, want := profile.APIKey, svc.agentAccessToken("u-alice"); got != want {
		t.Fatalf("runtimeProfileForKind().APIKey = %q, want %q", got, want)
	}
	if got, want := profile.ModelID, "gpt-4.1"; got != want {
		t.Fatalf("runtimeProfileForKind().ModelID = %q, want %q", got, want)
	}
	if got, want := profile.Env["FEATURE_FLAG"], "on"; got != want {
		t.Fatalf("runtimeProfileForKind().Env[FEATURE_FLAG] = %q, want %q", got, want)
	}
	if got, want := profile.Env["CSGCLAW_BASE_URL"], "http://127.0.0.1:18080"; got != want {
		t.Fatalf("runtimeProfileForKind().Env[CSGCLAW_BASE_URL] = %q, want %q", got, want)
	}
	if got, want := profile.Env["CSGCLAW_ACCESS_TOKEN"], svc.agentAccessToken("u-alice"); got != want {
		t.Fatalf("runtimeProfileForKind().Env[CSGCLAW_ACCESS_TOKEN] = %q, want %q", got, want)
	}
}

func TestRuntimeProfileForKindUsesBridgeForDSHRuntime(t *testing.T) {
	svc, err := NewController(
		config.ModelConfig{},
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		}, "manager-image:test", "",
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	profile := svc.runtimeProfileForKind(RuntimeKindDSH, "u-dsh", "dsh", "", AgentProfile{
		Name:     "dsh",
		Provider: ProviderAPI,
		ModelID:  "qwen3.7-plus",
		BaseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
		APIKey:   "dashscope-key",
	})

	if got, want := profile.BaseURL, "http://127.0.0.1:18080/api/v1/agents/u-dsh/llm"; got != want {
		t.Fatalf("runtimeProfileForKind().BaseURL = %q, want %q", got, want)
	}
	if got, want := profile.APIKey, svc.agentAccessToken("u-dsh"); got != want {
		t.Fatalf("runtimeProfileForKind().APIKey = %q, want %q", got, want)
	}
	if got, want := profile.Env["CSGCLAW_ACCESS_TOKEN"], svc.agentAccessToken("u-dsh"); got != want {
		t.Fatalf("runtimeProfileForKind().Env[CSGCLAW_ACCESS_TOKEN] = %q, want %q", got, want)
	}
	if got, want := profile.ModelID, "qwen3.7-plus"; got != want {
		t.Fatalf("runtimeProfileForKind().ModelID = %q, want %q", got, want)
	}
	if got := profile.InputModalities; len(got) != 2 || got[0] != "text" || got[1] != "image" {
		t.Fatalf("runtimeProfileForKind().InputModalities = %v, want text and image", got)
	}
}

func TestRuntimeProfileForKindUsesDiscoveredVisionCapability(t *testing.T) {
	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{ListenAddr: "127.0.0.1:18080"}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.SetLLMConfig(config.LLMConfig{Providers: map[string]config.ProviderConfig{
		"custom": {
			BaseURL: "https://models.example/v1", APIKey: "key",
			Models: []string{"custom-vision"}, VisionModels: []string{"custom-vision"},
		},
	}})

	profile := svc.runtimeProfileForKind(RuntimeKindDSH, "u-dsh", "dsh", "", AgentProfile{
		Name: "dsh", ModelProviderID: "custom", ModelID: "custom-vision",
	})
	if got := profile.InputModalities; len(got) != 2 || got[0] != "text" || got[1] != "image" {
		t.Fatalf("runtimeProfileForKind().InputModalities = %v, want discovered text and image", got)
	}
}

func TestRuntimeProfileForKindUsesHostReachableBridgeForCodexRuntime(t *testing.T) {
	orig := localIPv4Resolver
	localIPv4Resolver = func() string {
		return "198.18.0.1"
	}
	t.Cleanup(func() {
		localIPv4Resolver = orig
	})

	svc, err := NewController(
		config.ModelConfig{},
		config.ServerConfig{
			ListenAddr:  "0.0.0.0:18080",
			AccessToken: "shared-token",
		}, "manager-image:test", "",
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	profile := svc.runtimeProfileForKind(RuntimeKindCodex, "u-developer", "developer", "", AgentProfile{
		Name:     "developer",
		Provider: ProviderCodex,
		ModelID:  "gpt-5.4",
	})

	if got, want := profile.BaseURL, "http://127.0.0.1:18080/api/v1/agents/u-developer/llm"; got != want {
		t.Fatalf("runtimeProfileForKind().BaseURL = %q, want host-reachable %q", got, want)
	}
	if got, want := profile.Env["CSGCLAW_BASE_URL"], "http://127.0.0.1:18080"; got != want {
		t.Fatalf("runtimeProfileForKind().Env[CSGCLAW_BASE_URL] = %q, want %q", got, want)
	}
	if got, want := profile.Env["CSGCLAW_ACCESS_TOKEN"], svc.agentAccessToken("u-developer"); got != want {
		t.Fatalf("runtimeProfileForKind().Env[CSGCLAW_ACCESS_TOKEN] = %q, want %q", got, want)
	}
}

func TestRuntimeRecordForAgentPreservesEmptyRuntimeKind(t *testing.T) {
	rt := runtimeRecordForAgent(Agent{
		ID:        "u-alice",
		RuntimeID: "rt-u-alice",
		Role:      RoleWorker,
	})
	if rt.Kind != "" {
		t.Fatalf("runtimeRecordForAgent().Kind = %q, want empty", rt.Kind)
	}
}

func TestMigratedStoppedHostAgentWithoutRuntimeMetadataStaysStopped(t *testing.T) {
	item := Agent{ID: "agent-migrated", Name: "Migrated", RuntimeKind: RuntimeKindCodex, Status: "stopped", DesiredState: DesiredStateStopped}
	got := statusAfterHydrateFailure(item, "read_runtime_info", sandbox.ErrNotFound)
	if got.Status != "stopped" {
		t.Fatalf("unstarted migrated Agent status = %q", got.Status)
	}
	item.BoxID = "old-handle"
	got = statusAfterHydrateFailure(item, "read_runtime_info", sandbox.ErrNotFound)
	if got.Status != "unknown" {
		t.Fatal("missing existing runtime was hidden")
	}
}
