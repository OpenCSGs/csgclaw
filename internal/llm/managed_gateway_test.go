package llm

import (
	"csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCSGHubGatewayRefreshesRotatedKey(t *testing.T) {
	var generation atomic.Int32
	var lookups atomic.Int32
	var hubRejectsToken atomic.Bool
	var requests atomic.Int32
	generation.Store(1)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookups.Add(1)
		if r.URL.Path != "/api/v1/namespaces/owner-uuid/apikeys/builtin" ||
			r.URL.Query().Get("current_user") != "owner" || r.Header.Get("Authorization") != "Bearer hub-token" {
			t.Error("key lookup did not use the sandbox's owner and user token")
		}
		if hubRejectsToken.Load() {
			http.Error(w, "token revoked", http.StatusUnauthorized)
			return
		}
		fmt.Fprintf(w, `{"data":{"token":"gk_fixture_%d"}}`, generation.Load())
	}))
	defer hub.Close()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/v1/responses" && r.URL.Path != "/v1/chat/completions" {
			t.Error("unexpected gateway URL")
		}
		if r.Header.Get("Authorization") != fmt.Sprintf("Bearer gk_fixture_%d", generation.Load()) {
			http.Error(w, "key rejected", http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"id":"reply","output":[]}`)
	}))
	defer gateway.Close()
	cfg := CSGHubGatewayConfig{GatewayURL: gateway.URL + "/v1", HubURL: hub.URL,
		UserToken: "hub-token", UserName: "owner", UserUUID: "owner-uuid"}
	service := NewCSGHubService(config.ModelConfig{}, nil, cfg)
	ctx := WithTaskProfile(t.Context(), agents.AgentProfile{Provider: agents.ProviderAPI,
		ModelProviderID: "remote", BaseURL: cfg.GatewayURL, APIKey: "gk_deployment", ModelID: "fixture"})

	responses := func(want int) {
		response, err := service.Responses(ctx, "manager", []byte(`{"input":"hello"}`))
		if err != nil {
			t.Error(err)
			return
		}
		defer response.Body.Close()
		if response.StatusCode != want {
			t.Errorf("responses status = %d, want %d", response.StatusCode, want)
		}
	}
	responses(http.StatusOK)
	if lookups.Load() != 1 || requests.Load() != 1 {
		t.Fatal("first inference did not resolve the current key before forwarding")
	}

	generation.Store(2)
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() { responses(http.StatusOK) })
	}
	group.Wait()
	if lookups.Load() != 2 {
		t.Fatalf("concurrent requests made %d total lookups, want 2", lookups.Load())
	}

	generation.Store(3)
	hubRejectsToken.Store(true)
	responses(http.StatusUnauthorized)
	hubRejectsToken.Store(false)
	chat, err := service.ChatCompletionsStream(ctx, "manager", []byte(`{"messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer chat.Body.Close()
	if chat.StatusCode != http.StatusOK || lookups.Load() != 4 {
		t.Fatalf("chat failed to refresh using the recovered Hub access: status %d, lookups %d", chat.StatusCode, lookups.Load())
	}
	responses(http.StatusOK)
	if lookups.Load() != 4 {
		t.Fatal("next request did not reuse the refreshed key")
	}
}

func TestCSGHubGatewayUsesOnlyTheManagedRemoteProvider(t *testing.T) {
	var calls atomic.Int32
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"data":{"token":"gk_managed"}}`)
	}))
	defer hub.Close()
	static := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer gk_custom" {
			t.Error("custom provider did not use its own credential")
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer static.Close()
	service := NewCSGHubService(config.ModelConfig{}, nil, CSGHubGatewayConfig{
		GatewayURL: static.URL + "/v1", HubURL: hub.URL,
		UserToken: "hub-token", UserName: "owner", UserUUID: "owner-uuid",
	})
	for _, profile := range []agents.AgentProfile{
		{Provider: agents.ProviderAPI, ModelProviderID: "custom", BaseURL: static.URL + "/v1", APIKey: "gk_custom", ModelID: "fixture"},
		{Provider: agents.ProviderAPI, ModelProviderID: "remote", BaseURL: static.URL + "/other/v1", APIKey: "gk_custom", ModelID: "fixture"},
	} {
		ctx := WithTaskProfile(t.Context(), profile)
		response, err := service.Responses(ctx, "manager", []byte(`{"input":"hello"}`))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("responses status = %d, want 401", response.StatusCode)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("static provider triggered a managed credential lookup")
	}
}
