package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/config"
)

func TestRefreshModelProviderCatalogUpdatesBuiltinAndPreservesDefaults(t *testing.T) {
	got, results, changed := RefreshModelProviderCatalog(context.Background(), config.LLMConfig{}, func(_ context.Context, input ModelProviderCheckInput) ModelProviderCheckResult {
		if input.ID != ModelProviderIDCSGHubLite {
			return ModelProviderCheckResult{ID: input.ID, Status: ModelProviderStatusFailed}
		}
		return ModelProviderCheckResult{
			ID:     input.ID,
			Status: ModelProviderStatusConnected,
			Models: []string{"qwen3"},
		}
	})

	if !changed {
		t.Fatal("RefreshModelProviderCatalog() changed = false, want true")
	}
	if len(results) != 4 {
		t.Fatalf("results len = %d, want 4 builtin checks", len(results))
	}
	provider := got.Providers[ModelProviderIDCSGHubLite]
	if provider.BaseURL != defaultCSGHubLiteBaseURL {
		t.Fatalf("CSGHub Lite BaseURL = %q, want default %q", provider.BaseURL, defaultCSGHubLiteBaseURL)
	}
	if provider.APIKey != defaultCSGHubLiteAPIKey {
		t.Fatalf("CSGHub Lite APIKey = %q, want default key", provider.APIKey)
	}
	if len(provider.Models) != 1 || provider.Models[0] != "qwen3" {
		t.Fatalf("CSGHub Lite models = %+v, want [qwen3]", provider.Models)
	}
}

func TestCheckModelProviderUsesOpenCSGAIGatewayCredentials(t *testing.T) {
	var authHeader string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		authHeader = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[{"id":"opencsg/deepseek-v4"},{"id":"opencsg/deepseek-v4"}]}`))
	}))
	defer upstream.Close()

	oldCredentials := defaultCSGHubCredentials
	t.Cleanup(func() { defaultCSGHubCredentials = oldCredentials })
	defaultCSGHubCredentials = func(context.Context, *http.Client) (string, string, bool, error) {
		return upstream.URL + "/v1", "gk_builtin-test", true, nil
	}

	got := CheckModelProvider(context.Background(), ModelProviderCheckInput{ID: ModelProviderIDOpenCSG})

	if got.Status != ModelProviderStatusConnected {
		t.Fatalf("Status = %q, want connected; message=%q", got.Status, got.Message)
	}
	if authHeader != "Bearer gk_builtin-test" {
		t.Fatalf("Authorization = %q, want OpenCSG AI Gateway token", authHeader)
	}
	if strings.Join(got.Models, ",") != "opencsg/deepseek-v4" {
		t.Fatalf("Models = %+v, want deduplicated OpenCSG models", got.Models)
	}
}

func TestModelProviderCatalogExposesOpenCSGKind(t *testing.T) {
	oldGatewayBaseURL := defaultOpenCSGAIGatewayBaseURL
	t.Cleanup(func() { defaultOpenCSGAIGatewayBaseURL = oldGatewayBaseURL })
	defaultOpenCSGAIGatewayBaseURL = func() string {
		return "https://aigateway.opencsg-stg.com/v1"
	}

	catalog := ModelProviderCatalogFromLLM(config.LLMConfig{})

	var provider ModelProviderSummary
	for _, item := range catalog.Providers {
		if item.ID == ModelProviderIDOpenCSG {
			provider = item
			break
		}
	}
	if provider.ID == "" {
		t.Fatalf("OpenCSG provider missing from catalog: %+v", catalog.Providers)
	}
	if provider.Kind != ModelProviderIDOpenCSG {
		t.Fatalf("OpenCSG provider kind = %q, want %q", provider.Kind, ModelProviderIDOpenCSG)
	}
	if provider.BaseURL != "https://aigateway.opencsg-stg.com/v1" {
		t.Fatalf("OpenCSG provider BaseURL = %q, want stg gateway", provider.BaseURL)
	}
}

func TestRefreshModelProviderCatalogReplacesStaleProfileModels(t *testing.T) {
	llm := config.LLMConfig{
		Default: "openai.gpt-old",
		Providers: map[string]config.ProviderConfig{
			"openai": {
				DisplayName: "Team OpenAI",
				BaseURL:     "https://api.openai.example/v1",
				APIKey:      "sk-team",
				Models:      []string{"gpt-old"},
			},
		},
		Profiles: map[string]config.ModelConfig{
			"openai": {
				BaseURL: "https://api.openai.example/v1",
				APIKey:  "sk-team",
				ModelID: "gpt-old",
			},
		},
	}

	got, _, changed := RefreshModelProviderCatalog(context.Background(), llm, func(_ context.Context, input ModelProviderCheckInput) ModelProviderCheckResult {
		if input.ID != "openai" {
			return ModelProviderCheckResult{ID: input.ID, Status: ModelProviderStatusFailed}
		}
		return ModelProviderCheckResult{
			ID:     input.ID,
			Status: ModelProviderStatusConnected,
			Models: []string{"gpt-new", "gpt-new-mini"},
		}
	})

	if !changed {
		t.Fatal("RefreshModelProviderCatalog() changed = false, want true")
	}
	if got.Profiles["openai"].ModelID != "" {
		t.Fatalf("Profiles[openai] = %+v, want stale profile removed", got.Profiles["openai"])
	}
	if got := got.Providers["openai"].Models; len(got) != 2 || got[0] != "gpt-new" || got[1] != "gpt-new-mini" {
		t.Fatalf("Providers[openai].Models = %+v, want discovered models only", got)
	}
}

func TestApplyModelProviderCheckResultPersistsStatusMetadata(t *testing.T) {
	llm := config.LLMConfig{
		Providers: map[string]config.ProviderConfig{
			"openai": {
				DisplayName: "Team OpenAI",
				BaseURL:     "https://api.openai.example/v1",
				APIKey:      "sk-team",
				Models:      []string{"gpt-old"},
			},
		},
	}

	got, changed := ApplyModelProviderCheckResult(llm, "openai", ModelProviderCheckResult{
		ID:            "openai",
		Status:        ModelProviderStatusConnected,
		Message:       "connected",
		Models:        []string{"gpt-new"},
		LastCheckedAt: "2026-06-23T12:00:00Z",
	})

	if !changed {
		t.Fatal("ApplyModelProviderCheckResult() changed = false, want true")
	}
	provider := got.Providers["openai"]
	if provider.Status != ModelProviderStatusConnected {
		t.Fatalf("provider.Status = %q, want connected", provider.Status)
	}
	if provider.Message != "connected" {
		t.Fatalf("provider.Message = %q, want connected", provider.Message)
	}
	if provider.LastCheckedAt != "2026-06-23T12:00:00Z" {
		t.Fatalf("provider.LastCheckedAt = %q, want check timestamp", provider.LastCheckedAt)
	}
	summary := ModelProviderCatalogFromLLM(got)
	var custom ModelProviderSummary
	for _, provider := range summary.Providers {
		if provider.ID == "openai" {
			custom = provider
			break
		}
	}
	if custom.Status != ModelProviderStatusConnected {
		t.Fatalf("summary.Status = %q, want connected", custom.Status)
	}
}

func TestApplyModelProviderCheckResultDoesNotCreateDraftCustomProvider(t *testing.T) {
	got, changed := ApplyModelProviderCheckResult(config.LLMConfig{}, "openai-draft", ModelProviderCheckResult{
		ID:            "openai-draft",
		Status:        ModelProviderStatusConnected,
		Message:       "connected",
		Models:        []string{"gpt-live"},
		LastCheckedAt: "2026-06-23T12:00:00Z",
	})

	if changed {
		t.Fatal("ApplyModelProviderCheckResult() changed = true, want false for non-existing custom draft")
	}
	if _, ok := got.Normalized().Providers["openai-draft"]; ok {
		t.Fatal("draft custom provider was created during check")
	}
}
