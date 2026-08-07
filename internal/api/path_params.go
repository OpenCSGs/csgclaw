package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
)

const hubTemplateNamespaceSeparator = "~s~"

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

func hubTemplateIDPathValue(r *http.Request) string {
	value := pathValue(r, "id")
	var decoded strings.Builder
	for index := 0; index < len(value); {
		switch {
		case strings.HasPrefix(value[index:], hubTemplateNamespaceSeparator):
			decoded.WriteByte('/')
			index += len(hubTemplateNamespaceSeparator)
		case strings.HasPrefix(value[index:], "~~"):
			decoded.WriteByte('~')
			index += 2
		default:
			decoded.WriteByte(value[index])
			index++
		}
	}
	return decoded.String()
}
