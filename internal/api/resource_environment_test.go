package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	}, false)
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

func TestApplyOpenCSGEnvironmentToHubConfigPreservesExplicitOfficialURL(t *testing.T) {
	cfg := applyOpenCSGEnvironmentToHubConfig(config.HubConfig{
		Registries: []config.HubRegistryConfig{
			{Name: config.DefaultOfficialHubRegistryName, Kind: config.HubRegistryKindRemote, URL: "https://hub.example.test", Enabled: true},
		},
	}, auth.Environment{
		CSGHubBaseURL: auth.StageCSGHubBaseURL,
	}, true)
	resolved := cfg.Resolved()
	var official config.HubRegistryConfig
	for _, registry := range resolved.Registries {
		if registry.Name == config.DefaultOfficialHubRegistryName {
			official = registry
			break
		}
	}
	if got, want := official.URL, "https://hub.example.test"; got != want {
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

func TestCurrentOpenCSGEnvironmentUsesManagedHubURLWithoutLogin(t *testing.T) {
	t.Setenv("CSGHUB_API_BASE_URL", "https://opencsg-stg.com/")
	restore := stubAuthStatus(func(*http.Request) (auth.Status, error) {
		return auth.Status{}, nil
	})
	defer restore()

	env := (&Handler{}).currentOpenCSGEnvironment(httptest.NewRequest(http.MethodGet, "/api/v1/hub/templates", nil))
	if got, want := env.CSGHubBaseURL, "https://opencsg-stg.com"; got != want {
		t.Fatalf("CSGHubBaseURL = %q, want %q", got, want)
	}
}

func TestCurrentOpenCSGEnvironmentPrefersLoginOverManagedHubURL(t *testing.T) {
	t.Setenv("CSGHUB_API_BASE_URL", "https://managed.example.test/")
	restore := stubAuthStatus(func(*http.Request) (auth.Status, error) {
		return auth.Status{
			Authenticated:  true,
			OpenCSGBaseURL: auth.StageOpenCSGBaseURL,
			BaseURL:        auth.StageCSGHubBaseURL,
		}, nil
	})
	defer restore()

	env := (&Handler{}).currentOpenCSGEnvironment(httptest.NewRequest(http.MethodGet, "/api/v1/hub/templates", nil))
	if got, want := env.CSGHubBaseURL, auth.StageCSGHubBaseURL; got != want {
		t.Fatalf("CSGHubBaseURL = %q, want %q", got, want)
	}
}

func TestHubServiceUsesManagedCredentialsWithoutLogin(t *testing.T) {
	hubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer managed-token"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"total":0}`))
	}))
	defer hubServer.Close()
	t.Setenv("CSGHUB_API_BASE_URL", hubServer.URL)
	t.Setenv("CSGHUB_USER_TOKEN", "managed-token")
	t.Setenv("CSGHUB_USER_NAME", "alice")
	restore := stubAuthStatus(func(*http.Request) (auth.Status, error) {
		return auth.Status{}, nil
	})
	defer restore()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	hubSvc, err := (&Handler{configPath: configPath}).hubServiceForRequest(
		httptest.NewRequest(http.MethodGet, "/api/v1/hub/templates", nil),
	)
	if err != nil {
		t.Fatalf("hubServiceForRequest() error = %v", err)
	}
	if _, err := hubSvc.List(context.Background()); err != nil {
		t.Fatalf("List() error = %v", err)
	}
}

func TestHubServiceUsesManagedTokenWithoutBaseURLEnvironment(t *testing.T) {
	hubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer managed-token"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"total":0}`))
	}))
	defer hubServer.Close()
	t.Setenv("CSGHUB_API_BASE_URL", "")
	t.Setenv("CSGHUB_USER_TOKEN", "managed-token")
	restore := stubAuthStatus(func(*http.Request) (auth.Status, error) {
		return auth.Status{}, nil
	})
	defer restore()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	configBody := []byte("[hub]\n\n[[hub.registries]]\nname = \"official\"\nkind = \"remote\"\nurl = \"" + hubServer.URL + "\"\nenabled = true\n")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	hubSvc, err := (&Handler{configPath: configPath}).hubServiceForRequest(
		httptest.NewRequest(http.MethodGet, "/api/v1/hub/templates", nil),
	)
	if err != nil {
		t.Fatalf("hubServiceForRequest() error = %v", err)
	}
	if _, err := hubSvc.List(context.Background()); err != nil {
		t.Fatalf("List() error = %v", err)
	}
}

func TestOfficialHubBaseURLForRequestPreservesExplicitOfficialRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`
[hub]
default_registry = "builtin"
default_publish_registry = "local"

[[hub.registries]]
name = "official"
kind = "remote"
url = "https://hub.example.test"
enabled = true
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	restore := stubAuthStatus(func(*http.Request) (auth.Status, error) {
		return auth.Status{
			Authenticated:  true,
			OpenCSGBaseURL: auth.StageOpenCSGBaseURL,
			BaseURL:        auth.StageCSGHubBaseURL,
		}, nil
	})
	defer restore()

	got := (&Handler{}).officialHubBaseURLForRequest(httptest.NewRequest(http.MethodGet, "/api/v1/server/config", nil), cfg)
	if want := "https://hub.example.test"; got != want {
		t.Fatalf("officialHubBaseURLForRequest() = %q, want %q", got, want)
	}
}
