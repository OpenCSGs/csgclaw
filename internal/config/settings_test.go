package config

import (
	"strings"
	"testing"
)

func TestApplyUserSettingsUpdatesExposedFields(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{
			ListenAddr:       "127.0.0.1:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "secret",
			ShowUpgrade:      true,
		},
		Bootstrap: BootstrapConfig{
			DefaultManagerTemplate: DefaultBootstrapManagerTemplate,
			DefaultWorkerTemplate:  DefaultBootstrapWorkerTemplate,
		},
		Sandbox: SandboxConfig{Provider: BoxLiteProvider},
		Hub: HubConfig{
			Registries: []HubRegistryConfig{
				{Name: DefaultHubRegistry, Kind: HubRegistryKindBuiltin, Enabled: true},
				{Name: DefaultHubPublishRegistry, Kind: HubRegistryKindLocal, Path: "/old/hub", Enabled: true},
				{Name: DefaultOfficialHubRegistryName, Kind: HubRegistryKindRemote, URL: DefaultOfficialHubRegistryURL, Enabled: true},
				{Name: "team", Kind: HubRegistryKindRemote, URL: "https://team.example.com", Enabled: true},
			},
		},
		Models: LLMConfig{
			Default: "default.model",
			Providers: map[string]ProviderConfig{
				"default": {
					BaseURL: "http://127.0.0.1:4000",
					APIKey:  "sk",
					Models:  []string{"model"},
				},
			},
		},
	}

	updated, err := ApplyUserSettings(cfg, UserSettings{
		ListenAddr:             "0.0.0.0:19080",
		AdvertiseBaseURL:       "http://192.168.1.10:19080/",
		AccessToken:            "new-secret",
		ShowUpgrade:            false,
		SandboxProvider:        DockerProvider,
		HubLocalPath:           "/new/hub",
		DefaultManagerTemplate: "builtin.manager-codex",
		DefaultWorkerTemplate:  "builtin.picoclaw-worker",
	})
	if err != nil {
		t.Fatalf("ApplyUserSettings() error = %v", err)
	}
	if updated.Server.ListenAddr != "0.0.0.0:19080" {
		t.Fatalf("ListenAddr = %q", updated.Server.ListenAddr)
	}
	if updated.Server.AdvertiseBaseURL != "http://192.168.1.10:19080" {
		t.Fatalf("AdvertiseBaseURL = %q", updated.Server.AdvertiseBaseURL)
	}
	if updated.Server.ShowUpgrade {
		t.Fatalf("ShowUpgrade = true, want false")
	}
	if updated.Sandbox.Provider != DockerProvider {
		t.Fatalf("Sandbox.Provider = %q, want %q", updated.Sandbox.Provider, DockerProvider)
	}
	resolvedHub := updated.Hub.Resolved()
	if got, want := resolvedHub.Registries[1].Path, "/new/hub"; got != want {
		t.Fatalf("local hub path = %q, want %q", got, want)
	}
	if got, want := resolvedHub.Registries[2].URL, DefaultOfficialHubRegistryURL; got != want {
		t.Fatalf("official hub URL = %q, want %q", got, want)
	}
	if got, want := resolvedHub.Registries[3].URL, "https://team.example.com"; got != want {
		t.Fatalf("custom hub URL = %q, want preserved %q", got, want)
	}
	if updated.Models.Default != "default.model" {
		t.Fatalf("Models.Default = %q, want preserved", updated.Models.Default)
	}
}

func TestAccessTokenPreview(t *testing.T) {
	if got, want := AccessTokenPreview("secret"), ""; got != want {
		t.Fatalf("AccessTokenPreview(short) = %q, want empty", got)
	}
	if got, want := AccessTokenPreview("your-shared-token"), "your..."; got != want {
		t.Fatalf("AccessTokenPreview() = %q, want %q", got, want)
	}
}

func TestApplyUserSettingsRejectsInvalidListenAddr(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{
			ListenAddr:  "127.0.0.1:18080",
			AccessToken: "secret",
		},
		Bootstrap: BootstrapConfig{
			DefaultManagerTemplate: DefaultBootstrapManagerTemplate,
			DefaultWorkerTemplate:  DefaultBootstrapWorkerTemplate,
		},
		Sandbox: SandboxConfig{Provider: BoxLiteProvider},
	}
	_, err := ApplyUserSettings(cfg, UserSettings{
		ListenAddr:             "not-an-address",
		AccessToken:            "secret",
		ShowUpgrade:            true,
		SandboxProvider:        BoxLiteProvider,
		DefaultManagerTemplate: DefaultBootstrapManagerTemplate,
		DefaultWorkerTemplate:  DefaultBootstrapWorkerTemplate,
	})
	if err == nil || !strings.Contains(err.Error(), "listen_addr") {
		t.Fatalf("ApplyUserSettings() error = %v, want listen_addr validation failure", err)
	}
}
