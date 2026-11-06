// Package custom embeds a separately built frontend. Its source framework and
// build toolchain are independent of the existing web/app application.
package custom

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"

	"csgclaw/internal/uiassets"
)

//go:embed all:static-dist
var assets embed.FS

// Handler serves frontend paths relative to its root; the caller may strip a mount prefix.
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(assets, "static-dist")
	if err != nil {
		return nil, err
	}
	if info, err := fs.Stat(sub, "index.html"); err != nil || info.IsDir() {
		return nil, fmt.Errorf("custom UI assets are not built; run make prepare-custom-ui UI_DIR=<frontend-dist>")
	}
	return uiassets.SPA(sub), nil
}
