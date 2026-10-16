package modelprovider

import (
	"context"
	"csgclaw/internal/modelpreset"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestServiceDiscoveryAndVerification(t *testing.T) {
	for _, protocol := range []string{modelpreset.Responses, modelpreset.ChatCompletions, modelpreset.AnthropicMessages} {
		for _, discovery := range []int{200, 404, 405, 501} {
			t.Run(fmt.Sprintf("%s/discovery-%d", protocol, discovery), func(t *testing.T) {
				inferences := 0
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if protocol == modelpreset.AnthropicMessages {
						if r.Header.Get("x-api-key") != "test-secret" || r.Header.Get("anthropic-version") == "" {
							t.Error("missing native authentication")
						}
					} else if r.Header.Get("Authorization") != "Bearer test-secret" {
						t.Error("missing Bearer credential")
					}
					if r.URL.Path == "/models" {
						w.WriteHeader(discovery)
						if discovery == 200 {
							fmt.Fprint(w, `{"data":[{"id":"model"}]}`)
						}
						return
					}
					inferences++
					var p map[string]any
					if json.NewDecoder(r.Body).Decode(&p) != nil || p["model"] != "model" {
						t.Error("invalid probe")
					}
					switch protocol {
					case modelpreset.Responses:
						if r.URL.Path != "/responses" {
							t.Error(r.URL.Path)
						}
						fmt.Fprint(w, `{"output":[{"type":"message"}]}`)
					case modelpreset.AnthropicMessages:
						if r.URL.Path != "/messages" {
							t.Error(r.URL.Path)
						}
						fmt.Fprint(w, `{"content":[{"type":"text","text":"OK"}]}`)
					default:
						if r.URL.Path != "/chat/completions" {
							t.Error(r.URL.Path)
						}
						fmt.Fprint(w, `{"choices":[{"message":{"content":"OK"}}]}`)
					}
				}))
				defer upstream.Close()
				got, err := CheckService(context.Background(), ServiceConnection{BaseURL: upstream.URL, APIKey: "test-secret", Protocol: protocol, Models: []string{"model"}}, true)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got.Models, []string{"model"}) || inferences != 1 {
					t.Fatalf("got=%+v inferences=%d", got, inferences)
				}
				expected := "upstream"
				if discovery != 200 {
					expected = "manual"
				}
				if got.Source != expected {
					t.Errorf("source=%s", got.Source)
				}
			})
		}
	}
}

func TestServiceFailuresNeverBecomeConnectedViaPreset(t *testing.T) {
	for _, status := range []int{401, 402, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(status)
				fmt.Fprint(w, "test-secret should never escape")
			}))
			defer server.Close()
			_, err := CheckService(context.Background(), ServiceConnection{BaseURL: server.URL, APIKey: "test-secret", Preset: "zhipu-coding"}, true)
			if err == nil || strings.Contains(err.Error(), "test-secret") || calls != 1 {
				t.Fatalf("error=%v calls=%d", err, calls)
			}
		})
	}
}

func TestServicePresetFallbackRequiresSuccessfulInference(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(404)
		} else {
			w.WriteHeader(403)
		}
	}))
	defer server.Close()
	got, err := CheckService(context.Background(), ServiceConnection{BaseURL: server.URL, APIKey: "key", Preset: "zhipu-coding", Protocol: modelpreset.ChatCompletions}, false)
	if err == nil || got.Source != "preset" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestServiceClaudeModelPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("after_id") == "a" {
			fmt.Fprint(w, `{"data":[{"id":"b"}],"has_more":false}`)
		} else {
			fmt.Fprint(w, `{"data":[{"id":"a"}],"has_more":true}`)
		}
	}))
	defer server.Close()
	got, err := CheckService(context.Background(), ServiceConnection{BaseURL: server.URL, APIKey: "key", Protocol: modelpreset.AnthropicMessages}, false)
	if err != nil || !reflect.DeepEqual(got.Models, []string{"a", "b"}) {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestServiceDiscoveryPreservesMetadataAndSelectsConversationModel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"data":[{"id":"whisper-1"},{"id":"gpt-image-1"},{"id":"gpt-test","context_window":65536}]}`)
			return
		}
		var p map[string]any
		_ = json.NewDecoder(r.Body).Decode(&p)
		if p["model"] != "gpt-test" {
			t.Errorf("probed unsupported model: %v", p["model"])
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"OK"}}]}`)
	}))
	defer upstream.Close()
	got, err := CheckService(context.Background(), ServiceConnection{BaseURL: upstream.URL, APIKey: "test-key", Protocol: modelpreset.ChatCompletions}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Models, []string{"gpt-test"}) || got.ModelMetadata["gpt-test"].ContextWindow != 65536 || !reflect.DeepEqual(got.ImageModels, []string{"gpt-image-1"}) {
		t.Fatalf("directory lost capabilities: %+v", got)
	}
}
