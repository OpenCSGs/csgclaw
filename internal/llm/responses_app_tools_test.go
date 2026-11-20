package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/cliproxy"
	"csgclaw/internal/config"
)

func appToolsResponseService(t *testing.T, baseURL string) *Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	records := mustSeededAgentService(t, config.LLMConfig{}, []agent.Agent{{ID: agent.ManagerUserID, Name: agent.ManagerName, Role: agent.RoleManager, AgentProfile: agent.AgentProfile{Name: agent.ManagerName, Provider: agent.ProviderAPI, BaseURL: baseURL, APIKey: "fixture-key", ModelID: "fixture-model", ProfileComplete: true}}})
	return NewService(config.ModelConfig{}, records)
}

func TestResponsesRejectsToolDefinitionsBeforeChatFallback(t *testing.T) {
	for _, tc := range []struct{ name, payload string }{
		{"function", `{"input":"hello","tools":[{"type":"function","name":"inspect","parameters":{"type":"object"}}]}`},
		{"namespace", `{"input":"hello","tools":[{"type":"namespace","name":"csgclaw","tools":[{"type":"function","name":"inspect","parameters":{"type":"object"},"defer_loading":true}]}]}`},
		{"tool search", `{"input":"hello","tools":[{"type":"tool_search"}]}`},
		{"explicit none still preserves definitions", `{"input":"hello","tool_choice":"none","tools":[{"type":"function","name":"inspect","parameters":{"type":"object"}}]}`},
		{"search call input", `{"input":[{"type":"tool_search_call","call_id":"search-1","arguments":{"query":"GitLab"}}]}`},
		{"search output input", `{"input":[{"type":"tool_search_output","call_id":"search-1","tools":[{"type":"namespace","name":"csgclaw","tools":[{"type":"function","name":"inspect","defer_loading":true}]}]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var chats, responses atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/responses" {
					responses.Add(1)
					http.Error(w, "unsupported", http.StatusNotFound)
					return
				}
				chats.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"chat-fixture","choices":[{"message":{"role":"assistant","content":"text-only answer"},"finish_reason":"stop"}]}`)
			}))
			defer upstream.Close()
			svc := appToolsResponseService(t, upstream.URL+"/v1")
			for attempt := 0; attempt < 2; attempt++ {
				response, err := svc.Responses(context.Background(), agent.ManagerUserID, []byte(tc.payload))
				if response != nil {
					response.Body.Close()
				}
				if err == nil {
					t.Fatal("tool definitions were silently dropped through Chat fallback")
				}
				apiErr, ok := err.(*HTTPError)
				if !ok || apiErr.Status != http.StatusBadRequest || !strings.Contains(apiErr.Message, "Responses") {
					t.Fatalf("missing actionable unsupported response: %v", err)
				}
			}
			if chats.Load() != 0 || responses.Load() != 1 {
				t.Fatalf("upstream calls = responses %d, chat %d; expected cached rejection with no Chat request", responses.Load(), chats.Load())
			}
		})
	}
}

func TestNativeResponsesPreservesAppSearchDefinitionsHistoryAndSSE(t *testing.T) {
	requestBody := `{"model":"client-model","stream":true,"tools":[{"type":"tool_search"},{"type":"namespace","name":"csgclaw","description":"Agent Apps","tools":[{"type":"function","name":"gitlab_search","description":"Search projects","parameters":{"type":"object","properties":{"query":{"type":"string"}}},"defer_loading":true}]}],"input":[{"type":"tool_search_call","id":"search-call","call_id":"search-1","arguments":{"query":"GitLab projects"}},{"type":"tool_search_output","call_id":"search-1","tools":[{"type":"namespace","name":"csgclaw","tools":[{"type":"function","name":"gitlab_search","parameters":{"type":"object"},"defer_loading":true}]}]},{"role":"user","content":"Find my projects"}]}`
	searchCall := `{"type":"tool_search_call","id":"search-next","call_id":"search-2","arguments":{"query":"GitLab issues"}}`
	searchOutput := `{"type":"tool_search_output","call_id":"search-2","tools":[{"type":"namespace","name":"csgclaw","tools":[{"type":"function","name":"gitlab_issues","parameters":{"type":"object"},"defer_loading":true}]}]}`
	events := "event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"item\":" + searchCall + "}\n\n" +
		"event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"item\":" + searchOutput + "}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"response-1\",\"status\":\"completed\",\"output\":[" + searchCall + "," + searchOutput + "]}}\n\n"
	var got map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("unexpected fallback request %s", r.URL.Path)
			http.Error(w, "unexpected", 500)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, events)
	}))
	defer upstream.Close()
	svc := appToolsResponseService(t, upstream.URL+"/v1")
	response, err := svc.Responses(context.Background(), agent.ManagerUserID, []byte(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	actual, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != events {
		t.Fatalf("native tool-search SSE changed:\n%s", actual)
	}
	var expected map[string]any
	if err := json.Unmarshal([]byte(requestBody), &expected); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"tools", "input", "stream"} {
		if !reflect.DeepEqual(got[field], expected[field]) {
			t.Fatalf("native Responses %s was changed: %#v", field, got[field])
		}
	}
	if got["model"] != "fixture-model" {
		t.Fatalf("model routing was lost: %v", got["model"])
	}
}

func TestResponsesModelNotFoundDoesNotTriggerFallback(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("unexpected fallback request %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			w.Header().Set("X-Request-ID", "fixture-gateway-request")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"code":"model_not_found","message":"temporary upstream routing failure"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"response-ok","status":"completed","output":[{"type":"function_call","call_id":"call-1","name":"inspect","arguments":"{}"}]}`)
	}))
	defer upstream.Close()
	svc := appToolsResponseService(t, upstream.URL+"/v1")
	request := []byte(`{"input":"hello","tools":[{"type":"function","name":"inspect","parameters":{"type":"object"}}]}`)
	for _, tc := range []struct {
		status int
		body   string
	}{
		{http.StatusNotFound, `"code":"model_not_found"`},
		{http.StatusOK, `"type":"function_call"`},
	} {
		response, err := svc.Responses(t.Context(), agent.ManagerUserID, request)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil || response.StatusCode != tc.status || !strings.Contains(string(body), tc.body) {
			t.Fatalf("expected %d with %s, got %d %s (%v)", tc.status, tc.body, response.StatusCode, body, readErr)
		}
		if tc.status == http.StatusNotFound && response.Header.Get("X-Request-ID") != "fixture-gateway-request" {
			t.Fatal("missing upstream request ID on model error")
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("/responses called %d times, want 2", calls.Load())
	}
}

func TestResponsesUnsupportedCacheExpiresBeforeToolRequest(t *testing.T) {
	var responses atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("unexpected fallback request %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if responses.Add(1) == 1 {
			http.Error(w, "unsupported", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"call_id\":\"call-1\",\"name\":\"inspect\",\"arguments\":\"{}\"}}\n\n")
	}))
	defer upstream.Close()
	baseURL := upstream.URL + "/v1"
	svc := appToolsResponseService(t, baseURL)
	request := []byte(`{"input":"hello","stream":true,"tools":[{"type":"function","name":"inspect","parameters":{"type":"object"}}]}`)
	profile, err := svc.resolveProfile(t.Context(), agent.ManagerUserID)
	if err != nil {
		t.Fatal(err)
	}
	key := responsesCapabilityKey(profile, baseURL)
	for range 2 {
		response, err := svc.Responses(t.Context(), agent.ManagerUserID, request)
		if response != nil {
			response.Body.Close()
		}
		if err == nil || !strings.Contains(err.Error(), "native Responses tool support") {
			t.Fatalf("expected protected fallback, got %v", err)
		}
	}
	if responses.Load() != 1 || !time.Now().Before(svc.responsesUnsupported[key]) {
		t.Fatalf("expected one upstream probe and a live cache entry, got %d probes", responses.Load())
	}
	svc.responsesUnsupported[key] = time.Now().Add(-time.Second)
	response, err := svc.Responses(t.Context(), agent.ManagerUserID, request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(body), `"type":"function_call"`) {
		t.Fatalf("expected recovered tool stream, got %d %s (%v)", response.StatusCode, body, err)
	}
	if responses.Load() != 2 || len(svc.responsesUnsupported) != 0 {
		t.Fatalf("expected one recovery probe and no cached failure, got %d calls and %d entries", responses.Load(), len(svc.responsesUnsupported))
	}
}

func TestCodexResponsesNotFoundDoesNotDowngradeOrPoisonCache(t *testing.T) {
	oldTarget, oldAuth := embeddedCLIProxyProviderBaseURL, embeddedCLIProxyAuthStatus
	t.Cleanup(func() { embeddedCLIProxyProviderBaseURL = oldTarget; embeddedCLIProxyAuthStatus = oldAuth })
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("wrong protocol: %s", r.URL.Path)
			http.Error(w, "wrong route", 500)
			return
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			w.WriteHeader(404)
			io.WriteString(w, `{"error":{"code":"model_not_found","message":"temporary upstream routing failure"}}`)
			return
		}
		io.WriteString(w, `{"id":"response-ok","status":"completed","output":[]}`)
	}))
	defer upstream.Close()
	embeddedCLIProxyProviderBaseURL = func(context.Context, string) (string, error) { return upstream.URL + "/v1", nil }
	embeddedCLIProxyAuthStatus = func(context.Context, string) (cliproxy.AuthStatus, error) {
		return cliproxy.AuthStatus{Authenticated: true}, nil
	}
	service := NewService(config.ModelConfig{}, nil)
	profile := agent.AgentProfile{Provider: agent.ProviderCodex, ModelID: "gpt-5.6-luna"}
	body := []byte(`{"input":[{"type":"function_call_output","call_id":"tool-1","output":"preserved history"}],"tools":[{"type":"function","name":"inspect","parameters":{"type":"object"}}]}`)
	for _, status := range []int{404, 200} {
		resp, err := service.forwardRemoteResponses(context.Background(), profile, body)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != status {
			t.Fatalf("status=%d want=%d", resp.StatusCode, status)
		}
	}
	if calls != 2 {
		t.Fatalf("cached false capability: %d calls", calls)
	}
}

func TestMiMoResponsesKeepsAgentToolsWithoutHostedWebSearch(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want int
	}{
		{"https://api.xiaomimimo.com/v1", 2},
		{"https://token-plan-sgp.xiaomimimo.com/v1", 2},
		{"https://proxy.example.test/v1", 3},
	} {
		payload := map[string]any{"tools": []any{
			map[string]any{"type": "web_search"},
			map[string]any{"type": "custom", "name": "exec"},
			map[string]any{"type": "function", "name": "clock"},
		}}
		applyResponsesProviderPayloadConstraints(payload, agent.AgentProfile{BaseURL: tc.url, ModelID: "mimo-v2.6-pro"})
		if tools := payload["tools"].([]any); len(tools) != tc.want {
			t.Errorf("%s: forwarded tools = %v", tc.url, tools)
		}
	}
}

func TestNativeGLMResponsesReturnsLogicalAuthenticationError(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"code":401,"msg":"invalid token","success":false}`)),
	}
	service := &Service{}
	got, err := service.handleRemoteResponsesResult(context.Background(), agent.AgentProfile{Provider: agent.ProviderAPI},
		nil, "https://open.bigmodel.cn/api/v1", "", resp, false, nil)
	if err != nil || got == nil || got.StatusCode != http.StatusUnauthorized {
		t.Fatalf("logical authentication error returned %+v (%v)", got, err)
	}
	got.Body.Close()
}
