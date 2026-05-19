package webui

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:static-dist
var staticFiles embed.FS

func Handler() http.Handler {
	sub, err := fs.Sub(staticFiles, "static-dist")
	if err != nil {
		panic(err)
	}
	if !fileExists(sub, "index.html") {
		return missingStaticDistHandler{}
	}
	return spaHandler{fsys: sub, files: http.FileServer(http.FS(sub))}
}

type missingStaticDistHandler struct{}

func (missingStaticDistHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = fmt.Fprintln(w, "CSGClaw Web UI assets are not built.")
	_, _ = fmt.Fprintln(w, "Run `make build-web` from the csgclaw repository, then restart the server.")
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
