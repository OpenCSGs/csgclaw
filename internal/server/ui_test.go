package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIAwareFallbackRejectsUnknownAPIPaths(t *testing.T) {
	handler := apiAwareFallbackHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ui"))
	}))

	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api"},
		{method: http.MethodGet, path: "/api/unknown/u-manager/events"},
		{method: http.MethodPost, path: "/api/v1/channels/csgclaw/bots"},
	}

	for _, tt := range tests {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s status = %d, want %d", tt.method, tt.path, rec.Code, http.StatusNotFound)
		}
	}
}

func TestAPIAwareFallbackServesUIPaths(t *testing.T) {
	handler := apiAwareFallbackHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ui"))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/workspace", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "ui" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "ui")
	}
}
