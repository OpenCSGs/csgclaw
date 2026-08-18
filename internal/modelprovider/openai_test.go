package modelprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestUpstreamErrorCodeForPaymentRequiredResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "explicit code", body: `{"error":{"code":"insufficient_balance"}}`, want: "insufficient_balance"},
		{name: "message identifies balance", body: `{"error":{"code":null,"message":"Insufficient balance"}}`, want: "insufficient_balance"},
		{name: "generic payment", body: `{"error":{"code":null,"message":"Subscription renewal required"}}`, want: "payment_required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UpstreamErrorCodeForResponse(http.StatusPaymentRequired, []byte(tt.body)); got != tt.want {
				t.Fatalf("code = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListOpenAIModelsWithClientDoesNotAddPageSizeForOpenCSG(t *testing.T) {
	var gotURL string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotURL = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"Qwen/Qwen3"}]}`)),
			Request:    req,
		}, nil
	})}

	models, err := ListOpenAIModelsWithClient(context.Background(), client, "https://aigateway.opencsg.com/v1", "sk-test", nil)
	if err != nil {
		t.Fatalf("ListOpenAIModelsWithClient() error = %v", err)
	}
	if got, want := strings.Join(models, ","), "Qwen/Qwen3"; got != want {
		t.Fatalf("models = %v, want %s", models, want)
	}
	if got, want := gotURL, "https://aigateway.opencsg.com/v1/models"; got != want {
		t.Fatalf("request URL = %q, want %q", got, want)
	}
}

func TestListOpenAIModelsWithClientDoesNotAddPageSizeForOtherHosts(t *testing.T) {
	var gotURL string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotURL = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"gpt-test"}]}`)),
			Request:    req,
		}, nil
	})}

	models, err := ListOpenAIModelsWithClient(context.Background(), client, "https://api.example.com/v1", "sk-test", nil)
	if err != nil {
		t.Fatalf("ListOpenAIModelsWithClient() error = %v", err)
	}
	if got, want := strings.Join(models, ","), "gpt-test"; got != want {
		t.Fatalf("models = %v, want %s", models, want)
	}
	if got, want := gotURL, "https://api.example.com/v1/models"; got != want {
		t.Fatalf("request URL = %q, want %q", got, want)
	}
}

func TestListOpenAIModelsWithClientFiltersByTextGenerationTask(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{"data":[
				{"id":"deepseek-v4-pro","task":"text-generation","availability":{"is_available":true}},
				{"id":"qwen3-vl","task":"text-generation,image-text-to-text"},
				{"id":"image-capable-chat","task":"text-generation,image-text-to-text"},
				{"id":"claude-sonnet-4-6","task":"text-generation,image-text-to-text","availability":{"is_available":false}},
				{"id":"doubao-seedream-4-0-250828","task":"text-to-image"},
				{"id":"qwen3-asr","task":"speech-to-text"},
				{"id":"Qwen_Qwen3-Embedding-0.6B","task":""},
				{"id":"legacy-model"}
			]}`)),
			Request: req,
		}, nil
	})}

	models, err := ListOpenAIModelsWithClient(context.Background(), client, "https://aigateway.example/v1", "sk-test", nil)
	if err != nil {
		t.Fatalf("ListOpenAIModelsWithClient() error = %v", err)
	}
	want := "deepseek-v4-pro,qwen3-vl,image-capable-chat,Qwen_Qwen3-Embedding-0.6B,legacy-model"
	if got := strings.Join(models, ","); got != want {
		t.Fatalf("models = %q, want %q", got, want)
	}
}

func TestCheckResponsesAPIWithClientPostsMinimalResponsesRequest(t *testing.T) {
	var gotAuth string
	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %q, want /v1/responses", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp-test","object":"response","status":"completed"}`))
	}))
	defer srv.Close()

	err := CheckResponsesAPIWithClient(context.Background(), srv.Client(), srv.URL+"/v1", "sk-test", "gpt-test", map[string]string{
		"X-Test":        "ok",
		"Authorization": "Bearer ignored",
	})
	if err != nil {
		t.Fatalf("CheckResponsesAPIWithClient() error = %v", err)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("Authorization = %q, want Bearer sk-test", gotAuth)
	}
	if gotPayload["model"] != "gpt-test" {
		t.Fatalf("model = %#v, want gpt-test", gotPayload["model"])
	}
	if gotPayload["input"] == nil {
		t.Fatalf("input missing from payload: %#v", gotPayload)
	}
	if gotPayload["store"] != false {
		t.Fatalf("store = %#v, want false", gotPayload["store"])
	}
	if gotPayload["stream"] != true {
		t.Fatalf("stream = %#v, want true", gotPayload["stream"])
	}
	if gotPayload["input"] != "Reply with exactly: OK" {
		t.Fatalf("input = %#v, want constrained probe prompt", gotPayload["input"])
	}
	if gotPayload["max_output_tokens"] != float64(128) {
		t.Fatalf("max_output_tokens = %#v, want 128", gotPayload["max_output_tokens"])
	}
}

func TestCheckResponsesAPIWithClientAcceptsStreamingResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Fatalf("Accept = %q, want text/event-stream", got)
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = w.Write([]byte("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-test\",\"object\":\"response\",\"status\":\"in_progress\"}}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-test\",\"object\":\"response\",\"status\":\"completed\"}}\n\n"))
	}))
	defer srv.Close()

	if err := CheckResponsesAPIWithClient(context.Background(), srv.Client(), srv.URL+"/v1", "sk-test", "gpt-test", nil); err != nil {
		t.Fatalf("CheckResponsesAPIWithClient() error = %v", err)
	}
}

func TestCheckResponsesAPIWithClientRejectsCreatedWithoutCompletion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"object\":\"response\",\"status\":\"in_progress\"}}\n\n"))
	}))
	defer srv.Close()

	err := CheckResponsesAPIWithClient(context.Background(), srv.Client(), srv.URL, "sk-test", "gpt-test", nil)
	if err == nil || !strings.Contains(err.Error(), "before response.completed") {
		t.Fatalf("error = %v, want incomplete stream rejection", err)
	}
}

func TestCheckResponsesAPIWithClientMapsFailedEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"object\":\"response\",\"status\":\"failed\",\"error\":{\"code\":\"model_unavailable\",\"message\":\"internal route detail\"}}}\n\n"))
	}))
	defer srv.Close()

	err := CheckResponsesAPIWithClient(context.Background(), srv.Client(), srv.URL, "sk-test", "gpt-test", nil)
	status, code, message, ok := UserFacingUpstreamError(err)
	if !ok || status != http.StatusServiceUnavailable || code != "model_unavailable" {
		t.Fatalf("mapped error = (%d, %q, %q, %t), source=%v", status, code, message, ok, err)
	}
}

func TestCheckResponsesAPIWithClientAcceptsMultilineSSEData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\ndata: \"response\":{\"id\":\"resp-test\",\"object\":\"response\",\"status\":\"completed\"}}\n\n"))
	}))
	defer srv.Close()

	if err := CheckResponsesAPIWithClient(context.Background(), srv.Client(), srv.URL, "sk-test", "gpt-test", nil); err != nil {
		t.Fatalf("CheckResponsesAPIWithClient() error = %v", err)
	}
}

func TestCheckResponsesAPIWithClientClassifiesUnsupportedEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no responses here", http.StatusNotFound)
	}))
	defer srv.Close()

	err := CheckResponsesAPIWithClient(context.Background(), srv.Client(), srv.URL+"/v1", "sk-test", "gpt-test", nil)
	if err == nil {
		t.Fatal("CheckResponsesAPIWithClient() error = nil, want unsupported status")
	}
	if !errors.Is(err, ErrResponsesAPIUnsupported) {
		t.Fatalf("CheckResponsesAPIWithClient() error = %v, want ErrResponsesAPIUnsupported", err)
	}
}

func TestCheckResponsesAPIWithClientReturnsFriendlyWrappedUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"code":"model_unavailable","message":"model gpt-test has no available upstream endpoint"}}`))
	}))
	defer srv.Close()

	err := CheckResponsesAPIWithClient(context.Background(), srv.Client(), srv.URL, "sk-test", "gpt-test", nil)
	status, code, message, ok := UserFacingUpstreamError(err)
	if !ok {
		t.Fatalf("UserFacingUpstreamError(%v) ok = false, want true", err)
	}
	if status != http.StatusServiceUnavailable || code != "model_unavailable" {
		t.Fatalf("UserFacingUpstreamError() = (%d, %q), want (503, model_unavailable)", status, code)
	}
	if message != "The selected model is temporarily unavailable. Try again later or choose another model." {
		t.Fatalf("UserFacingUpstreamError() message = %q", message)
	}
	if !strings.Contains(err.Error(), "no available upstream endpoint") {
		t.Fatalf("diagnostic error = %q, want original detail retained for server-side diagnostics", err)
	}
}

func TestCheckChatCompletionsAPIWithClientReturnsTypedUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":"rate_limit_exceeded","message":"internal quota detail"}}`))
	}))
	defer srv.Close()

	err := CheckChatCompletionsAPIWithClient(context.Background(), srv.Client(), srv.URL, "sk-test", "gpt-test", nil)
	status, code, message, ok := UserFacingUpstreamError(err)
	if !ok || status != http.StatusTooManyRequests || code != "rate_limit_exceeded" {
		t.Fatalf("UserFacingUpstreamError(%v) = (%d, %q, %t)", err, status, code, ok)
	}
	if message != "The model service is busy or its quota has been reached. Please try again later." {
		t.Fatalf("UserFacingUpstreamError() message = %q", message)
	}
}

func TestUserFacingUpstreamErrorSanitizesTransportAndDecodeFailures(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "timeout",
			err:        &UpstreamRequestError{Operation: "request responses", BaseURL: "https://private.example/v1", Err: context.DeadlineExceeded},
			wantStatus: http.StatusGatewayTimeout,
			wantCode:   "upstream_timeout",
		},
		{
			name:       "invalid response",
			err:        &UpstreamRequestError{Operation: "decode responses", BaseURL: "https://private.example/v1", Err: errors.New("invalid byte at offset 42")},
			wantStatus: http.StatusBadGateway,
			wantCode:   "upstream_unavailable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code, message, ok := UserFacingUpstreamError(fmt.Errorf("validate provider: %w", tt.err))
			if !ok || status != tt.wantStatus || code != tt.wantCode {
				t.Fatalf("UserFacingUpstreamError() = (%d, %q, %t)", status, code, ok)
			}
			if strings.Contains(message, "private.example") || strings.Contains(message, "offset") {
				t.Fatalf("message exposes diagnostics: %q", message)
			}
		})
	}
}

func TestFriendlyUpstreamErrorMappings(t *testing.T) {
	tests := []struct {
		code       string
		wantStatus int
		wantText   string
	}{
		{code: "invalid_api_key", wantStatus: http.StatusUnauthorized, wantText: "authentication failed"},
		{code: "authentication_error", wantStatus: http.StatusUnauthorized, wantText: "authentication failed"},
		{code: "permission_denied", wantStatus: http.StatusForbidden, wantText: "does not have permission"},
		{code: "context_length_exceeded", wantStatus: http.StatusBadRequest, wantText: "exceeds this model's context length"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			status := UpstreamStatusForErrorCode(tt.code)
			if status != tt.wantStatus {
				t.Fatalf("UpstreamStatusForErrorCode(%q) = %d, want %d", tt.code, status, tt.wantStatus)
			}
			if message := FriendlyUpstreamErrorMessage(status, tt.code); !strings.Contains(message, tt.wantText) {
				t.Fatalf("FriendlyUpstreamErrorMessage(%q) = %q, want text %q", tt.code, message, tt.wantText)
			}
		})
	}
}

func TestUserFacingUpstreamErrorAddsFallbackCode(t *testing.T) {
	err := &ResponsesAPIStatusError{StatusCode: http.StatusTooManyRequests, Status: "429 Too Many Requests"}
	status, code, _, ok := UserFacingUpstreamError(err)
	if !ok || status != http.StatusTooManyRequests || code != "rate_limit_exceeded" {
		t.Fatalf("UserFacingUpstreamError() = (%d, %q, %t), want (429, rate_limit_exceeded, true)", status, code, ok)
	}
}

func TestUpstreamErrorDetectsErrorObjectWithoutCode(t *testing.T) {
	tests := []string{
		`{"error":{"message":"route unavailable"}}`,
		`{"error":{"code":null,"message":"route unavailable"}}`,
	}
	for _, body := range tests {
		if code, ok := UpstreamError([]byte(body)); !ok || code != "" {
			t.Fatalf("UpstreamError(%s) = (%q, %t), want empty code and true", body, code, ok)
		}
	}
	if code, ok := UpstreamError([]byte(`{"error":null,"choices":[]}`)); ok || code != "" {
		t.Fatalf("UpstreamError(error=null) = (%q, %t), want empty code and false", code, ok)
	}
}

func TestCheckChatCompletionsAPIWithClientAcceptsStreamingResponse(t *testing.T) {
	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Fatalf("Accept = %q, want text/event-stream", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"pong\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
	}))
	defer srv.Close()

	if err := CheckChatCompletionsAPIWithClient(context.Background(), srv.Client(), srv.URL+"/v1", "sk-test", "gpt-test", nil); err != nil {
		t.Fatalf("CheckChatCompletionsAPIWithClient() error = %v", err)
	}
	if gotPayload["stream"] != true {
		t.Fatalf("stream = %#v, want true", gotPayload["stream"])
	}
}

func TestCheckChatCompletionsAPIWithClientAcceptsMultilineSSEData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-test\",\ndata: \"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
	}))
	defer srv.Close()

	if err := CheckChatCompletionsAPIWithClient(context.Background(), srv.Client(), srv.URL, "sk-test", "gpt-test", nil); err != nil {
		t.Fatalf("CheckChatCompletionsAPIWithClient() error = %v", err)
	}
}

func TestCheckResponsesOrChatCompletionsAPIWithClientFallsBackToChat(t *testing.T) {
	var chatPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			http.Error(w, "no responses here", http.StatusNotFound)
		case "/v1/chat/completions":
			if err := json.NewDecoder(r.Body).Decode(&chatPayload); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"pong"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := CheckResponsesOrChatCompletionsAPIWithClient(context.Background(), srv.Client(), srv.URL+"/v1", "sk-test", "gpt-test", nil)
	if err != nil {
		t.Fatalf("CheckResponsesOrChatCompletionsAPIWithClient() error = %v", err)
	}
	if chatPayload["model"] != "gpt-test" {
		t.Fatalf("chat model = %#v, want gpt-test", chatPayload["model"])
	}
	if chatPayload["stream"] != true {
		t.Fatalf("chat stream = %#v, want true", chatPayload["stream"])
	}
	messages, ok := chatPayload["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("chat messages = %#v, want one probe message", chatPayload["messages"])
	}
}

func TestCheckResponsesOrChatCompletionsAPIWithClientRejectsBadBaseURL(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	err := CheckResponsesOrChatCompletionsAPIWithClient(context.Background(), srv.Client(), srv.URL+"/wrong/v1", "sk-test", "gpt-test", nil)
	if err == nil {
		t.Fatal("CheckResponsesOrChatCompletionsAPIWithClient() error = nil, want invalid fallback rejection")
	}
	if !strings.Contains(err.Error(), "chat completions fallback") || !strings.Contains(err.Error(), "404") {
		t.Fatalf("CheckResponsesOrChatCompletionsAPIWithClient() error = %v, want chat fallback 404", err)
	}
}
