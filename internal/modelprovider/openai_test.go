package modelprovider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
	if gotPayload["stream"] != false {
		t.Fatalf("stream = %#v, want false", gotPayload["stream"])
	}
	if gotPayload["max_output_tokens"] != float64(16) {
		t.Fatalf("max_output_tokens = %#v, want 16", gotPayload["max_output_tokens"])
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
