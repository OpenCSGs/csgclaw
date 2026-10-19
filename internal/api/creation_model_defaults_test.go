package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/config"
)

func TestCreationModelDefaultsAPI(t *testing.T) {
	service, _ := mustNewSeededServiceWithPath(t, nil)
	service.Models().SetLLMConfig(config.LLMConfig{Providers: map[string]config.ProviderConfig{
		"opencsg": {Status: "connected", Models: []string{"chat", "auto"}, ImageModels: []string{"image"}, VideoModels: []string{"video"}},
		"other":   {Status: "connected", Models: []string{"other-chat"}},
		"failed":  {Status: "failed", Models: []string{"stale"}},
	}})
	handler := &Handler{svc: service, agentModels: service.Models()}
	request := func(method, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.handleAgentProfileDefaults(rec, httptest.NewRequest(method, "/api/v1/agent-profile-defaults", strings.NewReader(body)))
		return rec
	}
	initial := request(http.MethodGet, "")
	if initial.Code != http.StatusOK || !strings.Contains(initial.Body.String(), `"model_id":"auto"`) {
		t.Fatalf("initial = %d %s", initial.Code, initial.Body)
	}
	for _, body := range []string{
		`{"kind":"agent","provider_id":"failed","model_id":"stale"}`,
		`{"kind":"image","provider_id":"opencsg","model_id":"chat"}`,
		`{"kind":"agent","provider_id":"opencsg","model_id":"auto","api_key":"rejected"}`,
		`{"model_id":`,
	} {
		got := request(http.MethodPatch, body)
		if got.Code != http.StatusBadRequest {
			t.Fatalf("invalid body accepted: %d %s", got.Code, got.Body)
		}
	}
	saved := request(http.MethodPatch, `{"kind":"agent","provider_id":"other","model_id":"other-chat"}`)
	if saved.Code != http.StatusOK {
		t.Fatalf("save = %d %s", saved.Code, saved.Body)
	}
	if got := request(http.MethodPatch, `{"kind":"video"}`); got.Code != http.StatusOK {
		t.Fatalf("clear video = %s", got.Body)
	}
	var defaults map[string]any
	if err := json.Unmarshal(request(http.MethodGet, "").Body.Bytes(), &defaults); err != nil {
		t.Fatal(err)
	}
	if defaults["model_id"] != "other-chat" || defaults["video_generation"] != nil {
		t.Fatalf("saved defaults = %+v", defaults)
	}
	if got := request(http.MethodPatch, `{"kind":"agent"}`); got.Code != http.StatusOK {
		t.Fatalf("clear = %s", got.Body)
	}
	if got := request(http.MethodGet, ""); strings.Contains(got.Body.String(), `"model_id":"auto"`) {
		t.Fatalf("explicit none lost: %s", got.Body)
	}
}
