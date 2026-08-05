package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"csgclaw/internal/runtimecatalog"
)

type bundledCodexResolver struct{}

func (bundledCodexResolver) Ensure(context.Context) (string, error) {
	return "/opt/csgclaw/bin/codex", nil
}

func TestAgentRuntimesListReportsBundledCodex(t *testing.T) {
	handler := &Handler{}
	handler.SetAgentRuntimeService(runtimecatalog.NewService(
		runtimecatalog.WithCodexResolver(bundledCodexResolver{}),
	))

	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/agent-runtimes", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	var runtimes []runtimecatalog.Runtime
	if err := json.NewDecoder(recorder.Body).Decode(&runtimes); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(runtimes) != 2 {
		t.Fatalf("runtimes = %+v, want Codex and Claude Code", runtimes)
	}
	if got := runtimes[0]; got.Name != runtimecatalog.RuntimeCodex || !got.Installed || got.Installable || got.Status != "installed" || got.Path != "/opt/csgclaw/bin/codex" {
		t.Fatalf("Codex runtime = %+v, want bundled installed runtime", got)
	}
}

func TestAgentRuntimeInstallRouteIsUnavailable(t *testing.T) {
	handler := &Handler{}
	handler.SetAgentRuntimeService(runtimecatalog.NewService())

	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/agent-runtimes/codex/install", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("POST status = %d, want 404; body=%s", recorder.Code, recorder.Body.String())
	}
}
