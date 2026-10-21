package api

import (
	"csgclaw/internal/modelprovider"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
)

// Log only structured upstream metadata: errors and response bodies may contain
// credentials, prompts, or other private provider diagnostics.
func agentOperationDiagnosticAttrs(err error) []slog.Attr {
	var statusErr *modelprovider.ResponsesAPIStatusError
	if errors.As(err, &statusErr) {
		return []slog.Attr{
			slog.String("upstream_operation", statusErr.Operation),
			slog.String("upstream_url", diagnosticEndpoint(statusErr.BaseURL)),
			slog.Int("upstream_status", statusErr.StatusCode),
			slog.String("upstream_code", modelprovider.UpstreamErrorCodeForResponse(statusErr.StatusCode, []byte(statusErr.Body))),
		}
	}
	var requestErr *modelprovider.UpstreamRequestError
	if errors.As(err, &requestErr) {
		return []slog.Attr{
			slog.String("upstream_operation", requestErr.Operation),
			slog.String("upstream_url", diagnosticEndpoint(requestErr.BaseURL)),
		}
	}
	return []slog.Attr{slog.String("error_type", fmt.Sprintf("%T", err))}
}

func diagnosticEndpoint(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "[invalid endpoint]"
	}
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	return u.String()
}
