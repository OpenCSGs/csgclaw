package serve

import "net/http"

func (opts serveOptions) apiOnly() bool {
	return opts.APIOnly || !embeddedUIAvailable
}

func (opts serveOptions) uiHandler() (http.Handler, error) {
	if opts.apiOnly() {
		return nil, nil
	}
	return bundledUIHandler()
}

func (opts serveOptions) uiURL(apiURL string) string {
	if opts.apiOnly() {
		return ""
	}
	return imOpenURL(apiURL)
}
