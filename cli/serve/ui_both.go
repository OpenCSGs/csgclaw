//go:build !headless && ui_both

package serve

import (
	"net/http"

	"csgclaw/internal/uiassets"
	webui "csgclaw/web"
	"csgclaw/web/custom"
)

const embeddedUIAvailable = true

func bundledUIHandler() (http.Handler, error) {
	secondary, err := custom.Handler()
	if err != nil {
		return nil, err
	}
	return uiassets.WithCustom(webui.Handler(), secondary), nil
}
