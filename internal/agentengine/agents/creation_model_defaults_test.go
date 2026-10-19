package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/config"
)

func TestRecommendedCreationDefaults(t *testing.T) {
	catalog := ModelProviderCatalog{Providers: []ModelProviderSummary{
		{ID: "unknown", Status: "unknown", Models: []string{"stale"}, ImageModels: []string{"stale-image"}},
		{ID: "failed", Status: "failed", Models: []string{"stale"}, VideoModels: []string{"stale-video"}},
		{ID: "other", Status: "connected", Models: []string{"chat", "image", "video"}, ImageModels: []string{"image"}, VideoModels: []string{"video"}},
		{ID: "opencsg", Status: "connected", Models: []string{"official-chat", "auto", "official-image"}, ImageModels: []string{"official-image", "second-image"}, VideoModels: []string{"official-video", "second-video"}},
	}}
	check := func(profile AgentProfile, provider, chat, image, video string) {
		t.Helper()
		if profile.ModelProviderID != provider || profile.ModelID != chat {
			t.Fatalf("chat = %+v", profile)
		}
		if image == "" {
			if profile.ImageGeneration != nil {
				t.Fatal("unexpected image")
			}
		} else if profile.ImageGeneration == nil || profile.ImageGeneration.ModelID != image {
			t.Fatalf("image = %+v", profile.ImageGeneration)
		}
		if video == "" {
			if profile.VideoGeneration != nil {
				t.Fatal("unexpected video")
			}
		} else if profile.VideoGeneration == nil || profile.VideoGeneration.ModelID != video {
			t.Fatalf("video = %+v", profile.VideoGeneration)
		}
	}
	check(resolveCreationDefaults(nil, catalog), "opencsg", "auto", "official-image", "official-video")
	explicit := CreationModelDefaults{Agent: &ModelDefaultRef{ProviderID: "other", ModelID: "chat"}, Image: &ModelDefaultRef{ProviderID: "opencsg", ModelID: "second-image"}, Video: &ModelDefaultRef{}}
	check(resolveCreationDefaults(&explicit, catalog), "other", "chat", "second-image", "")
	catalog.Providers[3].Status = "failed"
	check(resolveCreationDefaults(nil, catalog), "other", "chat", "image", "video")
	check(resolveCreationDefaults(&explicit, catalog), "other", "chat", "image", "")
	catalog.Providers[2].Status = "unknown"
	check(resolveCreationDefaults(nil, catalog), "", "", "", "")
}

func TestCreationDefaultsPersistenceAndValidation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, filename := range []string{"agents.json", "state.json"} {
		t.Run(filename, func(t *testing.T) {
			state := filepath.Join(t.TempDir(), filename)
			llm := config.LLMConfig{Providers: map[string]config.ProviderConfig{
				"opencsg": {Status: "connected", Models: []string{"chat", "auto"}, ImageModels: []string{"image"}, VideoModels: []string{"video"}},
				"failed":  {Status: "failed", Models: []string{"stale"}},
			}}
			service := &ModelConfiguration{Repository: &Repository{state: state}, llm: llm}
			if got := service.CreationDefaultsView(); got.ModelID != "auto" {
				t.Fatalf("initial = %+v", got)
			}
			for _, request := range []CreationModelDefaultRequest{{Kind: "agent", ProviderID: "opencsg", ModelID: "chat"}, {Kind: "image", ProviderID: "opencsg", ModelID: "image"}, {Kind: "video"}} {
				if _, err := service.SetCreationDefault(request); err != nil {
					t.Fatal(err)
				}
			}
			// A later creation can update remembered runtime options without replacing explicit defaults.
			service.profileDefaults = AgentProfile{ModelProviderID: "failed", ModelID: "stale", APIKey: "never-return-this", Env: map[string]string{"SECRET": "never-return-this"}}
			got := service.CreationDefaultsView()
			if got.ModelID != "chat" || got.ImageGeneration == nil || got.VideoGeneration != nil || got.APIKeySet || len(got.Env) > 0 {
				t.Fatalf("defaults = %+v", got)
			}
			reloaded := &Repository{state: state}
			if _, err := reloaded.readState(); err != nil {
				t.Fatal(err)
			}
			if reloaded.creationDefaults == nil || reloaded.creationDefaults.Agent.ModelID != "chat" {
				t.Fatalf("reload = %+v", reloaded.creationDefaults)
			}
			for _, bad := range []CreationModelDefaultRequest{
				{Kind: "agent", ProviderID: "failed", ModelID: "stale"},
				{Kind: "agent", ProviderID: "opencsg", ModelID: "image"},
				{Kind: "image", ProviderID: "opencsg", ModelID: "chat"},
				{Kind: "video", ProviderID: "opencsg", ModelID: "image"},
				{Kind: "invalid"},
			} {
				if _, err := service.SetCreationDefault(bad); err == nil {
					t.Fatalf("accepted invalid defaults: %+v", bad)
				}
			}
			if got := service.CreationDefaultsView(); got.ModelID != "chat" {
				t.Fatal("invalid request changed defaults")
			}
			for _, kind := range []string{"agent", "image", "video"} {
				if _, err := service.SetCreationDefault(CreationModelDefaultRequest{Kind: kind}); err != nil {
					t.Fatal(err)
				}
			}
			raw, err := os.ReadFile(state)
			if err != nil {
				t.Fatal(err)
			}
			var persisted map[string]any
			if err = json.Unmarshal(raw, &persisted); err != nil {
				t.Fatal(err)
			}
			empty := &Repository{state: state}
			if _, err = empty.readState(); err != nil {
				t.Fatal(err)
			}
			if empty.creationDefaults == nil {
				t.Fatal("explicit empty defaults lost on reload")
			}
			service.Repository = empty
			if got := service.CreationDefaultsView(); got.ModelID != "" || got.ImageGeneration != nil || got.VideoGeneration != nil {
				t.Fatalf("explicit empty = %+v", got)
			}
		})
	}
}

func TestCreationDefaultCategoriesRemainIndependent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	llm := config.LLMConfig{Providers: map[string]config.ProviderConfig{
		"other":   {Status: "connected", Models: []string{"chat"}, ImageModels: []string{"image"}, VideoModels: []string{"video"}},
		"opencsg": {Status: "failed", Models: []string{"auto"}, ImageModels: []string{"official-image"}, VideoModels: []string{"official-video"}},
	}}
	service := &ModelConfiguration{Repository: &Repository{}, llm: llm}
	if _, err := service.SetCreationDefault(CreationModelDefaultRequest{Kind: "image", ProviderID: "other", ModelID: "image"}); err != nil {
		t.Fatal(err)
	}
	official := llm.Providers["opencsg"]
	official.Status = "connected"
	llm.Providers["opencsg"] = official
	service.llm = llm
	got := service.CreationDefaultsView()
	if got.ModelID != "auto" || got.ImageGeneration.ModelID != "image" || got.VideoGeneration.ModelID != "official-video" {
		t.Fatalf("unmodified categories stopped following recommendations: %+v", got)
	}
	// A temporary fallback in the video category must not become explicit when
	// saving an unrelated category.
	if _, err := service.SetCreationDefault(CreationModelDefaultRequest{Kind: "video", ProviderID: "opencsg", ModelID: "official-video"}); err != nil {
		t.Fatal(err)
	}
	official.Status = "failed"
	llm.Providers["opencsg"] = official
	service.llm = llm
	if _, err := service.SetCreationDefault(CreationModelDefaultRequest{Kind: "image"}); err != nil {
		t.Fatal(err)
	}
	official.Status = "connected"
	llm.Providers["opencsg"] = official
	service.llm = llm
	got = service.CreationDefaultsView()
	if got.VideoGeneration.ModelID != "official-video" || got.ImageGeneration != nil {
		t.Fatalf("saved preferences lost: %+v", got)
	}
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	service.state = filepath.Join(blocker, "agents.json")
	if _, err := service.SetCreationDefault(CreationModelDefaultRequest{Kind: "video"}); err == nil {
		t.Fatal("expected persistence failure")
	}
	if got := service.CreationDefaultsView(); got.VideoGeneration == nil {
		t.Fatal("failed save changed default")
	}
}
