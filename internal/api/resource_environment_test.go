package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"csgclaw/internal/auth"
	"csgclaw/internal/config"
)

func TestSkillConfigForEnvironmentUsesDefaultRegistryForStage(t *testing.T) {
	cfg := skillConfigForEnvironment(config.SkillConfig{}, auth.Environment{
		OpenCSGBaseURL: auth.StageOpenCSGBaseURL,
		CSGHubBaseURL:  auth.StageCSGHubBaseURL,
	})
	if got, want := cfg.BaseURL, config.DefaultSkillBaseURL; got != want {
		t.Fatalf("BaseURL = %q, want %q", got, want)
	}
	if !cfg.OfficialBaseURLSet || cfg.OfficialBaseURL != "" {
		t.Fatalf("OfficialBaseURL = %q set=%t, want disabled", cfg.OfficialBaseURL, cfg.OfficialBaseURLSet)
	}
}

func TestSkillConfigForEnvironmentKeepsConfiguredRegistry(t *testing.T) {
	cfg := skillConfigForEnvironment(config.SkillConfig{
		BaseURL:         "https://skills.example.test",
		OfficialBaseURL: "https://official.example.test",
	}, auth.Environment{
		OpenCSGBaseURL: auth.StageOpenCSGBaseURL,
		CSGHubBaseURL:  auth.StageCSGHubBaseURL,
	})
	if got, want := cfg.BaseURL, "https://skills.example.test"; got != want {
		t.Fatalf("BaseURL = %q, want %q", got, want)
	}
	if got, want := cfg.OfficialBaseURL, "https://official.example.test"; got != want {
		t.Fatalf("OfficialBaseURL = %q, want %q", got, want)
	}
}

func TestApplyOpenCSGEnvironmentToHubConfigUsesLoginHubURL(t *testing.T) {
	cfg := applyOpenCSGEnvironmentToHubConfig(config.HubConfig{}, auth.Environment{
		CSGHubBaseURL: auth.StageCSGHubBaseURL,
	})
	resolved := cfg.Resolved()
	var official config.HubRegistryConfig
	for _, registry := range resolved.Registries {
		if registry.Name == config.DefaultOfficialHubRegistryName {
			official = registry
			break
		}
	}
	if got, want := official.URL, auth.StageCSGHubBaseURL; got != want {
		t.Fatalf("official URL = %q, want %q", got, want)
	}
}

func TestCurrentOpenCSGEnvironmentPrefersLoginSiteOverStoredHubURL(t *testing.T) {
	restore := stubAuthStatus(func(*http.Request) (auth.Status, error) {
		return auth.Status{
			Authenticated:  true,
			OpenCSGBaseURL: auth.StageOpenCSGBaseURL,
			BaseURL:        "https://csgclaw.opencsg-stg.com",
		}, nil
	})
	defer restore()

	env := (&Handler{}).currentOpenCSGEnvironment(httptest.NewRequest(http.MethodGet, "/api/v1/hub/templates", nil))
	if got, want := env.CSGHubBaseURL, auth.StageCSGHubBaseURL; got != want {
		t.Fatalf("CSGHubBaseURL = %q, want %q", got, want)
	}
}
