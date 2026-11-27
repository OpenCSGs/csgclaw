package modelprovider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGenerateImageUsesImagesEndpointAndValidatesContent(t *testing.T) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	for _, valid := range []bool{true, false} {
		t.Run(map[bool]string{true: "valid", false: "invalid content"}[valid], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/images/generations" || r.Header.Get("Authorization") != "Bearer test-secret" {
					t.Errorf("incorrect image routing")
				}
				var request map[string]any
				if json.NewDecoder(r.Body).Decode(&request) != nil || request["model"] != "gpt-image-2" || request["prompt"] != "blue sky" || request["n"] != float64(1) {
					t.Errorf("incorrect image request: %v", request)
				}
				data := pngData.Bytes()
				if !valid {
					data = []byte("this is not an image")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]string{"b64_json": base64.StdEncoding.EncodeToString(data)}}})
			}))
			defer server.Close()
			result, err := GenerateImage(context.Background(), server.Client(), server.URL+"/v1", "test-secret", nil, "gpt-image-2", "blue sky")
			if valid && (err != nil || !bytes.Equal(result.Data, pngData.Bytes()) || result.MediaType != "image/png") {
				t.Fatalf("unexpected result: %v", err)
			}
			if !valid && err == nil {
				t.Fatal("accepted non-image content")
			}
		})
	}
}

func TestImageCapabilityDoesNotConfuseVisionWithGeneration(t *testing.T) {
	for _, id := range []string{"gpt-6.2-astra", "gpt-6.2-sol", "gpt-6-astra", "gpt-5.5", "qwen-vl", "fake-gpt-image-2"} {
		if IsGPTImageModel(id) {
			t.Fatalf("incorrect image capability for %s", id)
		}
	}
	for _, id := range []string{"gpt-image-1.5", "gpt-image-2", "gpt-image-2.5", "gpt-image-2.5-flare", "gpt-image-2.5-sunburst", "gpt-image-2.6", "gpt-image-2.6-flare", "gpt-image-2.6-sunburst"} {
		if !IsGPTImageModel(id) {
			t.Fatalf("missing image capability for %s", id)
		}
	}
}

func TestImageProviderUnavailableStatus(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		_, err := GenerateImage(context.Background(), server.Client(), server.URL, "", nil, "gpt-image-2", "blue sky")
		server.Close()
		var details *ImageGenerationError
		if !errors.As(err, &details) || details.HTTPStatus != status || details.Code != FallbackUpstreamErrorCode(status) {
			t.Fatalf("HTTP %d: %v", status, err)
		}
	}
}

func TestImageErrorRetainsSafetyReasonAndRedactsCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-request-id", "request-123")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
			"code": "moderation_blocked", "message": "Safety system rejected the output. Bearer other-secret api_key=sk-hidden-key configured-api-key custom-header-secret",
			"moderation_details": map[string]string{"moderation_stage": "output"},
		}})
	}))
	defer server.Close()
	_, err := GenerateImage(context.Background(), server.Client(), server.URL, "configured-api-key", map[string]string{"X-Provider-Key": "custom-header-secret"}, "gpt-image-2", "original prompt")
	var details *ImageGenerationError
	if !errors.As(err, &details) {
		t.Fatalf("missing provider diagnostics: %v", err)
	}
	if details.HTTPStatus != 400 || details.Code != "moderation_blocked" || details.Stage != "output" || details.RequestID != "request-123" || !strings.Contains(details.Message, "Safety system rejected the output") {
		t.Fatalf("wrong diagnostics: %+v", details)
	}
	raw, _ := json.Marshal(details)
	for _, secret := range []string{"other-secret", "sk-hidden-key", "configured-api-key", "custom-header-secret"} {
		if strings.Contains(string(raw), secret) || strings.Contains(err.Error(), secret) {
			t.Fatal("provider diagnostics exposed a credential")
		}
	}
}

func TestImageErrorDoesNotExposeUnstructuredResponse(t *testing.T) {
	response := &http.Response{StatusCode: 500, Header: make(http.Header)}
	details := imageResponseError(response, []byte("<html>internal debug info</html>"), "", nil)
	if strings.Contains(details.Message, "debug") || details.Code != "upstream_unavailable" {
		t.Fatalf("unsafe diagnostics: %+v", details)
	}
}

func TestNonGPTImageRequestUsesPortableResponseFormat(t *testing.T) {
	for _, model := range []string{"gemini-2.5-flash-image", "Qwen-Image-2512", "doubao-seedream-4-0-250828", "custom-image-alias"} {
		t.Run(model, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req map[string]any
				_ = json.NewDecoder(r.Body).Decode(&req)
				if req["response_format"] != "b64_json" || req["model"] != model || req["output_format"] != nil {
					t.Errorf("non-portable image request: %v", req)
				}
				w.WriteHeader(http.StatusBadRequest)
			}))
			defer server.Close()
			_, _ = GenerateImage(context.Background(), server.Client(), server.URL, "", nil, model, "neutral test")
		})
	}
}

func TestEmptySuccessfulImageResponseReportsProviderFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer server.Close()
	_, err := GenerateImage(context.Background(), server.Client(), server.URL, "", nil, "gemini-2.5-flash-image", "neutral test")
	var details *ImageGenerationError
	if !errors.As(err, &details) || details.HTTPStatus != 200 || !strings.Contains(details.Message, "empty response") {
		t.Fatalf("missing empty-response diagnostic: %v", err)
	}
}

func TestImageGatewayMislabelledCompressionAndHTTP200Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") != "identity" {
			t.Error("image request allowed automatic gzip decoding")
		}
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write([]byte(`{"request_id":"request-1","code":"InvalidParameter","message":"Field required: input.messages"}`))
	}))
	defer server.Close()
	_, err := GenerateImage(context.Background(), server.Client(), server.URL, "", nil, "gemini-2.5-flash-image", "neutral test")
	var details *ImageGenerationError
	if !errors.As(err, &details) || details.HTTPStatus != 200 || details.Code != "InvalidParameter" || details.Message != "Field required: input.messages" {
		t.Fatalf("lost gateway error: %v", err)
	}
}

func TestGenerateImageDownloadsURLResults(t *testing.T) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, asset string
		wantStatus  int
	}{
		{name: "generation", asset: "/image"},
		{name: "edit", asset: "/image"},
		{name: "download rejected", asset: "/forbidden", wantStatus: http.StatusForbidden},
		{name: "non-image content", asset: "/invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/images/generations", "/images/edits":
					if r.Header.Get("Authorization") != "Bearer secret" {
						t.Error("missing generation credentials")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"url": "http://" + r.Host + tc.asset}}})
				default:
					if r.Method != http.MethodGet || r.Header.Get("Authorization") != "" || r.Header.Get("X-Provider-Key") != "" {
						t.Error("download used generation credentials or method")
					}
					switch r.URL.Path {
					case "/image":
						_, _ = w.Write(pngData.Bytes())
					case "/forbidden":
						w.WriteHeader(http.StatusForbidden)
					case "/invalid":
						_, _ = w.Write([]byte("not an image"))
					}
				}
			}))
			defer server.Close()
			var references []ReferenceImage
			if tc.name == "edit" {
				references = []ReferenceImage{{Name: "source.png", Data: pngData.Bytes()}}
			}
			result, err := GenerateImage(context.Background(), server.Client(), server.URL, "secret", map[string]string{"X-Provider-Key": "private"}, "mock_image_generations", "snow", references...)
			switch {
			case tc.wantStatus != 0:
				var details *ImageGenerationError
				if !errors.As(err, &details) || details.Code != "image_download_failed" || details.HTTPStatus != tc.wantStatus {
					t.Fatalf("unexpected download error: %v", err)
				}
			case tc.asset == "/invalid":
				if err == nil {
					t.Fatal("accepted non-image content")
				}
			default:
				if err != nil || result.MediaType != "image/png" || !bytes.Equal(result.Data, pngData.Bytes()) {
					t.Fatalf("unexpected downloaded result: %v", err)
				}
			}
		})
	}
}

func TestResolveImageGenerationOptionsDefaultsOnlyVerifiedTextRoutes(t *testing.T) {
	for _, tc := range []struct {
		provider string
		model    string
		edit     bool
		options  ImageGenerationOptions
		want     ImageGenerationOptions
	}{
		{provider: "opencsg", model: "Qwen/Qwen-Image-2512-test", want: ImageGenerationOptions{Size: "1024x1024"}},
		{provider: "opencsg", model: "Qwen-Image-2512", want: ImageGenerationOptions{Size: "1536x1024"}},
		{provider: "opencsg", model: "Qwen-Image-2512", edit: true},
		{provider: "custom", model: "Qwen-Image-2512"},
		{provider: "opencsg", model: "Qwen-Image-2512", options: ImageGenerationOptions{Size: "1024x1024"}, want: ImageGenerationOptions{Size: "1024x1024"}},
	} {
		if got := ResolveImageGenerationOptions(tc.provider, tc.model, tc.edit, tc.options); got != tc.want {
			t.Errorf("%+v: got %+v", tc, got)
		}
	}
}

func TestImageServiceUsesDocumentedProviderRequests(t *testing.T) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	ref := []ReferenceImage{{Name: "reference.png", Data: pngData.Bytes()}}
	for _, tc := range []struct {
		name, base, model, path, field string
		edit                           bool
	}{
		{"DashScope text", "https://dashscope.aliyuncs.com/compatible-mode/v1", "z-image-turbo", "/api/v1/services/aigc/multimodal-generation/generation", "input", false},
		{"DashScope Token Plan edit", "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1", "wan2.7-image-pro", "/api/v1/services/aigc/multimodal-generation/generation", "input", true},
		{"Ark edit", "https://ark.cn-beijing.volces.com/api/v3", "doubao-seedream-5-5-261028", "/images/generations", "image", true},
		{"GLM text", "https://open.bigmodel.cn/api/paas/v4", "glm-image", "/images/generations", "prompt", false},
		{"GLM Responses image", "https://open.bigmodel.cn/api/v1", "glm-image", "/api/paas/v4/images/generations", "prompt", false},
		{"GLM Responses image Z.ai", "https://api.z.ai/api/v1", "glm-image", "/api/paas/v4/images/generations", "prompt", false},
		{"Ark Messages image", "https://ark.cn-beijing.volces.com/api/plan/v1", "doubao-seedream-5.5", "/api/plan/v3/images/generations", "image", true},
		{"DashScope Messages image", "https://dashscope.aliyuncs.com/apps/anthropic/v1", "wan2.7-image", "/api/v1/services/aigc/multimodal-generation/generation", "input", true},
		{"MiniMax text", "https://api.minimax.io/v1", "MiniMax-H3-Image", "/images/generations", "prompt", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var images []ReferenceImage
			if tc.edit {
				images = ref
			}
			endpoint, contentType, body, err := buildImageRequest(tc.base, imageAPIForURL(tc.base), tc.model, "blue sky", ImageGenerationOptions{Size: "1024x1024"}, images)
			var request map[string]json.RawMessage
			if err != nil || !strings.HasSuffix(endpoint, tc.path) || contentType != "application/json" ||
				json.Unmarshal(body, &request) != nil || request[tc.field] == nil {
				t.Fatalf("unexpected image request: %s %s %s (%v)", endpoint, contentType, body, err)
			}
			sourceURL, _ := url.Parse(tc.base)
			targetURL, err := url.Parse(endpoint)
			if err != nil || sourceURL.Host != targetURL.Host {
				t.Fatalf("image generation changed provider origin: %s", endpoint)
			}
			if tc.edit && !bytes.Contains(body, []byte("data:image/png;base64,")) {
				t.Fatal("reference image missing from JSON")
			}
			if tc.field == "input" && (strings.Contains(endpoint, "compatible-mode") || !bytes.Contains(body, []byte("1024*1024"))) {
				t.Fatalf("incorrect DashScope request: %s %s", endpoint, body)
			}
			if tc.name == "GLM text" && (request["n"] != nil || request["response_format"] != nil) {
				t.Fatalf("unsupported GLM fields: %s", body)
			}
		})
	}
	images, err := decodeImageResults(imageDashScope, []byte(`{"output":{"choices":[{"message":{"content":[{"text":"generated"},{"image":"https://example.test/image.png"}]}}]}}`))
	if err != nil || len(images) != 1 || images[0].URL != "https://example.test/image.png" {
		t.Fatalf("decoded %v, error = %v", images, err)
	}
}
