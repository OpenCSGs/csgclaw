package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/cliproxy"
	"csgclaw/internal/config"
)

type sessionProfileFixture struct {
	base           string
	requestOptions map[string]any
}

func (f sessionProfileFixture) ResolvedAgentProfile(string) (agent.AgentProfile, error) {
	return agent.AgentProfile{Provider: agent.ProviderAPI, ModelID: "model-a", ReasoningEffort: "low", BaseURL: f.base, APIKey: "test-key", RequestOptions: f.requestOptions}, nil
}

func (f sessionProfileFixture) ResolvedSessionProfile(id, model, effort string) (agent.AgentProfile, error) {
	profile, _ := f.ResolvedAgentProfile(id)
	if model != "model-b" {
		return profile, fmt.Errorf("model unavailable")
	}
	profile.ModelID, profile.ReasoningEffort = model, effort
	return profile, nil
}

func TestSessionModelBridgeUsesOnlyScopedSelection(t *testing.T) {
	payloads := make(chan map[string]any, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		payloads <- payload
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"response","object":"response","status":"completed","output":[]}`)
	}))
	defer upstream.Close()
	service := NewService(config.ModelConfig{}, sessionProfileFixture{base: upstream.URL})
	for _, test := range []struct {
		ctx   context.Context
		model string
	}{
		{WithSessionModel(context.Background(), "model-b", "high"), "model-b"},
		{context.Background(), "model-a"},
		{context.Background(), "model-a"},
	} {
		response, err := service.Responses(test.ctx, "agent-a", []byte(`{"model":"untrusted-model","input":"hello"}`))
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if payload := <-payloads; payload["model"] != test.model {
			t.Fatalf("upstream model=%v want %s", payload["model"], test.model)
		}
	}
	if _, err := service.Responses(WithSessionModel(context.Background(), "missing", "high"), "agent-a", []byte(`{"input":"hello"}`)); err == nil {
		t.Fatal("invalid selection fell back to the Agent model")
	}
}

func TestSessionModelBridgeUsesResolvedReasoningEffort(t *testing.T) {
	for _, api := range []struct {
		name, body string
		call       func(*Service, context.Context, string, []byte) (*UpstreamResponse, error)
	}{
		{"responses", `{"input":"hello","reasoning":{"effort":"medium","summary":"auto"}}`, (*Service).Responses},
		{"chat", `{"messages":[],"reasoning_effort":"medium"}`, (*Service).ChatCompletionsStream},
	} {
		for _, effort := range []string{"high", "none", "auto", "", "unscoped", "task"} {
			t.Run(api.name+"/"+effort, func(t *testing.T) {
				payloads := make(chan map[string]any, 1)
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var payload map[string]any
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					payloads <- payload
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"id":"response","object":"response","status":"completed","output":[]}`)
				}))
				defer upstream.Close()
				service := NewService(config.ModelConfig{}, sessionProfileFixture{base: upstream.URL})
				ctx := WithSessionModel(t.Context(), "model-b", effort)
				var want any = effort
				switch effort {
				case "auto", "":
					want = nil
				case "unscoped":
					ctx, want = t.Context(), "medium"
				case "task":
					ctx = WithTaskProfile(ctx, agent.AgentProfile{Provider: agent.ProviderAPI, BaseURL: upstream.URL, ModelID: "task-model", ReasoningEffort: "low"})
					want = "medium"
				}
				response, err := api.call(service, ctx, "agent-a", []byte(api.body))
				if err != nil {
					t.Fatal(err)
				}
				io.Copy(io.Discard, response.Body)
				response.Body.Close()
				payload := <-payloads
				got := payload["reasoning_effort"]
				if api.name == "responses" {
					reasoning, _ := payload["reasoning"].(map[string]any)
					got = reasoning["effort"]
					if reasoning["summary"] != "auto" {
						t.Fatalf("reasoning summary changed: %v", reasoning)
					}
				}
				if got != want {
					t.Fatalf("upstream reasoning effort = %v, want %v", got, want)
				}
			})
		}
	}
}

func TestSessionModelResponsesFallbackPreservesReasoningEffort(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, test := range []struct {
			name, effort, mode string
			want               any
		}{
			{name: "auto", effort: "auto"},
			{name: "model_default"},
			{name: "high", effort: "high", want: "high"},
			{name: "none", effort: "none", want: "none"},
			{name: "profile_default", mode: "profile", want: "low"},
			{name: "task_profile", mode: "task", effort: "auto", want: "high"},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", test.name, stream), func(t *testing.T) {
				type modelRequest struct {
					path    string
					payload map[string]any
				}
				requests := make(chan modelRequest, 4)
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var payload map[string]any
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
						http.Error(w, "invalid request", http.StatusBadRequest)
						return
					}
					requests <- modelRequest{r.URL.Path, payload}
					switch r.URL.Path {
					case "/responses":
						http.Error(w, "responses unavailable", http.StatusNotFound)
					case "/chat/completions":
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							io.WriteString(w, "data: {\"id\":\"fixture\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"done\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"fixture\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
							return
						}
						w.Header().Set("Content-Type", "application/json")
						io.WriteString(w, `{"id":"fixture","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`)
					default:
						t.Errorf("unexpected upstream path %q", r.URL.Path)
						http.NotFound(w, r)
					}
				}))
				defer upstream.Close()
				options := map[string]any{"reasoning_effort": "high"}
				service := NewService(config.ModelConfig{}, sessionProfileFixture{base: upstream.URL, requestOptions: options})
				ctx := WithSessionModel(t.Context(), "model-b", test.effort)
				wantModel := "model-b"
				switch test.mode {
				case "profile":
					ctx, wantModel = t.Context(), "model-a"
				case "task":
					wantModel = "task-model"
					ctx = WithTaskProfile(ctx, agent.AgentProfile{Provider: agent.ProviderAPI, BaseURL: upstream.URL, ModelID: wantModel, ReasoningEffort: "auto", RequestOptions: options})
				}
				for attempt := 1; attempt <= 2; attempt++ {
					body := fmt.Appendf(nil, `{"input":"hello","stream":%t,"reasoning":{"effort":"medium"}}`, stream)
					response, err := service.Responses(ctx, "agent-a", body)
					if err != nil {
						t.Fatal(err)
					}
					data, err := io.ReadAll(response.Body)
					response.Body.Close()
					if err != nil || response.StatusCode != http.StatusOK {
						t.Fatalf("attempt %d: status=%d body=%s error=%v", attempt, response.StatusCode, data, err)
					}
					if stream {
						if !strings.Contains(string(data), `"type":"response.completed"`) || !strings.Contains(string(data), `"delta":"done"`) {
							t.Fatalf("attempt %d: incomplete Responses stream: %s", attempt, data)
						}
					} else {
						var result map[string]any
						if err := json.Unmarshal(data, &result); err != nil || result["status"] != "completed" || result["output_text"] != "done" {
							t.Fatalf("attempt %d: invalid Responses result: %s, error=%v", attempt, data, err)
						}
					}
					responses, chats := 0, 0
					for len(requests) > 0 {
						got := <-requests
						if got.path == "/responses" {
							responses++
							continue
						}
						chats++
						if got.payload["model"] != wantModel || got.payload["stream"] != stream || got.payload["reasoning_effort"] != test.want {
							t.Errorf("attempt %d: model=%v stream=%v effort=%v, want %s/%t/%v", attempt, got.payload["model"], got.payload["stream"], got.payload["reasoning_effort"], wantModel, stream, test.want)
						}
					}
					wantResponses := 0
					if attempt == 1 {
						wantResponses = 1
					}
					if responses != wantResponses || chats != 1 {
						t.Fatalf("attempt %d: Responses requests=%d Chat requests=%d, want %d/1", attempt, responses, chats, wantResponses)
					}
				}
				if options["reasoning_effort"] != "high" {
					t.Fatalf("Profile request options changed: %v", options)
				}
			})
		}
	}
}

func TestSessionModelPreservesTaskProvider(t *testing.T) {
	upstream := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"upstream": name, "model": payload["model"], "authorization": r.Header.Get("Authorization")})
		}))
	}
	agentUpstream := upstream("agent")
	defer agentUpstream.Close()
	taskUpstream := upstream("task")
	defer taskUpstream.Close()
	service := NewService(config.ModelConfig{}, sessionProfileFixture{base: agentUpstream.URL})
	taskProfile := agent.AgentProfile{Provider: agent.ProviderAPI, ModelProviderID: "task", BaseURL: taskUpstream.URL, APIKey: "task-key", ModelID: "task-model", ReasoningEffort: "low"}
	for _, test := range []struct {
		name string
		call func(context.Context, string, []byte) (*UpstreamResponse, error)
		body string
	}{
		{"responses", service.Responses, `{"model":"model-b","input":"hello"}`},
		{"chat", service.ChatCompletionsStream, `{"model":"model-b","messages":[]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := WithTaskProfile(context.Background(), taskProfile)
			ctx = WithSessionModel(ctx, "model-b", "high")
			response, err := test.call(ctx, "agent-a", []byte(test.body))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var result map[string]any
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if result["upstream"] != "task" || result["model"] != taskProfile.ModelID || result["authorization"] != "Bearer task-key" {
				t.Fatalf("task provider changed: %v", result)
			}
		})
	}
}

func TestSessionModelBridgePreservesInheritedProfileEffort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	controller := mustSeededAgentService(t, config.LLMConfig{Default: "codex.gpt-5.5", Providers: map[string]config.ProviderConfig{
		agent.ModelProviderIDCodex: {Models: []string{"gpt-5.5"}},
	}}, []agent.Agent{{ID: "agent-inherited", Name: "Inherited", Role: agent.RoleWorker, AgentProfile: agent.AgentProfile{
		Provider: agent.ProviderCodex, ModelProviderID: agent.ModelProviderIDCodex, ModelID: "gpt-5.5", ReasoningEffort: "max",
	}}})
	defer controller.Close()
	models := controller.Models()
	if _, err := models.ResolvedSessionProfile("agent-inherited", "gpt-5.5", "max"); err == nil {
		t.Fatal("explicit unsupported effort was accepted")
	}
	selected, err := models.ResolvedSessionProfile("agent-inherited", "gpt-5.5", "")
	if err != nil || selected.ReasoningEffort != "max" {
		t.Fatalf("inherited selection = %+v, error = %v", selected, err)
	}
	before, err := models.ResolvedAgentProfile("agent-inherited")
	if err != nil {
		t.Fatal(err)
	}
	payloads := make(chan map[string]any, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		payloads <- payload
		w.Header().Set("Content-Type", "text/event-stream")
		if r.URL.Path == "/v1/responses" {
			io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"fixture\",\"status\":\"completed\",\"output\":[]}}\n\n")
		} else {
			io.WriteString(w, "data: {\"id\":\"fixture\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer upstream.Close()
	previousURL, previousAuth := embeddedCLIProxyProviderBaseURL, embeddedCLIProxyAuthStatus
	defer func() {
		embeddedCLIProxyProviderBaseURL, embeddedCLIProxyAuthStatus = previousURL, previousAuth
	}()
	embeddedCLIProxyProviderBaseURL = func(context.Context, string) (string, error) { return upstream.URL + "/v1", nil }
	embeddedCLIProxyAuthStatus = func(context.Context, string) (cliproxy.AuthStatus, error) {
		return cliproxy.AuthStatus{Authenticated: true}, nil
	}
	service := NewService(config.ModelConfig{}, models)
	ctx := WithSessionModel(context.Background(), selected.ModelID, selected.ReasoningEffort)
	for _, test := range []struct {
		name, body string
		call       func(context.Context, string, []byte) (*UpstreamResponse, error)
	}{
		{"responses", `{"model":"client-model","input":"hello","reasoning":{"effort":"max"},"stream":true}`, service.Responses},
		{"chat", `{"model":"client-model","messages":[],"stream":true}`, service.ChatCompletionsStream},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := test.call(ctx, "agent-inherited", []byte(test.body))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if _, err := io.Copy(io.Discard, response.Body); err != nil {
				t.Fatal(err)
			}
			payload := <-payloads
			effort := payload["reasoning_effort"]
			if reasoning, ok := payload["reasoning"].(map[string]any); ok {
				effort = reasoning["effort"]
			}
			if payload["model"] != selected.ModelID || effort != "max" || payload["stream"] != true {
				t.Fatalf("inherited upstream selection = %v", payload)
			}
		})
	}
	after, err := models.ResolvedAgentProfile("agent-inherited")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("bridge changed Profile defaults", err)
	}
}
