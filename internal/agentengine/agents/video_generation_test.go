package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"csgclaw/internal/config"
	"csgclaw/internal/modelcap"
	"csgclaw/internal/modelprovider"
)

func TestGenerateVideoPreservesConfirmedOmittedParameters(t *testing.T) {
	submitted := make(chan map[string]any, 1)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/videos" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode video request: %v", err)
		}
		submitted <- body
		http.Error(w, "fixture rejection", http.StatusBadRequest)
	}))
	defer gateway.Close()

	models := &ModelConfiguration{Repository: &Repository{}, llm: config.LLMConfig{Providers: map[string]config.ProviderConfig{
		"fixture": {
			BaseURL: gateway.URL + "/v1", VideoModels: []string{"video"},
			VideoMetadata: map[string]modelcap.VideoGeneration{
				"video": {Sizes: []string{"480P"}, Seconds: []int{4}},
			},
		},
	}}}
	ref := &modelprovider.VideoGenerationConfig{ProviderID: "fixture", ModelID: "video"}
	if _, err := models.GenerateVideo(context.Background(), ref, "animate", modelprovider.VideoGenerationOptions{ParametersConfirmed: true}); err == nil {
		t.Fatal("expected fixture rejection")
	}
	select {
	case body := <-submitted:
		if _, ok := body["size"]; ok {
			t.Fatalf("request added unconfirmed size: %v", body)
		}
		if _, ok := body["seconds"]; ok {
			t.Fatalf("request added unconfirmed duration: %v", body)
		}
	default:
		t.Fatal("video request did not reach the fixture")
	}
}

func TestVideoGenerationCapabilitiesRefreshesPopulatedCache(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"video","task":"text-to-video","metadata":{"capabilities":{"video":{"seconds":[6]}}}}]}`))
	}))
	defer gateway.Close()

	models := &ModelConfiguration{Repository: &Repository{}, llm: config.LLMConfig{Providers: map[string]config.ProviderConfig{
		"fixture": {
			BaseURL: gateway.URL + "/v1", VideoModels: []string{"video"},
			VideoMetadata: map[string]modelcap.VideoGeneration{"video": {Seconds: []int{4}}},
		},
	}}}
	ref := &modelprovider.VideoGenerationConfig{ProviderID: "fixture", ModelID: "video"}
	cached, err := models.VideoGenerationCapabilities(context.Background(), ref, false)
	if err != nil || len(cached.Seconds) != 1 || cached.Seconds[0] != 4 {
		t.Fatalf("cached capabilities: %+v, %v", cached, err)
	}
	fresh, err := models.VideoGenerationCapabilities(context.Background(), ref, true)
	if err != nil || len(fresh.Seconds) != 1 || fresh.Seconds[0] != 6 {
		t.Fatalf("refreshed capabilities: %+v, %v", fresh, err)
	}
}
