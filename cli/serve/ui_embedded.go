//go:build !headless && !ui_custom && !ui_both

package serve

import (
	"net/http"

	webui "csgclaw/web"
)

const embeddedUIAvailable = true

// The application entrypoint chooses the UI; the HTTP backend only consumes a handler.
func bundledUIHandler() (http.Handler, error) { return webui.Handler(), nil }
