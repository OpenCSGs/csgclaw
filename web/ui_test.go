package webui

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestHandlerServesIndexForSPARoutes(t *testing.T) {
	requireBuiltWebAssets(t)

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
	requireBuiltWebAssets(t)

	handler := Handler()

	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	if index.Code != http.StatusOK {
		t.Fatalf("index status = %d, want %d", index.Code, http.StatusOK)
	}

	assets := assetPaths(index.Body.String())
	if len(assets) == 0 {
		t.Fatal("index.html does not reference any local assets")
	}
	for _, asset := range assets {
		t.Run(asset, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, asset, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("%s status = %d, want %d", asset, rec.Code, http.StatusOK)
			}
			if strings.Contains(rec.Body.String(), `<div id="root"></div>`) {
				t.Fatalf("%s returned index.html", asset)
			}
		})
	}

	for _, missing := range []string{"/missing.js", "/assets/missing.js"} {
		t.Run(missing, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, missing, nil))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("%s status = %d, want %d", missing, rec.Code, http.StatusNotFound)
			}
		})
	}
}

func TestHandlerSetsCacheHeaders(t *testing.T) {
	requireBuiltWebAssets(t)

	handler := Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("index Cache-Control = %q, want no-cache", got)
	}

	for _, asset := range assetPaths(rec.Body.String()) {
		if !strings.HasPrefix(asset, "/assets/") {
			continue
		}
		assetRec := httptest.NewRecorder()
		handler.ServeHTTP(assetRec, httptest.NewRequest(http.MethodGet, asset, nil))
		if got := assetRec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
			t.Fatalf("%s Cache-Control = %q, want immutable asset cache", asset, got)
		}
		return
	}
	t.Fatal("index.html does not reference a Vite /assets/ file")
}

func TestHandlerReportsMissingWebBuild(t *testing.T) {
	if builtWebAssets() {
		t.Skip("web/static-dist is built")
	}

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if body := rec.Body.String(); !strings.Contains(body, "make build-web") {
		t.Fatalf("body = %q, want make build-web hint", body)
	}
}

func requireBuiltWebAssets(t *testing.T) {
	t.Helper()
	if !builtWebAssets() {
		t.Skip("web/static-dist is not built; run make build-web")
	}
}

func builtWebAssets() bool {
	return fileExists(staticFiles, "static-dist/index.html")
}

var htmlAssetPattern = regexp.MustCompile(`(?:src|href)=["']([^"']+)["']`)

func assetPaths(index string) []string {
	matches := htmlAssetPattern.FindAllStringSubmatch(index, -1)
	seen := map[string]bool{}
	var paths []string
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		value := strings.TrimSpace(match[1])
		if value == "" || value == "/" || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "data:") {
			continue
		}
		if strings.HasPrefix(value, "#") {
			continue
		}
		if !strings.HasPrefix(value, "/") {
			value = "/" + value
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		paths = append(paths, value)
	}
	return paths
}
