package server

import (
	"net/http"
	"strings"

	webui "csgclaw/web"
)

func uiHandler() http.Handler {
	return webui.Handler()
}

func uiFallbackHandler() http.Handler {
	return apiAwareFallbackHandler(uiHandler())
}

func apiAwareFallbackHandler(ui http.Handler) http.Handler {
	if ui == nil {
		ui = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		ui.ServeHTTP(w, r)
	})
}
