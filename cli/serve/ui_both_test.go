//go:build !headless && ui_both

package serve

import (
	"net/http/httptest"
	"testing"

	"csgclaw/internal/uiassets"
	webui "csgclaw/web"
	"csgclaw/web/custom"
)

func TestBothBuildPreservesExistingUIAndAddsCustom(t *testing.T) {
	handler, err := bundledUIHandler()
	if err != nil {
		t.Fatal(err)
	}
	existing := webui.Handler()
	for _, path := range []string{"/", "/workspace", "/sse-shared-worker.js"} {
		before, after := httptest.NewRecorder(), httptest.NewRecorder()
		existing.ServeHTTP(before, httptest.NewRequest("GET", path, nil))
		handler.ServeHTTP(after, httptest.NewRequest("GET", path, nil))
		if after.Code != 200 || before.Code != after.Code || before.Body.String() != after.Body.String() || before.Header().Get("Cache-Control") != after.Header().Get("Cache-Control") {
			t.Fatalf("existing UI changed at %s: status=%d", path, after.Code)
		}
	}
	secondary, err := custom.Handler()
	if err != nil {
		t.Fatal(err)
	}
	before, after := httptest.NewRecorder(), httptest.NewRecorder()
	secondary.ServeHTTP(before, httptest.NewRequest("GET", "/", nil))
	handler.ServeHTTP(after, httptest.NewRequest("GET", uiassets.CustomPath, nil))
	if after.Code != 200 || before.Body.String() != after.Body.String() {
		t.Fatal("both build did not expose the selected custom UI")
	}
	if got := (serveOptions{}).uiURL("https://example.test/instance/35"); got != "https://example.test/instance/35/" {
		t.Fatalf("default browser entrypoint changed: %s", got)
	}
}
