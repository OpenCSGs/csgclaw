package modelprovider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestCSGHubLiteEndpointCandidates(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    []string
	}{
		{
			name:    "default prefers CLI then Desktop",
			baseURL: CSGHubLiteDefaultBaseURL,
			want:    []string{CSGHubLiteDefaultBaseURL, CSGHubLiteDesktopAPIBaseURL},
		},
		{
			name:    "Desktop prefers Desktop then CLI",
			baseURL: CSGHubLiteDesktopAPIBaseURL,
			want:    []string{CSGHubLiteDesktopAPIBaseURL, CSGHubLiteDefaultBaseURL},
		},
		{
			name:    "custom endpoint has no implicit local fallback",
			baseURL: "https://models.example.com/v1/",
			want:    []string{"https://models.example.com/v1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := csgHubLiteEndpointCandidates(tt.baseURL); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("csgHubLiteEndpointCandidates(%q) = %q, want %q", tt.baseURL, got, tt.want)
			}
		})
	}
}

func TestListCSGHubLiteModelsFallsBackToDesktopAPI(t *testing.T) {
	var requested []string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requested = append(requested, req.URL.String())
		if got := req.Header.Get("Authorization"); got != "Bearer "+CSGHubLiteDefaultAPIKey {
			t.Fatalf("Authorization = %q, want default API key", got)
		}
		if req.URL.Host == "127.0.0.1:11435" {
			return nil, fmt.Errorf("connection refused")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"Qwen3.5-2B"}]}`)),
			Request:    req,
		}, nil
	})}

	result, err := ListCSGHubLiteModelsWithClient(
		context.Background(),
		client,
		"",
		"",
		nil,
	)

	if err != nil {
		t.Fatalf("ListCSGHubLiteModelsWithClient() error = %v", err)
	}
	if got, want := strings.Join(requested, ","), "http://127.0.0.1:11435/v1/models,http://127.0.0.1:11436/v1/models"; got != want {
		t.Fatalf("requested URLs = %q, want %q", got, want)
	}
	if result.ResolvedBaseURL != CSGHubLiteDesktopAPIBaseURL {
		t.Fatalf("ResolvedBaseURL = %q, want desktop API %q", result.ResolvedBaseURL, CSGHubLiteDesktopAPIBaseURL)
	}
	if got, want := strings.Join(result.Models, ","), "Qwen3.5-2B"; got != want {
		t.Fatalf("Models = %q, want %q", got, want)
	}
}
