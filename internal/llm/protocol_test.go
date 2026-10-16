package llm

import (
	"context"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"csgclaw/internal/modelpreset"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type protocolProfile struct{ p agent.AgentProfile }

func (r protocolProfile) ResolvedAgentProfile(string) (agent.AgentProfile, error) { return r.p, nil }

func TestConfiguredProtocolsPreserveToolRoundTrip(t *testing.T) {
	for _, protocol := range []string{modelpreset.ChatCompletions, modelpreset.AnthropicMessages, modelpreset.Responses} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", protocol, stream), func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					var payload map[string]any
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					if payload["model"] != "model-test" {
						t.Errorf("model=%v", payload["model"])
					}
					if protocol == modelpreset.AnthropicMessages {
						if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "test-key" || r.Header.Get("anthropic-version") == "" {
							t.Errorf("native authentication/path: %s %v", r.URL.Path, r.Header)
						}
					} else if r.Header.Get("Authorization") != "Bearer test-key" {
						t.Error("missing bearer authentication")
					}
					encoded, _ := json.Marshal(payload)
					if calls == 1 && !strings.Contains(string(encoded), "lookup") {
						t.Error("tool definition lost")
					}
					if calls == 2 && (!strings.Contains(string(encoded), "call_1") || !strings.Contains(string(encoded), "tool-result")) {
						t.Errorf("tool result lost: %s", encoded)
					}
					if payload["stream"] != true {
						w.Header().Set("Content-Type", "application/json")
						switch protocol {
						case modelpreset.ChatCompletions:
							fmt.Fprint(w, `{"id":"cmpl-1","object":"chat.completion","model":"model-test","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}}`)
						case modelpreset.AnthropicMessages:
							fmt.Fprint(w, `{"id":"msg-1","type":"message","role":"assistant","model":"model-test","content":[{"type":"tool_use","id":"call_1","name":"lookup","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":8,"output_tokens":3}}`)
						default:
							fmt.Fprint(w, `{"id":"resp-1","object":"response","status":"completed","output":[{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"}]}`)
						}
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					switch protocol {
					case modelpreset.ChatCompletions:
						fmt.Fprint(w, "data: "+`{"id":"cmpl-1","object":"chat.completion.chunk","model":"model-test","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":null}]}`+"\n\n")
						fmt.Fprint(w, "data: "+`{"id":"cmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}}`+"\n\ndata: [DONE]\n\n")
					case modelpreset.AnthropicMessages:
						for _, event := range []string{`{"type":"message_start","message":{"id":"msg-1","type":"message","role":"assistant","model":"model-test","content":[],"usage":{"input_tokens":8,"output_tokens":0}}}`, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"lookup","input":{}}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`, `{"type":"content_block_stop","index":0}`, `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":3}}`, `{"type":"message_stop"}`} {
							fmt.Fprint(w, "data: "+event+"\n\n")
						}
					default:
						fmt.Fprint(w, "event: response.completed\ndata: "+`{"type":"response.completed","response":{"id":"resp-1","status":"completed","output":[{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"}]}}`+"\n\n")
					}
				}))
				defer server.Close()
				p := agent.AgentProfile{Provider: agent.ProviderAPI, ModelID: "model-test", APIKey: "test-key", BaseURL: server.URL + "/v1", APIProtocol: protocol, ProfileComplete: true}
				svc := NewService(config.ModelConfig{}, protocolProfile{p})
				for _, input := range []string{`"hi"`, `[{"role":"user","content":"hi"},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"tool-result"}]`} {
					resp, err := svc.Responses(context.Background(), "agent", []byte(fmt.Sprintf(`{"input":%s,"tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{}}}],"stream":%v}`, input, stream)))
					if err != nil {
						t.Fatal(err)
					}
					body, err := io.ReadAll(resp.Body)
					resp.Body.Close()
					if err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(string(body), "call_1") || !strings.Contains(string(body), "lookup") {
						t.Fatalf("lost tool response: %s", body)
					}
					if stream && !strings.Contains(string(body), "response.completed") {
						t.Fatalf("missing final event: %s", body)
					}
				}
				if calls != 2 {
					t.Fatalf("calls=%d", calls)
				}
			})
		}
	}
}

func TestProtocolStreamCloseCancelsUpstream(t *testing.T) {
	canceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(canceled)
	}))
	defer server.Close()
	svc := NewService(config.ModelConfig{}, protocolProfile{agent.AgentProfile{Provider: agent.ProviderAPI, BaseURL: server.URL + "/v1", APIKey: "key", ModelID: "model", APIProtocol: modelpreset.ChatCompletions}})
	resp, err := svc.Responses(context.Background(), "agent", []byte(`{"input":"hi","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream was not canceled")
	}
}

func TestChatResponsesIncompletePreservesFinishReason(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				terminal := `{"id":"limited","object":"response","model":"model","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[],"usage":{"input_tokens":8,"output_tokens":2,"total_tokens":10}}`
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: "+`{"type":"response.created","response":{"id":"limited","model":"model"}}`+"\n\n")
					fmt.Fprint(w, "data: "+`{"type":"response.incomplete","response":`+terminal+"}\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, terminal)
				}
			}))
			defer upstream.Close()
			svc := NewService(config.ModelConfig{}, protocolProfile{agent.AgentProfile{Provider: agent.ProviderAPI, BaseURL: upstream.URL, APIKey: "key", ModelID: "model", APIProtocol: modelpreset.Responses}})
			body := []byte(fmt.Sprintf(`{"messages":[{"role":"user","content":"hi"}],"stream":%v}`, stream))
			result, status, _, err := svc.ChatCompletions(context.Background(), "agent", body)
			if err != nil || status != 200 || !strings.Contains(string(result), `"finish_reason":"length"`) {
				t.Fatalf("lost incomplete status: status=%d body=%s err=%v", status, result, err)
			}
			if stream && strings.Count(string(result), "data: [DONE]") != 1 {
				t.Fatalf("missing terminal marker: %s", result)
			}
		})
	}
}
