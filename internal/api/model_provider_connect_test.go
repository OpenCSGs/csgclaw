package api

import (
	"csgclaw/internal/config"
	"csgclaw/internal/modelpreset"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelProviderConnectIsAtomicAndPersistsProtocol(t *testing.T) {
	valid := false
	probes := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !valid {
			w.WriteHeader(401)
			fmt.Fprint(w, "never-return-this-key")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/models" {
			fmt.Fprint(w, `{"data":[{"id":"model"}]}`)
			return
		}
		probes++
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"OK"}}]}`)
	}))
	defer upstream.Close()
	path := filepath.Join(t.TempDir(), "config.toml")
	writeMinimalAPIConfig(t, path)
	handler := newModelProviderTestHandler(t, path, nil)
	request := fmt.Sprintf(`{"preset":"custom","api_protocol":"chat_completions","base_url":%q,"api_key":"never-return-this-key","connect":true}`, upstream.URL+"/v1")
	post := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/model-providers", strings.NewReader(request)))
		return rec
	}
	failed := post()
	if failed.Code != 400 || strings.Contains(failed.Body.String(), "never-return-this-key") {
		t.Fatalf("failed=%d %s", failed.Code, failed.Body)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Models.Providers) != 1 || cfg.Models.Providers["default"].APIKey != "sk" {
		t.Fatalf("failed connect persisted providers: %+v", cfg.Models.Providers)
	}
	valid = true
	for n := 0; n < 2; n++ {
		rec := post()
		if rec.Code != 201 {
			t.Fatalf("connect=%d %s", rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), "never-return-this-key") {
			t.Fatal("secret leaked")
		}
	}
	cfg, err = config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for id, p := range cfg.Models.Providers {
		if id == "default" {
			continue
		}
		names[p.DisplayName] = true
		if p.APIProtocol != modelpreset.ChatCompletions || p.Region != "custom" || p.Status != "connected" || p.ModelSource != "upstream" {
			t.Fatalf("connection state not persisted: %+v", p)
		}
	}
	if !names["Custom API"] || !names["Custom API 2"] || probes != 2 {
		t.Fatalf("names=%v probes=%d", names, probes)
	}
}

func TestModelProviderPresetsAndInvalidRequests(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeMinimalAPIConfig(t, path)
	h := newModelProviderTestHandler(t, path, nil)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/model-provider-presets", nil))
	var data struct {
		Presets []modelpreset.Preset `json:"presets"`
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &data) != nil || len(data.Presets) < 16 {
		t.Fatalf("directory=%d %s", rec.Code, rec.Body)
	}
	for _, body := range []string{`{"preset":"mimo-plan","region":"invalid"}`, `{"preset":"unknown"}`, `{"api_protocol":"invalid"}`} {
		rec = httptest.NewRecorder()
		h.Routes().ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/model-providers", strings.NewReader(body)))
		if rec.Code != 400 {
			t.Fatalf("invalid request=%d %s", rec.Code, rec.Body)
		}
	}
}
