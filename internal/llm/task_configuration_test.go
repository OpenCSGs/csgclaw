package llm

import (
	"context"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type taskProfileFixture struct{}

func (taskProfileFixture) ResolvedAgentProfile(string) (agent.AgentProfile, error) {
	return agent.AgentProfile{ModelID: "agent-default"}, nil
}

func TestConcurrentTaskProfilesReachTheirOwnUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]any{"model": body["model"], "effort": body["reasoning_effort"], "authorization": r.Header.Get("Authorization")})
	}))
	defer upstream.Close()
	svc := NewService(config.ModelConfig{}, taskProfileFixture{})
	var wg sync.WaitGroup
	for _, model := range []string{"model-one", "model-two"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := WithTaskProfile(context.Background(), agent.AgentProfile{Provider: agent.ProviderAPI, BaseURL: upstream.URL, APIKey: model, ModelID: model, ReasoningEffort: "high"})
			response, err := svc.ChatCompletionsStream(ctx, "same-agent", []byte(`{"model":"agent-default","messages":[]}`))
			if err != nil {
				t.Error(err)
				return
			}
			defer response.Body.Close()
			data, _ := io.ReadAll(response.Body)
			var actual map[string]any
			_ = json.Unmarshal(data, &actual)
			if actual["model"] != model || actual["effort"] != "high" || actual["authorization"] != "Bearer "+model {
				t.Errorf("wrong task profile: %s", data)
			}
		}()
	}
	wg.Wait()
	profile, err := svc.resolveProfile(context.Background(), "same-agent")
	if err != nil || profile.ModelID != "agent-default" {
		t.Fatal("changed agent defaults")
	}
}
