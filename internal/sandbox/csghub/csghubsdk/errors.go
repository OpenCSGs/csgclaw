package csghubsdk

import (
	"errors"
	"fmt"
	"strings"
)

// HTTPError wraps a non-2xx response from a lifecycle or runtime call.
type HTTPError struct {
	// StatusCode is the HTTP response status.
	StatusCode int
	// URL is the full request URL that produced the failure.
	URL string
	// Detail is the best-effort human-readable body (may be truncated).
	Detail string
}

func (e *HTTPError) Error() string {
	if e == nil {
		return "sandbox: nil http error"
	}
	return fmt.Sprintf("sandbox: HTTP %d: %s", e.StatusCode, e.Detail)
}

// TransportError wraps failed network / TLS calls (pre-response errors).
type TransportError struct {
	URL   string
	Cause error
}

func (e *TransportError) Error() string {
	if e == nil {
		return "sandbox: nil transport error"
	}
	if e.URL == "" {
		return fmt.Sprintf("sandbox: transport error: %v", e.Cause)
	}
	return fmt.Sprintf("sandbox: transport error at %s: %v", e.URL, e.Cause)
}

func (e *TransportError) Unwrap() error { return e.Cause }

// ParseError indicates the server returned a body that could not be decoded
// into the expected shape (for example malformed JSON).
type ParseError struct {
	Detail string
	Cause  error
}

func (e *ParseError) Error() string {
	if e == nil {
		return "sandbox: nil parse error"
	}
	if e.Detail != "" {
		return fmt.Sprintf("sandbox: parse error: %s", e.Detail)
	}
	if e.Cause != nil {
		return fmt.Sprintf("sandbox: parse error: %v", e.Cause)
	}
	return "sandbox: parse error"
}

func (e *ParseError) Unwrap() error { return e.Cause }

// IsConflict returns true when err is an HTTPError with status 409.
func IsConflict(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == 409
	}
	return false
}

// IsNotFound returns true when err semantically means "sandbox missing".
//
// Some CSGHub clusters return HTTP 400 with a "not found" detail instead of
// HTTP 404 for missing sandboxes, so we treat both shapes as not-found.
func IsNotFound(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		if httpErr.StatusCode == 404 {
			return true
		}
		if httpErr.StatusCode == 400 {
			detail := strings.ToLower(strings.TrimSpace(httpErr.Detail))
			return strings.Contains(detail, "not found")
		}
	}
	return false
}

func formatCodeMessage(code int, msg string) string {
	return fmt.Sprintf("%d: %s", code, msg)
}
