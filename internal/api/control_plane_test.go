package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestControlPlaneErrorsPreserveStatusAndDetails(t *testing.T) {
	for _, test := range []struct {
		name string
		run  http.HandlerFunc
		code string
	}{
		{"plain", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "invalid model", 400) }, "invalid_request"},
		{"coded", func(w http.ResponseWriter, _ *http.Request) {
			writeCodedAPIError(w, 409, "resource_version_conflict", "changed")
		}, "resource_version_conflict"},
		{"details", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "1")
			writeJSON(w, 429, map[string]any{"error": "try again", "published_template_id": "local/template"})
		}, "rate_limited"},
	} {
		t.Run(test.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			controlPlane(test.run)(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			var body struct {
				Error               struct{ Code, Message string }
				PublishedTemplateID string `json:"published_template_id"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != test.code || body.Error.Message == "" || rec.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("response: %d %s", rec.Code, rec.Body.String())
			}
			if test.name == "details" && (rec.Code != 429 || body.PublishedTemplateID != "local/template" || rec.Header().Get("Retry-After") != "1") {
				t.Fatalf("lost failure details: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestControlPlaneSuccessfulStreamIsNotBuffered(t *testing.T) {
	rec := httptest.NewRecorder()
	controlPlane(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "first line\n")
		w.(http.Flusher).Flush()
		if rec.Body.String() != "first line\n" || !rec.Flushed {
			t.Fatal("stream must reach the client before the handler completes")
		}
		fmt.Fprint(w, "runtime failure\n")
	})(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 200 || rec.Body.String() != "first line\nruntime failure\n" || rec.Header().Get("Content-Type") != "text/plain" {
		t.Fatalf("changed stream: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRoomLLMRouteDoesNotAcceptTaskOrUnversionedAliases(t *testing.T) {
	h := &Handler{serverNoAuth: true}
	for _, path := range []string{
		"/api/v1/agents/alice/llm/models",
		"/api/v1/agents/alice/tasks/room/llm/v1/models",
		"/api/v1/agents/alice/rooms/room/llm/models",
	} {
		rec := httptest.NewRecorder()
		h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != 404 {
			t.Fatalf("%s: got %d, want 404", path, rec.Code)
		}
	}
}
