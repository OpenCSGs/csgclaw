package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
)

func pathValue(r *http.Request, key string) string {
	if r == nil {
		return ""
	}
	if value := strings.TrimSpace(chi.URLParam(r, key)); value != "" {
		if decoded, err := url.PathUnescape(value); err == nil {
			return strings.TrimSpace(decoded)
		}
		return value
	}
	value := strings.TrimSpace(r.PathValue(key))
	if decoded, err := url.PathUnescape(value); err == nil {
		return strings.TrimSpace(decoded)
	}
	return value
}
