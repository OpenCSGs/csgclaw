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

func TestResponsesConnectionProbeValidatesNullableError(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantError string
	}{
		{name: "null error", body: `{"status":"completed","error":null,"output":[{"type":"message"}]}`},
		{name: "upstream error", body: `{"status":"failed","error":{"message":"failed"},"output":[]}`, wantError: "invalid_response"},
		{name: "wrong protocol", body: `{"error":null,"choices":[{"message":{"content":"OK"}}]}`, wantError: "invalid_response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/models" {
					fmt.Fprint(w, `{"data":[{"id":"deepseek-flash"}]}`)
					return
				}
				fmt.Fprint(w, tc.body)
			}))
			defer upstream.Close()

			_, err := CheckService(context.Background(), ServiceConnection{
				BaseURL: upstream.URL, APIKey: "test-key", Protocol: modelpreset.Responses,
			}, true)
			if tc.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || ConnectionErrorCode(err) != tc.wantError {
				t.Fatalf("error = %v, want %s", err, tc.wantError)
			}
		})
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

func TestArkConnectionUsesEnabledTextModels(t *testing.T) {
	var probed []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"data":[
				{"id":"doubao-vision-embedding","domain":"Embedding"},
				{"id":"seedream-fixture","domain":"ImageGeneration","modalities":{"input_modalities":["text","image"],"output_modalities":["image"]}},
				{"id":"chat-only","api_supported":{"chat/completions":true}},
				{"id":"hitem3d-2-0-251223"},
				{"id":"doubao-seed-2-1-pro-260915","api_supported":{"responses":true}},
				{"id":"working-model","domain":"VLM","api_supported":{"responses":true}}
			]}`)
			return
		}
		var request struct{ Model string }
		if r.URL.Path != "/responses" || json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("expected a Responses inference probe")
		}
		probed = append(probed, request.Model)
		if request.Model == "doubao-seed-2-1-pro-260915" {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":{"code":"ModelNotOpen"}}`)
			return
		}
		fmt.Fprint(w, `{"status":"completed","error":null,"output":[{"type":"message"}]}`)
	}))
	defer upstream.Close()
	conn := ServiceConnection{BaseURL: upstream.URL, APIKey: "fixture-key", Protocol: modelpreset.Responses, Preset: "volcengine"}
	out, err := CheckService(context.Background(), conn, true)
	if err != nil || !reflect.DeepEqual(probed, []string{"doubao-seed-2-1-pro-260915", "working-model"}) {
		t.Fatalf("probed=%v error=%v", probed, err)
	}
	if !reflect.DeepEqual(out.Models, []string{"working-model", "doubao-seed-2-1-pro-260915"}) || !reflect.DeepEqual(out.ImageModels, []string{"seedream-fixture"}) ||
		!out.ImageMetadata["seedream-fixture"].ImageToImage {
		t.Fatalf("directory = %+v", out)
	}

	probed = nil
	conn.Models = []string{"ep-private"}
	out, err = CheckService(context.Background(), conn, true)
	if err != nil || !reflect.DeepEqual(probed, []string{"ep-private"}) || out.Models[0] != "ep-private" {
		t.Fatalf("manual endpoint: models=%v probed=%v error=%v", out.Models, probed, err)
	}
}

func TestMiMoConnectionDoesNotProbeAudioModels(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"data":[{"id":"mimo-v2.5-asr"},{"id":"mimo-v2.5-tts"},{"id":"mimo-v2.6-pro"},{"id":"mimo-v2.6-flash"}]}`)
			return
		}
		var request struct{ Model string }
		if r.URL.Path != "/responses" || json.NewDecoder(r.Body).Decode(&request) != nil || request.Model != "mimo-v2.6-flash" {
			t.Errorf("expected MiMo text probe, got %s %q", r.URL.Path, request.Model)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"status":"completed","error":null,"output":[{"type":"message"}]}`)
	}))
	defer upstream.Close()
	for _, preset := range []string{"mimo", "mimo-plan"} {
		out, err := CheckService(context.Background(), ServiceConnection{BaseURL: upstream.URL, APIKey: "fake", Protocol: modelpreset.Responses, Preset: preset}, true)
		if err != nil || !reflect.DeepEqual(out.Models, []string{"mimo-v2.6-flash", "mimo-v2.6-pro"}) {
			t.Fatalf("%s models = %v (%v)", preset, out.Models, err)
		}
	}
}

func TestProtocolConnectionUsesOfficialDiscoveryAndReportsGLMGatewayErrors(t *testing.T) {
	original := ServiceConnection{BaseURL: "https://open.bigmodel.cn/api/v1", APIKey: "fake", Protocol: modelpreset.Responses, Preset: "zhipu"}
	if models := documentedResponsesModels(original); !reflect.DeepEqual(models, []string{"glm-5.3"}) {
		t.Fatalf("official GLM Responses models = %v", models)
	}
	custom := original
	custom.BaseURL = "https://private.example/v1"
	if models := documentedResponsesModels(custom); len(models) != 0 {
		t.Fatalf("built-in model restrictions applied to a custom gateway: %v", models)
	}
	directory := catalogConnection(original)
	if directory.BaseURL != "https://open.bigmodel.cn/api/paas/v4" || directory.Protocol != modelpreset.ChatCompletions {
		t.Fatalf("discovery = %s %s", directory.BaseURL, directory.Protocol)
	}
	for _, body := range []string{
		`{"code":1001,"msg":"auth required","success":false}`,
		`{"code":401,"msg":"invalid token","success":false}`,
	} {
		if got := ZhipuResponsesGatewayStatus(original.BaseURL, 200, []byte(body)); got != 401 {
			t.Fatalf("logical authentication failure got HTTP %d", got)
		}
	}
	if got := ZhipuResponsesGatewayStatus(original.BaseURL, 200, []byte(`{"status":"completed","error":null,"output":[]}`)); got != 200 {
		t.Fatalf("successful GLM response got HTTP %d", got)
	}
}

func TestInternationalGLMCodingPlanKeepsDiscoveryAndInferenceInRegion(t *testing.T) {
	responses := ServiceConnection{
		BaseURL: "https://api.z.ai/api/v1", APIKey: "fixture-key",
		Protocol: modelpreset.Responses, Preset: "zhipu-coding",
	}
	directory := catalogConnection(responses)
	if directory.BaseURL != "https://api.z.ai/api/coding/paas/v4" || directory.Protocol != modelpreset.ChatCompletions {
		t.Fatalf("discovery = %s %s", directory.BaseURL, directory.Protocol)
	}
	body := []byte(`{"code":1001,"msg":"auth required","success":false}`)
	if got := ZhipuResponsesGatewayStatus(responses.BaseURL, 200, body); got != http.StatusUnauthorized {
		t.Fatalf("gateway authentication failure got HTTP %d", got)
	}
}

func TestResponsesEndpointUnsupportedDistinguishesModelFailures(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{`{"error":{"code":"ModelNotOpen"}}`, false},
		{`{"error":{"code":"InvalidEndpointOrModel.NotFound"}}`, false},
		{`{"error":{"code":"model_not_found"}}`, false},
		{`{"error":{"code":"not_found","message":"unknown endpoint"}}`, true},
		{"404 page not found", true},
	} {
		if got := ResponsesEndpointUnsupported(http.StatusNotFound, []byte(tc.body)); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.body, got, tc.want)
		}
	}
}
