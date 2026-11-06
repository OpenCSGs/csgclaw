//go:build !headless && ui_custom && !ui_both

package serve

import (
	"net/http"

	"csgclaw/web/custom"
)

const embeddedUIAvailable = true

func bundledUIHandler() (http.Handler, error) { return custom.Handler() }
