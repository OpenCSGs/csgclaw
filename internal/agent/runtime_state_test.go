package agent

import (
	"path/filepath"
	"testing"

	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
)

func TestPicoClawRuntimeHostAgentHomeUsesAgentRoot(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	host := svc.PicoClawRuntimeHost()
	got, err := host.AgentHome("alice")
	if err != nil {
		t.Fatalf("host.AgentHome() error = %v", err)
	}

	want := filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, "alice")
	if got != want {
		t.Fatalf("host.AgentHome() = %q, want %q", got, want)
	}
}

func TestRuntimeProfileForAgentUsesBridgeForCodex(t *testing.T) {
	svc, err := NewService(
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
	if got, want := profile.APIKey, "shared-token"; got != want {
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
	if got, want := profile.Env["CSGCLAW_ACCESS_TOKEN"], "shared-token"; got != want {
		t.Fatalf("runtimeProfileForAgent().Env[CSGCLAW_ACCESS_TOKEN] = %q, want %q", got, want)
	}
}

func TestRuntimeProfileForKindUsesBridgeForCodexRuntime(t *testing.T) {
	svc, err := NewService(
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
	if got, want := profile.APIKey, "shared-token"; got != want {
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
	if got, want := profile.Env["CSGCLAW_ACCESS_TOKEN"], "shared-token"; got != want {
		t.Fatalf("runtimeProfileForKind().Env[CSGCLAW_ACCESS_TOKEN] = %q, want %q", got, want)
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

	svc, err := NewService(
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
	if got, want := profile.Env["CSGCLAW_ACCESS_TOKEN"], "shared-token"; got != want {
		t.Fatalf("runtimeProfileForKind().Env[CSGCLAW_ACCESS_TOKEN] = %q, want %q", got, want)
	}
}

func TestPicoClawRuntimeHostResolveRuntimeProfilePreservesAPIProfile(t *testing.T) {
	svc, err := NewService(
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

	svc.agents["u-alice"] = Agent{
		ID:        "u-alice",
		Name:      "alice",
		RuntimeID: "rt-alice",
		AgentProfile: AgentProfile{
			Name:     "alice",
			Provider: ProviderAPI,
			ModelID:  "gpt-4.1",
			BaseURL:  "https://api.example/v1",
			APIKey:   "api-key",
			Env: map[string]string{
				" FEATURE_FLAG ": " on ",
			},
		},
	}

	host := svc.PicoClawRuntimeHost()
	profile, err := host.ResolveRuntimeProfile(agentruntime.Handle{RuntimeID: "rt-alice"})
	if err != nil {
		t.Fatalf("host.ResolveRuntimeProfile() error = %v", err)
	}

	if got, want := profile.BaseURL, "https://api.example/v1"; got != want {
		t.Fatalf("host.ResolveRuntimeProfile().BaseURL = %q, want %q", got, want)
	}
	if got, want := profile.APIKey, "api-key"; got != want {
		t.Fatalf("host.ResolveRuntimeProfile().APIKey = %q, want %q", got, want)
	}
	if got, want := profile.ModelID, "gpt-4.1"; got != want {
		t.Fatalf("host.ResolveRuntimeProfile().ModelID = %q, want %q", got, want)
	}
	if got, want := profile.Env["FEATURE_FLAG"], "on"; got != want {
		t.Fatalf("host.ResolveRuntimeProfile().Env[FEATURE_FLAG] = %q, want %q", got, want)
	}
}

func TestRuntimeProfileForKindUsesEffectiveCSGHubLiteTarget(t *testing.T) {
	svc, err := NewService(
		config.ModelConfig{},
		config.ServerConfig{},
		"manager-image:test",
		"",
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	profile := svc.runtimeProfileForKind(RuntimeKindPicoClawSandbox, "u-alice", "alice", "", AgentProfile{
		Name:     "alice",
		Provider: ProviderCSGHubLite,
		ModelID:  "Qwen3-0.6B-GGUF",
		BaseURL:  "https://api.deepseek.com",
		APIKey:   "stale-key",
	})

	if got, want := profile.BaseURL, defaultCSGHubLiteBaseURL; got != want {
		t.Fatalf("runtimeProfileForKind().BaseURL = %q, want CSGHub Lite default %q", got, want)
	}
	if got, want := profile.APIKey, defaultCSGHubLiteAPIKey; got != want {
		t.Fatalf("runtimeProfileForKind().APIKey = %q, want CSGHub Lite default key", got)
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
