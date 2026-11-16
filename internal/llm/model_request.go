package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Chat and background generation share connection recovery here. Only failures
// known to precede model execution may be replayed. Once a response stream has
// been handed to a caller, its failures are never replayed by this layer.
func (s *Service) modelRequest(req *http.Request, stream bool) (*http.Response, error) {
	const attempts = 5
	original := req
	connectionFailed := false
	for attempt := 0; ; attempt++ {
		response, err := s.modelRequestAttempt(req, stream)
		if attempt == attempts-1 || original.Context().Err() != nil || (original.Body != nil && original.GetBody == nil) {
			return response, err
		}
		delay := 250 * time.Millisecond << attempt
		retry := false
		if err != nil {
			var operation *net.OpError
			retry = transientTLSFailure(err.Error()) || errors.As(err, &operation) && operation.Op == "dial"
		} else if response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests {
			var body []byte
			body, err = peekModelError(response)
			if err != nil {
				_ = response.Body.Close()
				return nil, err
			}
			var failure struct {
				Error struct {
					Code              string `json:"code"`
					Message           string `json:"message"`
					LastUpstreamError string `json:"last_upstream_error"`
					ResetSeconds      int    `json:"reset_seconds"`
				} `json:"error"`
			}
			if json.Unmarshal(body, &failure) == nil {
				if response.StatusCode >= 500 {
					retry = transientTLSFailure(failure.Error.Message)
				} else if failure.Error.Code == "model_cooldown" && (connectionFailed || transientTLSFailure(failure.Error.LastUpstreamError)) {
					retry = true
					if failure.Error.ResetSeconds > 30 {
						return response, nil
					}
					delay = max(delay, time.Duration(max(0, failure.Error.ResetSeconds))*time.Second)
					if seconds, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil {
						if seconds > 30 {
							return response, nil
						}
						delay = max(delay, time.Duration(max(0, seconds))*time.Second)
					} else if at, err := http.ParseTime(response.Header.Get("Retry-After")); err == nil {
						delay = max(delay, time.Until(at))
					}
					if delay > 30*time.Second {
						retry = false
					}
				}
			}
		}
		if !retry {
			return response, err
		}
		connectionFailed = true
		if response != nil {
			_ = response.Body.Close()
		}
		if err := waitForModelRetry(original.Context(), delay); err != nil {
			return nil, err
		}
		req = original.Clone(original.Context())
		if original.GetBody != nil {
			req.Body, err = original.GetBody()
			if err != nil {
				return nil, err
			}
		}
	}
}

func transientTLSFailure(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "tls handshake") &&
		(strings.Contains(message, "eof") || strings.Contains(message, "connection reset") ||
			strings.Contains(message, "connection closed") || strings.Contains(message, "handshake timeout"))
}

func waitForModelRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Preserve the entire final error response, including bodies larger than the
// classification limit, for the existing protocol/error handlers.
func peekModelError(response *http.Response) ([]byte, error) {
	const limit = 16 << 10
	body := response.Body
	prefix, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	response.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(prefix), body), body}
	if len(prefix) > limit {
		return nil, nil
	}
	return prefix, nil
}
