// Package uiassets serves static frontend builds without embedding a specific UI.
package uiassets

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// SPA serves frontend assets and falls back to index.html for client routes.
// Callers validate that their build output contains index.html before serving.
func SPA(fsys fs.FS) http.Handler {
	return spaHandler{fsys: fsys, files: http.FileServer(http.FS(fsys))}
}

type spaHandler struct {
	fsys  fs.FS
	files http.Handler
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.files.ServeHTTP(w, r)
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "." {
		name = ""
	}
	if name == "" {
		serveIndex(w, r, h.fsys)
		return
	}

	if fileExists(h.fsys, name) {
		setAssetCacheHeaders(w, name)
		h.files.ServeHTTP(w, r)
		return
	}

	if path.Ext(name) != "" {
		http.NotFound(w, r)
		return
	}

	serveIndex(w, r, h.fsys)
}

func fileExists(fsys fs.FS, name string) bool {
	info, err := fs.Stat(fsys, name)
	return err == nil && !info.IsDir()
}

func serveIndex(w http.ResponseWriter, r *http.Request, fsys fs.FS) {
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, fsys, "index.html")
}

func setAssetCacheHeaders(w http.ResponseWriter, name string) {
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	}
}
