package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
)

func TestGenerateTaskTitleUsesStandaloneCompletion(t *testing.T) {
	for _, tt := range []struct {
		name, reply, finishReason, want string
	}{
		{name: "valid", reply: "  排查登录超时  ", want: "排查登录超时"},
		{name: "truncated", reply: "排查登录", finishReason: "length"},
		{name: "filtered", reply: "排查登录", finishReason: "content_filter"},
		{name: "multiline", reply: "登录超时\n排查方案"},
		{name: "empty"},
		{name: "too long", reply: strings.Repeat("长", 65)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Model    string              `json:"model"`
					Stream   bool                `json:"stream"`
					Tools    json.RawMessage     `json:"tools"`
					Messages []map[string]string `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.Model != "selected-model" || request.Stream || len(request.Tools) != 0 || len(request.Messages) != 2 {
					t.Errorf("unexpected model request: %+v", request)
				} else {
					var qa map[string]string
					if err := json.Unmarshal([]byte(request.Messages[1]["content"]), &qa); err != nil ||
						qa["question"] != strings.Repeat("问", 1200) || qa["answer"] != "回答" {
						t.Error("title request did not contain the bounded first exchange")
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{
					map[string]any{"message": map[string]string{"content": tt.reply}, "finish_reason": tt.finishReason},
				}})
			}))
			defer upstream.Close()
			svc := NewService(config.ModelConfig{}, nil)
			title, err := svc.GenerateTaskTitle(context.Background(), "worker", agent.AgentProfile{
				Provider: agent.ProviderAPI, ModelID: "selected-model", BaseURL: upstream.URL,
				RequestOptions: map[string]any{"tools": []string{"unexpected"}},
			}, strings.Repeat("问", 1300), "回答")
			if title != tt.want || (err == nil) != (tt.want != "") {
				t.Fatalf("GenerateTaskTitle() = %q, %v, want %q", title, err, tt.want)
			}
		})
	}
}
