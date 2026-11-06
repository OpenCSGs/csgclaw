package uiassets

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"testing/fstest"
)

func TestWithCustomKeepsExistingRoutesAndSeparatesAssets(t *testing.T) {
	fixture := func(name string) http.Handler {
		return SPA(fstest.MapFS{
			"index.html":    {Data: []byte(name + " UI")},
			"assets/app.js": {Data: []byte(name + " script")},
		})
	}
	primary := fixture("existing")
	handler := WithCustom(primary, fixture("custom"))
	for _, path := range []string{"/", "/workspace", "/rooms/1", "/assets/app.js", "/ui/legacy", "/ui/custom-other"} {
		before, after := httptest.NewRecorder(), httptest.NewRecorder()
		primary.ServeHTTP(before, httptest.NewRequest("GET", path, nil))
		handler.ServeHTTP(after, httptest.NewRequest("GET", path, nil))
		if before.Code != after.Code || before.Body.String() != after.Body.String() || before.Header().Get("Cache-Control") != after.Header().Get("Cache-Control") {
			t.Fatalf("existing route changed: %s", path)
		}
	}
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{CustomPath, "custom UI", 200},
		{CustomPath + "room/1", "custom UI", 200},
		{CustomPath + "assets/app.js", "custom script", 200},
		{CustomPath + "assets/missing.js", "404 page not found\n", 404},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", tc.path, nil))
		if rec.Code != tc.status || rec.Body.String() != tc.body {
			t.Fatalf("%s: status=%d body=%q", tc.path, rec.Code, rec.Body.String())
		}
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/ui/custom?from=link", nil))
	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("redirect status=%d", rec.Code)
	}
	for _, prefix := range []string{"", "/instance/35"} {
		base, _ := url.Parse("https://example.test" + prefix + "/ui/custom?from=link")
		relative, _ := url.Parse(rec.Header().Get("Location"))
		if got := base.ResolveReference(relative).String(); got != "https://example.test"+prefix+"/ui/custom/?from=link" {
			t.Fatalf("redirect lost prefix or query: %s", got)
		}
	}
}
