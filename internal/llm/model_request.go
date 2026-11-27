package llm

import (
	"bytes"
	"context"
	"csgclaw/internal/diagnostics"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
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
	if s.managedGateway.matches(req) {
		return s.managedGatewayRequest(req, stream)
	}
	return s.modelRequestWithRecovery(s.client, req, stream)
}

func (s *Service) modelRequestWithRecovery(client *http.Client, req *http.Request, stream bool) (*http.Response, error) {
	const attempts = 5
	original := req
	exchange, _ := req.Context().Value(modelExchangeKey{}).(*modelExchange)
	managed := exchange != nil && exchange.openCSG
	if managed {
		// Preserve client policies, including the CSGHub gateway redirect guard.
		tracedClient := *client
		tracedClient.Transport = s.gatewayClient.Transport
		client = &tracedClient
	}
	connectionFailed := false
	var recovery diagnostics.ModelRequestStatus
	for attempt := 0; ; attempt++ {
		response, err := s.modelRequestAttempt(client, req, stream)
		if original.Context().Err() != nil {
			return response, err
		}
		if err == nil && response.StatusCode < 400 {
			if recovery.State != "" {
				recovery.State, recovery.RetryAt = "recovered", nil
				recovery.GatewayRequestID = upstreamRequestID(response.Header)
				diagnostics.ReportModelRequest(original.Context(), recovery)
			}
			return response, nil
		}
		delay := 250 * time.Millisecond << attempt
		retry := false
		canReplay := attempt < attempts-1 && (original.Body == nil || original.GetBody != nil)
		var body []byte
		if err != nil {
			var operation *net.OpError
			retry = canReplay && (transientTLSFailure(err.Error()) || errors.As(err, &operation) && operation.Op == "dial")
		} else if exchange != nil || canReplay && (response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests) {
			var readErr error
			body, readErr = peekModelError(response)
			if readErr != nil {
				if exchange != nil {
					failure := modelRequestFailure(original.Context(), response, nil, readErr)
					exchange.failure.Store(&failure)
				}
				_ = response.Body.Close()
				return nil, readErr
			}
			var failure struct {
				Error struct {
					Code              string `json:"code"`
					Message           string `json:"message"`
					LastUpstreamError string `json:"last_upstream_error"`
					ResetSeconds      int    `json:"reset_seconds"`
				} `json:"error"`
			}
			decodeErr := json.Unmarshal(body, &failure)
			if canReplay && response.StatusCode == http.StatusTooManyRequests {
				retryAfter := modelRetryAfter(response.Header.Get("Retry-After"), failure.Error.ResetSeconds, time.Now())
				if managed && !permanentRateLimit(failure.Error.Code) {
					delay, retry = gatewayRateLimitBackoff(attempt, retryAfter)
				} else if decodeErr == nil && failure.Error.Code == "model_cooldown" && (connectionFailed || transientTLSFailure(failure.Error.LastUpstreamError)) {
					delay = max(delay, retryAfter)
					retry = delay <= 30*time.Second
				}
			} else if canReplay && response.StatusCode >= 500 && decodeErr == nil {
				retry = transientTLSFailure(failure.Error.Message)
			}
		}
		status := modelRequestFailure(original.Context(), response, body, err)
		if !retry {
			if exchange != nil {
				exchange.failure.Store(&status)
			}
			return response, err
		}
		retryAt := time.Now().Add(delay)
		status.State, status.Attempt, status.MaxAttempts, status.RetryAt = "retrying", attempt+2, attempts, &retryAt
		if status.RequestID != "" {
			diagnostics.ReportModelRequest(original.Context(), status)
			slog.InfoContext(original.Context(), "model request retry scheduled", "request_id", status.RequestID, "trace_id", modelTraceID(original.Context()), "gateway_request_id", status.GatewayRequestID, "category", status.Category, "source", status.Source, "attempt", status.Attempt, "max_attempts", attempts, "retry_at", status.RetryAt)
			recovery = status
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

func gatewayRateLimitBackoff(attempt int, retryAfter time.Duration) (time.Duration, bool) {
	const maxWait = 30 * time.Second
	// Spread retries from clients throttled at the same time. Each successive
	// retry doubles its base wait, and never precedes Retry-After.
	base := time.Second << attempt
	delay := max(retryAfter, min(maxWait, base+rand.N(base/4)))
	return delay, delay <= maxWait
}

func modelRetryAfter(header string, resetSeconds int, now time.Time) time.Duration {
	const aboveLimit = 31
	delay := time.Duration(min(aboveLimit, max(0, resetSeconds))) * time.Second
	if seconds, err := strconv.ParseUint(strings.TrimSpace(header), 10, 64); err == nil {
		return max(delay, time.Duration(min(aboveLimit, seconds))*time.Second)
	}
	if at, err := http.ParseTime(header); err == nil {
		return max(delay, at.Sub(now))
	}
	return delay
}

func permanentRateLimit(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "insufficient_quota", "insufficient_balance", "token_plan_quota_exceeded", "billing_hard_limit_reached":
		return true
	default:
		return false
	}
}

func temporaryRateLimit(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "rate_limit_exceeded", "rate_limit_error", "queue_full", "capacity_exceeded", "user_rate_limit_exceeded", "user_g_rate_limit_exceeded", "model_cooldown", "upstream_rate_limit_exceeded", "too_many_requests":
		return true
	default:
		return false
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
