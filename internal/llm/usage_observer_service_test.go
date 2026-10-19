package llm

import (
	"context"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"csgclaw/internal/modelpreset"
	"csgclaw/internal/modelusage"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type usageObserverProfile struct{ profile agent.AgentProfile }

func (p usageObserverProfile) ResolvedAgentProfile(string) (agent.AgentProfile, error) {
	return p.profile, nil
}

func TestServiceUsageWithMissingStreamContentType(t *testing.T) {
	for _, tc := range []struct {
		name     string
		protocol string
		body     string
	}{
		{"responses", modelpreset.Responses, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r1\",\"usage\":{\"input_tokens\":100,\"output_tokens\":10}}}\n\n"},
		{"chat", modelpreset.ChatCompletions, "data: {\"id\":\"r1\",\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":10}}\n\ndata: [DONE]\n\n"},
		{"messages", modelpreset.AnthropicMessages, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"r1\",\"usage\":{\"input_tokens\":100,\"output_tokens\":0}}}\n\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":10}}\n\ndata: {\"type\":\"message_stop\"}\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header()["Content-Type"] = nil // Deliberately suppress net/http content sniffing.
				_, _ = io.WriteString(w, tc.body)
			}))
			defer upstream.Close()
			svc := NewService(config.ModelConfig{}, usageObserverProfile{agent.AgentProfile{Provider: agent.ProviderAPI, ModelID: "fixture", BaseURL: upstream.URL + "/v1", APIKey: "fixture", APIProtocol: tc.protocol}})
			var got *modelusage.Usage
			count := 0
			ctx := modelusage.WithRequestObserver(context.Background(), func(u *modelusage.Usage) { count++; got = u })
			resp, err := svc.Responses(ctx, "alice", []byte(`{"model":"fixture","input":"hi","stream":true}`))
			if err != nil {
				t.Fatal(err)
			}
			_, err = io.Copy(io.Discard, resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if count != 1 || got == nil || got.Total != 110 {
				t.Fatalf("reports=%d usage=%+v", count, got)
			}
		})
	}
}
