package api

import (
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelMetadataAPIOverrideResetAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeMinimalAPIConfig(t, path)
	handler := newModelProviderTestHandler(t, path, nil)
	call := func(method, url, body string) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		handler.Routes().ServeHTTP(r, httptest.NewRequest(method, url, strings.NewReader(body)))
		return r
	}
	response := call(http.MethodPost, "/api/v1/model-providers", `{"id":"context-test","display_name":"Context test","base_url":"http://fixture/v1","models":["m"],"model_overrides":{"m":{"context_window":8192}}}`)
	if response.Code != 201 {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
	var summary agent.ModelProviderSummary
	if err := json.Unmarshal(response.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.ModelDefaults["m"].ContextWindow != 200000 {
		t.Fatalf("reset preview should use automatic capacity: %+v", summary.ModelDefaults)
	}
	if summary.ModelMetadata["m"].ContextWindow != 8192 || summary.ModelMetadata["m"].ContextSource != "user" {
		t.Fatalf("%+v", summary)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Models.Providers[summary.ID].ModelOverrides["m"].ContextWindow != 8192 {
		t.Fatal("override was not persisted")
	}
	bad := call(http.MethodPut, "/api/v1/model-providers/"+summary.ID, `{"model_overrides":{"m":{"context_window":-1}}}`)
	if bad.Code != 400 {
		t.Fatalf("bad override accepted: %d", bad.Code)
	}
	reset := call(http.MethodPut, "/api/v1/model-providers/"+summary.ID, `{"model_overrides":{}}`)
	if reset.Code != 200 {
		t.Fatalf("reset %d %s", reset.Code, reset.Body.String())
	}
	if err := json.Unmarshal(reset.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.ModelMetadata["m"].ContextWindow != 200000 || summary.ModelMetadata["m"].ContextSource != "default" {
		t.Fatalf("reset failed %+v", summary.ModelMetadata)
	}
}

func TestImageModelsDoNotAdvertiseChatContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeMinimalAPIConfig(t, path)
	handler := newModelProviderTestHandler(t, path, nil)
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/model-providers", strings.NewReader(`{"id":"image-context-test","display_name":"Image context test","base_url":"http://fixture/v1","models":["Qwen/Qwen-Image-2512","gpt-image-2.5","gemini-2.5-flash-image","qwen3.8-max"]}`)))
	if response.Code != 201 {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
	var summary agent.ModelProviderSummary
	if err := json.Unmarshal(response.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.ImageModels) != 3 {
		t.Fatalf("image models: %v", summary.ImageModels)
	}
	if len(summary.ModelMetadata) != 1 || len(summary.ModelDefaults) != 1 || summary.ModelMetadata["qwen3.8-max"].ContextWindow != 1000000 {
		t.Fatalf("unexpected chat context: %+v", summary)
	}
}
