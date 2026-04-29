package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesIndexForSPARoutes(t *testing.T) {
	handler := Handler()
	for _, route := range []string{"/", "/computer", "/agents/u-manager", "/rooms/room-1", "/channels/room-1", "/dms/dm-1"} {
		t.Run(route, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, route, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if !strings.Contains(rec.Body.String(), `<div id="root"></div>`) {
				t.Fatalf("body does not look like index.html: %q", rec.Body.String())
			}
		})
	}
}

func TestHandlerServesAssetsAndKeepsMissingAssetsNotFound(t *testing.T) {
	handler := Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("app.js status = %d, want %d", rec.Code, http.StatusOK)
	}
	if strings.Contains(rec.Body.String(), `<div id="root"></div>`) {
		t.Fatal("app.js returned index.html")
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing.js status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
