package llm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
)

func TestOpenCSGResponsesRetainsGatewayFormatErrors(t *testing.T) {
	for _, status := range []int{400, 404, 405, 422} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/v1/responses" {
					t.Errorf("request switched protocol to %s", r.URL.Path)
				}
				w.WriteHeader(status)
				io.WriteString(w, `{"error":{"code":"unsupported_feature","message":"unsupported input format","param":"tools[0]"}}`)
			}))
			defer gateway.Close()
			profile := openCSGTestProfile(gateway.URL)
			service := NewService(config.ModelConfig{}, nil)
			service.markResponsesAPIUnsupported(profile, profile.BaseURL)
			response, err := service.Responses(WithTaskProfile(t.Context(), profile), "agent", []byte(`{"input":"test"}`))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if calls.Load() != 1 || response.StatusCode != status {
				t.Fatalf("received status %d after %d calls", response.StatusCode, calls.Load())
			}
		})
	}
}

func TestOpenCSGRetries429WithUnrecognizedCode(t *testing.T) {
	var calls atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"error":{"code":"provider_busy"}}`)
			return
		}
		io.WriteString(w, `{"status":"completed"}`)
	}))
	defer gateway.Close()
	ctx := WithTaskProfile(t.Context(), openCSGTestProfile(gateway.URL))
	response, err := NewService(config.ModelConfig{}, nil).Responses(ctx, "agent", []byte(`{"input":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if calls.Load() != 2 || response.StatusCode != http.StatusOK {
		t.Fatalf("received status %d after %d calls", response.StatusCode, calls.Load())
	}
}

func openCSGTestProfile(url string) agent.AgentProfile {
	return agent.AgentProfile{Provider: agent.ProviderAPI, ModelProviderID: agent.ModelProviderIDOpenCSG, APIProtocol: "responses", BaseURL: url + "/v1", APIKey: "fixture-key", ModelID: "qwen3.8-flash", ProfileComplete: true}
}
