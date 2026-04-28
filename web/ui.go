package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed static/*
var staticFiles embed.FS

func Handler() http.Handler {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	return spaHandler{fsys: sub, files: http.FileServer(http.FS(sub))}
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
	http.ServeFileFS(w, r, fsys, "index.html")
}
