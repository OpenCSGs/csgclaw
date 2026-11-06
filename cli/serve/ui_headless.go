//go:build headless

package serve

import "net/http"

const embeddedUIAvailable = false

func bundledUIHandler() (http.Handler, error) { return nil, nil }
