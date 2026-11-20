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

func TestModelProviderConnectUsesResponsesPresetDefaults(t *testing.T) {
	for _, tc := range []struct {
		preset, firstModel, probeModel string
	}{
		{"kimi", "kimi-k2.6", "kimi-k3.1-pro"},
		{"deepseek", "deepseek-chat", "deepseek-flash"},
		{"aliyun", "qwen-vl-max", "qwen3.8-flash"},
		{"aliyun-token-plan", "kimi-k2.6", "auto"},
	} {
		t.Run(tc.preset, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/models":
					fmt.Fprintf(w, `{"data":[{"id":%q},{"id":%q}]}`, tc.firstModel, tc.probeModel)
				case r.Method == http.MethodPost && r.URL.Path == "/responses":
					var probe struct {
						Model string `json:"model"`
					}
					if err := json.NewDecoder(r.Body).Decode(&probe); err != nil || probe.Model != tc.probeModel {
						t.Errorf("probe model = %q, want %q (decode error: %v)", probe.Model, tc.probeModel, err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					fmt.Fprint(w, `{"status":"completed","error":null,"output":[{"type":"message","content":[{"type":"output_text","text":"OK"}]}]}`)
				default:
					t.Errorf("unexpected upstream request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer upstream.Close()
			path := filepath.Join(t.TempDir(), "config.toml")
			writeMinimalAPIConfig(t, path)
			h := newModelProviderTestHandler(t, path, nil)
			req := fmt.Sprintf(`{"id":"vendor","preset":%q,"base_url":%q,"api_key":"fixture-key","connect":true}`, tc.preset, upstream.URL)
			rec := httptest.NewRecorder()
			h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/model-providers", strings.NewReader(req)))
			if rec.Code != http.StatusCreated {
				t.Fatalf("connect = %d %s", rec.Code, rec.Body)
			}
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			provider := cfg.Models.Providers["vendor"]
			if provider.APIProtocol != modelpreset.Responses || provider.Status != "connected" || strings.Join(provider.Models, ",") != tc.probeModel+","+tc.firstModel {
				t.Fatalf("protocol = %q, status = %q, models = %v", provider.APIProtocol, provider.Status, provider.Models)
			}
		})
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

func TestModelProviderPresetsResolveEveryDocumentedProtocol(t *testing.T) {
	for _, preset := range modelpreset.Catalog() {
		for _, region := range preset.Regions {
			if len(region.ProtocolURLs) > 0 {
				expected := modelpreset.ChatCompletions
				if region.ProtocolURLs[modelpreset.Responses] != "" {
					expected = modelpreset.Responses
				} else if region.ProtocolURLs[modelpreset.ChatCompletions] == "" {
					expected = modelpreset.AnthropicMessages
				}
				req := modelProviderRequest{Preset: preset.ID, Region: region.ID}
				if err := resolveModelProviderRequest(&req); err != nil || req.APIProtocol != expected || req.BaseURL != region.ProtocolURLs[expected] {
					t.Fatalf("%s/%s default = %s %s (%v)", preset.ID, region.ID, req.APIProtocol, req.BaseURL, err)
				}
			}
			for protocol, want := range region.ProtocolURLs {
				t.Run(preset.ID+"/"+region.ID+"/"+protocol, func(t *testing.T) {
					req := modelProviderRequest{Preset: preset.ID, Region: region.ID, APIProtocol: protocol, BaseURL: region.BaseURL}
					if err := resolveModelProviderRequest(&req); err != nil || req.BaseURL != want {
						t.Fatalf("resolved base = %q, want %q, error = %v", req.BaseURL, want, err)
					}
				})
			}
		}
	}
	for _, req := range []modelProviderRequest{
		{Preset: "claude-api", APIProtocol: modelpreset.Responses},
		{Preset: "zhipu", Region: "global", APIProtocol: modelpreset.Responses},
	} {
		if err := resolveModelProviderRequest(&req); err == nil {
			t.Fatalf("accepted unverified protocol for %s/%s", req.Preset, req.Region)
		}
	}
}

func TestGLMCodingPlanDefaultsToRegionalResponses(t *testing.T) {
	for _, tc := range []struct{ region, host string }{
		{"cn", "https://open.bigmodel.cn"},
		{"global", "https://api.z.ai"},
	} {
		t.Run(tc.region, func(t *testing.T) {
			create := modelProviderRequest{Preset: "zhipu-coding", Region: tc.region}
			if err := resolveModelProviderRequest(&create); err != nil ||
				create.APIProtocol != modelpreset.Responses || create.BaseURL != tc.host+"/api/v1" {
				t.Fatalf("new provider = %+v (%v)", create, err)
			}
			chatURL := tc.host + "/api/coding/paas/v4"
			chat := modelProviderRequest{Preset: create.Preset, Region: create.Region, APIProtocol: modelpreset.ChatCompletions, BaseURL: create.BaseURL}
			if err := resolveModelProviderRequest(&chat); err != nil || chat.BaseURL != chatURL {
				t.Fatalf("manual Chat selection = %+v (%v)", chat, err)
			}
			saved := providerConfigFromRequest(config.ProviderConfig{}, chat, false)
			edit := modelProviderRequest{DisplayName: "Renamed Coding Plan"}
			if err := resolveSavedModelProviderRequest(&edit, saved); err != nil {
				t.Fatal(err)
			}
			updated := providerConfigFromRequest(saved, edit, true)
			if updated.APIProtocol != modelpreset.ChatCompletions || updated.BaseURL != chatURL {
				t.Fatalf("renaming changed saved transport to %s %s", updated.APIProtocol, updated.BaseURL)
			}
		})
	}
}

func TestGLMResponseSwitchChangesOfficialAddressAndPreservesCustomGateway(t *testing.T) {
	draft := modelProviderRequest{Preset: "zhipu", Region: "cn", BaseURL: "https://gateway.example.test/v1"}
	if err := resolveModelProviderRequest(&draft); err != nil || draft.APIProtocol != modelpreset.ChatCompletions {
		t.Fatalf("custom gateway default = %q (%v)", draft.APIProtocol, err)
	}
	saved := config.ProviderConfig{Preset: "zhipu", Region: "cn", BaseURL: "https://open.bigmodel.cn/api/paas/v4", APIProtocol: modelpreset.ChatCompletions}
	req := modelProviderRequest{APIProtocol: modelpreset.Responses}
	if err := resolveSavedModelProviderRequest(&req, saved); err != nil || req.BaseURL != "https://open.bigmodel.cn/api/v1" {
		t.Fatalf("resolved request = %+v, error = %v", req, err)
	}
	req = modelProviderRequest{APIProtocol: modelpreset.Responses, BaseURL: "https://gateway.example.test/v1"}
	if err := resolveSavedModelProviderRequest(&req, saved); err != nil || req.BaseURL != "https://gateway.example.test/v1" {
		t.Fatalf("resolved request = %+v, error = %v", req, err)
	}
}
