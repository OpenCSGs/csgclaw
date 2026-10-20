package llm

import (
	"context"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"csgclaw/internal/modelpreset"
	"csgclaw/internal/modelusage"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Counters and event placement match live OpenCSG probes on 2026-10-19.
// In particular, Messages starts at zero and updates input in message_delta.
func TestGatewayUsageSurvivesProtocolConversion(t *testing.T) {
	for _, protocol := range []string{modelpreset.ChatCompletions, modelpreset.Responses, modelpreset.AnthropicMessages} {
		for _, consumer := range []string{"chat", "responses"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/%v", protocol, consumer, stream), func(t *testing.T) {
					input, output := int64(14), int64(144)
					if protocol == modelpreset.Responses {
						input, output = 50, 139
					}
					if protocol == modelpreset.AnthropicMessages {
						output = 164
					}
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var p map[string]any
						_ = json.NewDecoder(r.Body).Decode(&p)
						s, _ := p["stream"].(bool)
						if !s {
							w.Header().Set("Content-Type", "application/json")
							switch protocol {
							case modelpreset.ChatCompletions:
								fmt.Fprint(w, `{"id":"c1","model":"routed-model","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":14,"completion_tokens":144,"total_tokens":158,"completion_tokens_details":{"reasoning_tokens":140}}}`)
							case modelpreset.Responses:
								fmt.Fprint(w, `{"id":"r1","object":"response","status":"completed","model":"routed-model","output":[{"id":"m1","type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":50,"output_tokens":139,"total_tokens":189,"output_tokens_details":{"reasoning_tokens":135},"x_details":[{"total_tokens":189}]}}`)
							default:
								t.Error("Messages adapter should request SSE")
							}
							return
						}
						w.Header().Set("Content-Type", "text/event-stream")
						emit := func(v string) { fmt.Fprintf(w, "data: %s\n\n", v) }
						switch protocol {
						case modelpreset.ChatCompletions:
							options, _ := p["stream_options"].(map[string]any)
							if options["include_usage"] != true {
								t.Error("missing include_usage")
							}
							emit(`{"id":"c1","model":"routed-model","choices":[{"index":0,"delta":{"role":"assistant","content":"OK"},"finish_reason":null}]}`)
							emit(`{"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
							emit(`{"id":"c1","choices":[],"usage":{"prompt_tokens":14,"completion_tokens":144,"total_tokens":158,"completion_tokens_details":{"reasoning_tokens":140}}}`)
							emit(`[DONE]`)
						case modelpreset.Responses:
							emit(`{"type":"response.created","response":{"id":"r1","model":"routed-model"}}`)
							emit(`{"type":"response.output_text.delta","delta":"OK","output_index":0,"content_index":0}`)
							emit(`{"type":"response.completed","response":{"id":"r1","object":"response","status":"completed","output":[],"usage":{"input_tokens":50,"output_tokens":139,"total_tokens":189,"output_tokens_details":{"reasoning_tokens":135},"x_details":[{"total_tokens":189}]}}}`)
						case modelpreset.AnthropicMessages:
							for _, event := range []string{`{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","model":"auto","content":[],"usage":{"input_tokens":0,"output_tokens":0}}}`, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"OK"}}`, `{"type":"content_block_stop","index":0}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":14,"output_tokens":164}}`, `{"type":"message_stop"}`} {
								emit(event)
							}
						}
					}))
					defer server.Close()
					svc := NewService(config.ModelConfig{}, protocolProfile{agent.AgentProfile{Provider: agent.ProviderAPI, APIProtocol: protocol, BaseURL: server.URL + "/v1", APIKey: "test", ModelID: "auto"}})
					var response *UpstreamResponse
					var err error
					if consumer == "responses" {
						response, err = svc.Responses(context.Background(), "a", []byte(fmt.Sprintf(`{"model":"auto","input":"Reply OK.","stream":%v}`, stream)))
					} else {
						response, err = svc.ChatCompletionsStream(context.Background(), "a", []byte(fmt.Sprintf(`{"model":"auto","messages":[{"role":"user","content":"Reply OK."}],"stream":%v}`, stream)))
					}
					if err != nil {
						t.Fatal(err)
					}
					defer response.Body.Close()
					raw, err := io.ReadAll(response.Body)
					if err != nil {
						t.Fatal(err)
					}
					var usage *modelusage.Usage
					inspect := func(data string) {
						var v map[string]any
						if json.Unmarshal([]byte(data), &v) != nil {
							return
						}
						if nested, ok := v["response"].(map[string]any); ok {
							v = nested
						}
						if u, ok := v["usage"].(map[string]any); ok {
							usage = modelusage.Parse(u, "auto")
						}
					}
					if stream {
						for _, line := range strings.Split(string(raw), "\n") {
							if strings.HasPrefix(line, "data:") {
								inspect(strings.TrimSpace(line[5:]))
							}
						}
					} else {
						inspect(string(raw))
					}
					if usage == nil || usage.Input != input || usage.Output != output || usage.Total != input+output {
						t.Fatalf("usage=%+v; response=%s", usage, raw)
					}
				})
			}
		}
	}
}

func TestTextFallbackRetainsLateUsage(t *testing.T) {
	var out strings.Builder
	raw := "data: " + `{"id":"c1","choices":[{"index":0,"delta":{"content":"OK"},"finish_reason":"stop"}]}` + "\n\ndata: " + `{"id":"c1","choices":[],"usage":{"prompt_tokens":14,"completion_tokens":144,"total_tokens":158,"completion_tokens_details":{"reasoning_tokens":140}}}` + "\n\ndata: [DONE]\n\n"
	if err := writeChatCompletionStreamAsResponse(&out, strings.NewReader(raw), "auto"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"input_tokens":14`) || !strings.Contains(out.String(), `"output_tokens":144`) || strings.Count(out.String(), `"type":"response.completed"`) != 1 {
		t.Fatal(out.String())
	}
}
