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
		DefaultManagerTemplate: "builtin.picoclaw-manager",
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
