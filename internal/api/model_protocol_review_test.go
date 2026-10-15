package api

import (
	"bufio"
	"bytes"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"csgclaw/internal/llm"
	"csgclaw/internal/modelpreset"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Exercise the HTTP interface used by Chat Completions clients against a native
// Responses service, including the following request carrying a tool result.
func TestChatCompletionsWithResponsesProvider(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("upstream path/auth = %s %s", r.URL.Path, r.Header.Get("Authorization"))
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if payload["model"] != "model-test" || payload["stream"] != stream || payload["messages"] != nil || payload["input"] == nil {
					t.Errorf("incorrect Responses request: %+v", payload)
				}
				for key, want := range map[string]any{"max_output_tokens": float64(64), "temperature": 0.3, "top_p": 0.8, "parallel_tool_calls": false, "store": false} {
					if payload[key] != want {
						t.Errorf("%s=%v, want %v", key, payload[key], want)
					}
				}
				raw, _ := json.Marshal(payload)
				if !strings.Contains(string(raw), "lookup") || !strings.Contains(string(raw), "keep-context") {
					t.Errorf("tool or history missing: %s", raw)
				}
				if calls == 2 && (!strings.Contains(string(raw), `"type":"function_call_output"`) || !strings.Contains(string(raw), "call-1") || !strings.Contains(string(raw), "tool-result")) {
					t.Errorf("tool result missing: %s", raw)
				}
				var output string
				if calls == 1 {
					output = `[{"id":"fc-1","type":"function_call","call_id":"call-1","name":"lookup","arguments":"{\"query\":\"value\"}"}]`
				} else {
					output = `[{"id":"msg-1","type":"message","role":"assistant","content":[{"type":"output_text","text":"Done."}]}]`
				}
				terminal := fmt.Sprintf(`{"id":"resp-1","object":"response","model":"model-test","status":"completed","output":%s,"usage":{"input_tokens":8,"output_tokens":3,"total_tokens":11}}`, output)
				if !stream {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, terminal)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				emit := func(event string) { fmt.Fprintf(w, "data: %s\n\n", event); w.(http.Flusher).Flush() }
				emit(`{"type":"response.created","response":{"id":"resp-1","model":"model-test","created_at":1}}`)
				if calls == 1 {
					emit(`{"type":"response.output_item.added","output_index":0,"item":{"id":"fc-1","type":"function_call","call_id":"call-1","name":"lookup","arguments":""}}`)
					emit(`{"type":"response.function_call_arguments.delta","item_id":"fc-1","output_index":0,"delta":"{\"query\":"}`)
					emit(`{"type":"response.function_call_arguments.delta","item_id":"fc-1","output_index":0,"delta":"\"value\"}"}`)
					emit(`{"type":"response.function_call_arguments.done","item_id":"fc-1","output_index":0,"arguments":"{\"query\":\"value\"}"}`)
				} else {
					emit(`{"type":"response.output_text.delta","item_id":"msg-1","output_index":0,"content_index":0,"delta":"Done."}`)
				}
				emit(`{"type":"response.completed","response":` + terminal + `}`)
			}))
			defer upstream.Close()
			agentSvc := mustNewSeededService(t, []agent.Agent{{ID: agent.ManagerUserID, Name: agent.ManagerName, Role: agent.RoleManager, AgentProfile: agent.AgentProfile{Provider: agent.ProviderAPI, BaseURL: upstream.URL + "/v1", APIKey: "test-key", ModelID: "model-test", APIProtocol: modelpreset.Responses, ProfileComplete: true}}})
			h := &Handler{svc: agentSvc, llm: llm.NewService(config.ModelConfig{}, agentSvc)}
			bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.handleBotLLMChatCompletions(w, r, agent.ManagerUserID) }))
			defer bridge.Close()
			messages := []map[string]any{{"role": "system", "content": "keep-context"}, {"role": "user", "content": "Look it up."}}
			for turn := 0; turn < 2; turn++ {
				body, _ := json.Marshal(map[string]any{"messages": messages, "stream": stream, "stream_options": map[string]bool{"include_usage": true}, "max_tokens": 128, "max_completion_tokens": 64, "temperature": 0.3, "top_p": 0.8, "parallel_tool_calls": false, "store": false, "tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "lookup", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}}}}})
				resp, err := http.Post(bridge.URL, "application/json", bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				result, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("turn %d: status=%d body=%s", turn, resp.StatusCode, result)
				}
				if stream {
					if strings.Count(string(result), "data: [DONE]") != 1 || !strings.Contains(string(result), `"total_tokens":11`) {
						t.Fatalf("incomplete chat stream: %s", result)
					}
					if turn == 0 {
						var arguments strings.Builder
						for _, line := range strings.Split(string(result), "\n") {
							if !strings.HasPrefix(line, "data: {") {
								continue
							}
							var chunk struct {
								Choices []struct {
									Delta struct {
										ToolCalls []struct {
											Function struct {
												Arguments string `json:"arguments"`
											} `json:"function"`
										} `json:"tool_calls"`
									} `json:"delta"`
								} `json:"choices"`
							}
							if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err != nil {
								t.Fatal(err)
							}
							for _, choice := range chunk.Choices {
								for _, call := range choice.Delta.ToolCalls {
									arguments.WriteString(call.Function.Arguments)
								}
							}
						}
						if arguments.String() != `{"query":"value"}` {
							t.Fatalf("corrupted streamed tool arguments: %s", arguments.String())
						}
					}
					if turn == 0 && (!strings.Contains(string(result), "call-1") || !strings.Contains(string(result), "lookup") || !strings.Contains(string(result), `"finish_reason":"tool_calls"`)) {
						t.Fatalf("tool stream lost: %s", result)
					}
					if turn == 1 && !strings.Contains(string(result), "Done.") {
						t.Fatalf("text stream lost: %s", result)
					}
				} else {
					var completion struct {
						Object  string `json:"object"`
						Choices []struct {
							Message struct {
								Content   string `json:"content"`
								ToolCalls []struct {
									ID       string `json:"id"`
									Function struct {
										Name      string `json:"name"`
										Arguments string `json:"arguments"`
									} `json:"function"`
								} `json:"tool_calls"`
							} `json:"message"`
							FinishReason string `json:"finish_reason"`
						} `json:"choices"`
						Usage struct {
							TotalTokens int `json:"total_tokens"`
						} `json:"usage"`
					}
					if json.Unmarshal(result, &completion) != nil || completion.Object != "chat.completion" || len(completion.Choices) != 1 || completion.Usage.TotalTokens != 11 {
						t.Fatalf("invalid chat response: %s", result)
					}
					if turn == 0 {
						tools := completion.Choices[0].Message.ToolCalls
						if len(tools) != 1 || tools[0].ID != "call-1" || tools[0].Function.Name != "lookup" || tools[0].Function.Arguments != `{"query":"value"}` || completion.Choices[0].FinishReason != "tool_calls" {
							t.Fatalf("lost tool response: %s", result)
						}
					} else if completion.Choices[0].Message.Content != "Done." {
						t.Fatalf("lost text response: %s", result)
					}
				}
				messages = append(messages, map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "call-1", "type": "function", "function": map[string]string{"name": "lookup", "arguments": `{"query":"value"}`}}}}, map[string]any{"role": "tool", "tool_call_id": "call-1", "content": "tool-result"})
			}
			if calls != 2 {
				t.Fatalf("upstream calls=%d", calls)
			}
		})
	}
}

func TestModelProviderRefreshPreservesMediaOnly(t *testing.T) {
	for _, tc := range []struct{ name, model, task string }{{"image", "gpt-image-1", ""}, {"video", "video-test", "text-to-video"}} {
		for _, verify := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/verify=%v", tc.name, verify), func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodGet || r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer media-key" {
						t.Errorf("unexpected media probe: %s %s", r.Method, r.URL.Path)
						http.Error(w, "unexpected", 400)
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": tc.model, "task": tc.task}}})
				}))
				defer upstream.Close()
				path := filepath.Join(t.TempDir(), "config.toml")
				writeMinimalAPIConfig(t, path)
				cfg, err := config.Load(path)
				if err != nil {
					t.Fatal(err)
				}
				provider := config.ProviderConfig{Preset: "custom", BaseURL: upstream.URL + "/v1", APIKey: "media-key", APIProtocol: modelpreset.ChatCompletions, Status: "connected"}
				if tc.name == "image" {
					provider.ImageModels = []string{tc.model}
				} else {
					provider.VideoModels = []string{tc.model}
				}
				cfg.Models.Providers["media"] = provider
				if err = cfg.Save(path); err != nil {
					t.Fatal(err)
				}
				h := newModelProviderTestHandler(t, path, nil)
				rec := httptest.NewRecorder()
				h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/model-providers/media/check", strings.NewReader(fmt.Sprintf(`{"connect":%v}`, verify))))
				if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"connected"`) {
					t.Fatalf("media refresh rejected: %d %s", rec.Code, rec.Body)
				}
				saved, err := config.Load(path)
				if err != nil {
					t.Fatal(err)
				}
				got := saved.Models.Providers["media"]
				if got.Status != "connected" || got.ModelSource != "upstream" || len(got.Models) != 0 || tc.name == "image" && strings.Join(got.ImageModels, ",") != tc.model || tc.name == "video" && strings.Join(got.VideoModels, ",") != tc.model {
					t.Fatalf("lost media connection: %+v", got)
				}
				create, err := json.Marshal(map[string]any{"id": "new-media", "preset": "custom", "base_url": upstream.URL + "/v1", "api_key": "media-key", "api_protocol": modelpreset.ChatCompletions, "connect": true})
				if err != nil {
					t.Fatal(err)
				}
				rec = httptest.NewRecorder()
				h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/model-providers", bytes.NewReader(create)))
				if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"status":"connected"`) || !strings.Contains(rec.Body.String(), tc.model) {
					t.Fatalf("media connection rejected: %d %s", rec.Code, rec.Body)
				}
			})
		}
	}
}

func TestChatResponsesStreamFlushesAndCancelsUpstream(t *testing.T) {
	canceled := make(chan struct{})
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: "+`{"type":"response.created","response":{"id":"partial","model":"model-test"}}`+"\n\n")
		fmt.Fprint(w, "data: "+`{"type":"response.output_text.delta","delta":"partial-reply"}`+"\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			close(canceled)
		case <-release:
		}
	}))
	defer upstream.Close()
	defer close(release)
	agentSvc := mustNewSeededService(t, []agent.Agent{{ID: agent.ManagerUserID, Name: agent.ManagerName, Role: agent.RoleManager, AgentProfile: agent.AgentProfile{Provider: agent.ProviderAPI, BaseURL: upstream.URL, APIKey: "test-key", ModelID: "model-test", APIProtocol: modelpreset.Responses, ProfileComplete: true}}})
	h := &Handler{svc: agentSvc, llm: llm.NewService(config.ModelConfig{}, agentSvc)}
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.handleBotLLMChatCompletions(w, r, agent.ManagerUserID) }))
	defer bridge.Close()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Post(bridge.URL, "application/json", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}],"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	scanner := bufio.NewScanner(resp.Body)
	partial := false
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "partial-reply") {
			partial = true
			break
		}
	}
	if !partial {
		t.Fatalf("reply buffered until completion: %v", scanner.Err())
	}
	resp.Body.Close()
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream was not canceled when client disconnected")
	}
}
