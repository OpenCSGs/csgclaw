package uiassets

import (
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestSPA(t *testing.T) {
	handler := SPA(fstest.MapFS{
		"index.html":    {Data: []byte("<html>custom UI</html>")},
		"assets/app.js": {Data: []byte("custom script")},
	})
	for _, tc := range []struct {
		method, path, body, cache string
		status                    int
	}{
		{"GET", "/", "<html>custom UI</html>", "no-cache", 200},
		{"HEAD", "/", "", "no-cache", 200},
		{"GET", "/room/1", "<html>custom UI</html>", "no-cache", 200},
		{"GET", "/assets/app.js", "custom script", "public, max-age=31536000, immutable", 200},
		{"GET", "/assets/missing.js", "404 page not found\n", "", 404},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != tc.status || rec.Body.String() != tc.body || rec.Header().Get("Cache-Control") != tc.cache {
				t.Fatalf("status=%d body=%q cache=%q", rec.Code, rec.Body.String(), rec.Header().Get("Cache-Control"))
			}
		})
	}
}
