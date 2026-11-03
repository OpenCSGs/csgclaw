package uiassets

import (
	"net/http"
	"strings"
)

// CustomPath is the optional second UI's entrypoint. The existing UI stays at /.
const CustomPath = "/ui/custom/"

// WithCustom routes only CustomPath to the second UI. Every other path is passed
// unchanged to the existing UI, preserving its routes and asset URLs.
// The backend still owns API routes and wraps this handler with its API fallback.
func WithCustom(primary, custom http.Handler) http.Handler {
	prefix := strings.TrimSuffix(CustomPath, "/")
	secondary := http.StripPrefix(prefix, custom)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == prefix:
			// A relative Location preserves an external gateway prefix.
			location := "custom/"
			if r.URL.RawQuery != "" {
				location += "?" + r.URL.RawQuery
			}
			w.Header().Set("Location", location)
			w.WriteHeader(http.StatusTemporaryRedirect)
		case strings.HasPrefix(r.URL.Path, CustomPath):
			secondary.ServeHTTP(w, r)
		default:
			primary.ServeHTTP(w, r)
		}
	})
}
