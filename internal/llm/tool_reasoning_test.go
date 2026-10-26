package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
)

func TestResponsesPreservesExplicitReasoningAndTools(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawSearch := false
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["reasoning"].(map[string]any)["effort"] != "minimal" {
			t.Error("explicit effort was silently replaced")
		}
		for _, raw := range payload["tools"].([]any) {
			if raw.(map[string]any)["type"] == "web_search" {
				sawSearch = true
			}
		}
		if !sawSearch {
			t.Error("supported web search was removed")
		}
		io.WriteString(w, `{"object":"response","status":"completed","output":[]}`)
	}))
	defer upstream.Close()
	old := embeddedCLIProxyProviderBaseURL
	embeddedCLIProxyProviderBaseURL = func(context.Context, string) (string, error) { return upstream.URL, nil }
	defer func() { embeddedCLIProxyProviderBaseURL = old }()
	service := NewService(config.ModelConfig{}, nil)
	response, err := service.forwardRemoteResponses(context.Background(), agent.AgentProfile{Provider: agent.ProviderCodex, ModelID: "gpt-6-luna"}, []byte(`{"input":"OK","reasoning":{"effort":"minimal"},"tools":[{"type":"web_search"},{"type":"function","name":"lookup"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("status %d", response.StatusCode)
	}
}
