package agent

import (
	"context"
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
	if len(results) != 3 {
		t.Fatalf("results len = %d, want 3 builtin checks", len(results))
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
