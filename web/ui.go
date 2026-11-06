package webui

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"

	"csgclaw/internal/uiassets"
)

//go:embed all:static-dist
var staticFiles embed.FS

func Handler() http.Handler {
	sub, err := fs.Sub(staticFiles, "static-dist")
	if err != nil {
		panic(err)
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return missingStaticDistHandler{}
	}
	return uiassets.SPA(sub)
}

type missingStaticDistHandler struct{}

func (missingStaticDistHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = fmt.Fprintln(w, "CSGClaw Web UI assets are not built.")
	_, _ = fmt.Fprintln(w, "Run `make build-web` from the csgclaw repository, then restart the server.")
}
