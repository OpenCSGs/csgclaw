package agents

import (
	"context"
	"csgclaw/internal/cliproxy"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"csgclaw/internal/config"
	"csgclaw/internal/modelprovider"
)

func TestRefreshModelProviderCatalogUpdatesBuiltinAndPreservesDefaults(t *testing.T) {
	got, results, changed := RefreshModelProviderCatalog(context.Background(), config.LLMConfig{}, func(_ context.Context, input ModelProviderCheckInput) ModelProviderCheckResult {
		if input.ID != ModelProviderIDCSGHubLite {
			return ModelProviderCheckResult{ID: input.ID, Status: ModelProviderStatusFailed}
		}
		return ModelProviderCheckResult{
			ID:           input.ID,
			Status:       ModelProviderStatusConnected,
			Models:       []string{"qwen3"},
			VisionModels: []string{"qwen3"},
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
	if len(provider.VisionModels) != 1 || provider.VisionModels[0] != "qwen3" {
		t.Fatalf("CSGHub Lite vision models = %+v, want [qwen3]", provider.VisionModels)
	}
}

func TestApplyModelProviderCheckResultPersistsResolvedCSGHubLiteDesktopAPI(t *testing.T) {
	llm := config.LLMConfig{Providers: map[string]config.ProviderConfig{
		ModelProviderIDCSGHubLite: {
			BaseURL: modelprovider.CSGHubLiteDefaultBaseURL,
		},
	}}

	got, changed := ApplyModelProviderCheckResult(llm, ModelProviderIDCSGHubLite, ModelProviderCheckResult{
		ID:              ModelProviderIDCSGHubLite,
		ResolvedBaseURL: modelprovider.CSGHubLiteDesktopAPIBaseURL,
		Status:          ModelProviderStatusConnected,
		Models:          []string{"Qwen3.5-2B"},
	})

	if !changed {
		t.Fatal("ApplyModelProviderCheckResult() changed = false, want true")
	}
	if baseURL := got.Providers[ModelProviderIDCSGHubLite].BaseURL; baseURL != modelprovider.CSGHubLiteDesktopAPIBaseURL {
		t.Fatalf("BaseURL = %q, want %q", baseURL, modelprovider.CSGHubLiteDesktopAPIBaseURL)
	}
}

func TestCheckModelProviderUsesOpenCSGAIGatewayCredentials(t *testing.T) {
	var authHeader string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		authHeader = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[
			{"id":"opencsg/deepseek-v4","tasks":["text-generation"]},
			{"id":"qwen3.7-plus","tasks":["text-generation","image-text-to-text"]},
			{"id":"opencsg/deepseek-v4","tasks":["text-generation"]}
		]}`))
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
	if strings.Join(got.Models, ",") != "qwen3.7-plus,opencsg/deepseek-v4" {
		t.Fatalf("Models = %+v, want deduplicated OpenCSG models", got.Models)
	}
	if strings.Join(got.VisionModels, ",") != "qwen3.7-plus" {
		t.Fatalf("VisionModels = %+v, want OpenCSG tasks metadata to declare qwen3.7-plus as vision", got.VisionModels)
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
	if provider.Preset != ModelProviderIDOpenCSG {
		t.Fatalf("OpenCSG provider preset = %q, want %q", provider.Preset, ModelProviderIDOpenCSG)
	}
	if provider.BaseURL != "https://aigateway.opencsg-stg.com/v1" {
		t.Fatalf("OpenCSG provider BaseURL = %q, want stg gateway", provider.BaseURL)
	}
}

func TestModelProviderCatalogInfersPresetForLegacyCustomProvider(t *testing.T) {
	catalog := ModelProviderCatalogFromLLM(config.LLMConfig{
		Providers: map[string]config.ProviderConfig{
			"legacy-zhipu": {
				DisplayName: "Legacy Zhipu",
				BaseURL:     "https://open.bigmodel.cn/api/paas/v4",
				Models:      []string{"glm-4.5"},
			},
		},
	})

	var provider ModelProviderSummary
	for _, item := range catalog.Providers {
		if item.ID == "legacy-zhipu" {
			provider = item
			break
		}
	}
	if provider.Preset != ModelProviderPresetZhipu {
		t.Fatalf("legacy custom provider preset = %q, want %q", provider.Preset, ModelProviderPresetZhipu)
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

func TestClearModelProviderCachedStateRemovesModelsAndCheckMetadata(t *testing.T) {
	llm := config.LLMConfig{
		Providers: map[string]config.ProviderConfig{
			ModelProviderIDOpenCSG: {
				Models:        []string{"prod-model"},
				Status:        ModelProviderStatusConnected,
				Message:       "connected",
				LastCheckedAt: "2026-07-16T09:00:00Z",
			},
		},
		Profiles: map[string]config.ModelConfig{
			ModelProviderIDOpenCSG: {ModelID: "prod-model"},
		},
	}

	got, changed := ClearModelProviderCachedState(llm, ModelProviderIDOpenCSG)

	if !changed {
		t.Fatal("ClearModelProviderCachedState() changed = false, want true")
	}
	provider := got.Providers[ModelProviderIDOpenCSG]
	if len(provider.Models) != 0 || provider.Status != "" || provider.Message != "" || provider.LastCheckedAt != "" {
		t.Fatalf("cleared provider = %+v, want no cached models or check metadata", provider)
	}
	if _, exists := got.Profiles[ModelProviderIDOpenCSG]; exists {
		t.Fatal("stale generated OpenCSG profile was not removed")
	}
}

func TestImageModelDiscoveryCachesAndClearsNonGPTModels(t *testing.T) {
	includeImages := true
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if includeImages {
			_, _ = w.Write([]byte(`{"data":[{"id":"chat-vision","task":"text-generation,image-text-to-text"},{"id":"vendor-image","task":"text-to-image"}]}`))
		} else {
			_, _ = w.Write([]byte(`{"data":[{"id":"chat-vision","task":"text-generation"}]}`))
		}
	}))
	defer upstream.Close()
	llm := config.LLMConfig{Providers: map[string]config.ProviderConfig{"custom": {BaseURL: upstream.URL, APIKey: "key"}}}
	for _, include := range []bool{true, false} {
		includeImages = include
		result := CheckModelProvider(context.Background(), ModelProviderCheckInput{ID: "custom", BaseURL: upstream.URL, APIKey: "key"})
		if result.Status != ModelProviderStatusConnected || (len(result.ImageModels) > 0) != include {
			t.Fatalf("discovery=%+v", result)
		}
		llm, _ = ApplyModelProviderCheckResult(llm, "custom", result)
		if (len(llm.Providers["custom"].ImageModels) > 0) != include {
			t.Fatal("image catalog cache was not updated")
		}
		catalog := ModelProviderCatalogFromLLM(llm)
		for _, provider := range catalog.Providers {
			if provider.ID == "custom" && (len(provider.ImageModels) > 0) != include {
				t.Fatal("API catalog lost image models")
			}
		}
	}
}

func TestCheckCLIProviderRequiresAuthAndRegisteredModels(t *testing.T) {
	for _, provider := range []string{ModelProviderIDCodex, ModelProviderIDClaude} {
		t.Run(provider, func(t *testing.T) {
			for _, tc := range []struct {
				name          string
				authenticated bool
				authError     error
				listError     error
				wantStatus    string
				wantMessage   string
			}{
				{name: "missing auth", wantStatus: ModelProviderStatusFailed, wantMessage: "Sign in first"},
				{name: "auth lookup fails", authError: errors.New("cannot read auth"), wantStatus: ModelProviderStatusFailed, wantMessage: "cannot read auth"},
				{name: "model discovery fails", authenticated: true, listError: errors.New("models unavailable"), wantStatus: ModelProviderStatusFailed, wantMessage: "models unavailable"},
				{name: "authenticated models", authenticated: true, wantStatus: ModelProviderStatusConnected, wantMessage: "connected"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					oldAuth, oldModels, oldChoices := cliProxyAuthStatus, listCLIProxyModels, listCLIProxyModelChoices
					t.Cleanup(func() {
						cliProxyAuthStatus, listCLIProxyModels, listCLIProxyModelChoices = oldAuth, oldModels, oldChoices
					})
					cliProxyAuthStatus = func(_ context.Context, gotProvider string) (cliproxy.AuthStatus, error) {
						if gotProvider != provider {
							t.Fatalf("auth provider = %q, want %q", gotProvider, provider)
						}
						return cliproxy.AuthStatus{Authenticated: tc.authenticated, Message: "Sign in first"}, tc.authError
					}
					listed := false
					listCLIProxyModels = func(_ context.Context, gotProvider string) ([]string, error) {
						listed = true
						if gotProvider != provider {
							t.Fatalf("models provider = %q, want %q", gotProvider, provider)
						}
						return []string{"registered-model"}, tc.listError
					}
					listCLIProxyModelChoices = func(context.Context, string) ([]string, error) {
						t.Fatal("connection check must not use fallback model choices")
						return nil, nil
					}
					models, err := (&ModelConfiguration{}).ListModelsForRequest(context.Background(), ProfileModelRequest{Provider: provider})
					if tc.wantStatus == ModelProviderStatusConnected {
						if err != nil || strings.Join(models, ",") != "registered-model" {
							t.Fatalf("agent picker models = %v, err = %v", models, err)
						}
					} else if err == nil || err.Error() != tc.wantMessage || len(models) != 0 {
						t.Fatalf("agent picker models = %v, err = %v; want no models and %q", models, err, tc.wantMessage)
					}
					got := CheckModelProvider(context.Background(), ModelProviderCheckInput{ID: provider})
					if got.Status != tc.wantStatus || got.Message != tc.wantMessage {
						t.Fatalf("check = %+v, want %s: %s", got, tc.wantStatus, tc.wantMessage)
					}
					if listed != (tc.authenticated && tc.authError == nil) {
						t.Fatalf("models listed = %v", listed)
					}
					if tc.wantStatus == ModelProviderStatusConnected {
						if strings.Join(got.Models, ",") != "registered-model" {
							t.Fatalf("models = %v", got.Models)
						}
					} else if len(got.Models) != 0 {
						t.Fatalf("failed check returned models: %v", got.Models)
					}
				})
			}
		})
	}
}

func TestFailedCLIProviderCheckClearsCachedCatalog(t *testing.T) {
	for _, provider := range []string{ModelProviderIDCodex, ModelProviderIDClaude} {
		t.Run(provider, func(t *testing.T) {
			llm := config.LLMConfig{Providers: map[string]config.ProviderConfig{provider: {
				Status: ModelProviderStatusConnected, Models: []string{"stale-model"}, ImageModels: []string{"stale-image"}, VisionModels: []string{"stale-model"},
			}}}
			got, changed := ApplyModelProviderCheckResult(llm, provider, ModelProviderCheckResult{Status: ModelProviderStatusFailed, Message: "Sign in first"})
			cached := got.Providers[provider]
			if !changed || cached.Status != ModelProviderStatusFailed || len(cached.Models)+len(cached.ImageModels)+len(cached.VisionModels) != 0 {
				t.Fatalf("cached provider after failure = %+v, changed = %v", cached, changed)
			}
			got, changed = ApplyModelProviderCheckResult(got, provider, ModelProviderCheckResult{Status: ModelProviderStatusConnected, Models: []string{"registered-model"}})
			if !changed || strings.Join(got.Providers[provider].Models, ",") != "registered-model" {
				t.Fatalf("catalog did not recover: %+v", got)
			}
		})
	}
}

func TestOpenCSGMultimodalChatCatalog(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer gk_test" {
			t.Errorf("unexpected gateway request: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[
   {"id":"Qwen3.8-27B:xx","task":"image-text-to-text","availability":{"is_available":true}},
   {"id":"omni","tasks":["any-to-any"]},
   {"id":"audio-chat","task":"audio-text-to-text"},
   {"id":"video-chat","task":"video-text-to-text"},
   {"id":"vision-chat","task":"vision"},
   {"id":"combined","tasks":["text-generation,image-text-to-text"]},
   {"id":"offline","task":"image-text-to-text","availability":{"is_available":false}},
   {"id":"caption","task":"image-to-text"},
   {"id":"asr","task":"speech-to-text"},
   {"id":"embedding","task":"feature-extraction"}
  ]}`))
	}))
	defer upstream.Close()
	old := defaultCSGHubCredentials
	t.Cleanup(func() { defaultCSGHubCredentials = old })
	defaultCSGHubCredentials = func(context.Context, *http.Client) (string, string, bool, error) {
		return upstream.URL + "/v1", "gk_test", true, nil
	}
	checked := CheckModelProvider(context.Background(), ModelProviderCheckInput{ID: ModelProviderIDOpenCSG})
	if checked.Status != ModelProviderStatusConnected {
		t.Fatalf("check failed: %+v", checked)
	}
	updated, _ := ApplyModelProviderCheckResult(config.LLMConfig{}, ModelProviderIDOpenCSG, checked)
	catalog := ModelProviderCatalogFromLLM(updated)
	for _, provider := range catalog.Providers {
		if provider.ID != ModelProviderIDOpenCSG {
			continue
		}
		for _, id := range []string{"Qwen3.8-27B:xx", "omni", "audio-chat", "video-chat", "vision-chat", "combined"} {
			if !slices.Contains(provider.Models, id) {
				t.Errorf("chat model missing from UI catalog: %s; models=%v", id, provider.Models)
			}
		}
		for _, id := range []string{"Qwen3.8-27B:xx", "vision-chat", "combined"} {
			if !slices.Contains(provider.VisionModels, id) {
				t.Errorf("vision capability missing: %s", id)
			}
		}
		for _, id := range []string{"offline", "caption", "asr", "embedding"} {
			if slices.Contains(provider.Models, id) {
				t.Errorf("non-chat/unavailable model included: %s", id)
			}
		}
		return
	}
	t.Fatal("OpenCSG provider missing")
}
