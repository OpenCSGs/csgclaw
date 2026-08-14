package modelprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type UpstreamRequestError struct {
	Operation string
	BaseURL   string
	Err       error
}

func (e *UpstreamRequestError) Error() string {
	return fmt.Sprintf("%s from %s: %v", strings.TrimSpace(e.Operation), strings.TrimSpace(e.BaseURL), e.Err)
}

func (e *UpstreamRequestError) Unwrap() error {
	return e.Err
}

// UserFacingUpstreamError unwraps a provider request error and returns the safe
// response fields that API handlers may expose.
func UserFacingUpstreamError(err error) (status int, code, message string, ok bool) {
	var statusErr *ResponsesAPIStatusError
	if !errors.As(err, &statusErr) {
		var requestErr *UpstreamRequestError
		if !errors.As(err, &requestErr) {
			return 0, "", "", false
		}
		if errors.Is(requestErr, context.DeadlineExceeded) {
			return http.StatusGatewayTimeout, "upstream_timeout", "The model service request timed out. Please try again.", true
		}
		return http.StatusBadGateway, "upstream_unavailable", "The model service is unavailable. Check its configuration or try again later.", true
	}
	status = statusErr.StatusCode
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	code = statusErr.Code()
	if code == "" {
		code = fallbackUpstreamErrorCode(status)
	}
	return status, code, FriendlyUpstreamErrorMessage(status, code), true
}

func fallbackUpstreamErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusPaymentRequired:
		return "insufficient_balance"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusTooManyRequests:
		return "rate_limit_exceeded"
	default:
		return "upstream_unavailable"
	}
}

// UpstreamErrorCode extracts the stable provider error code without exposing
// the provider's diagnostic message to callers.
func UpstreamErrorCode(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if response, ok := payload["response"].(map[string]any); ok {
		if code := errorCodeFromValue(response["error"]); code != "" {
			return code
		}
	}
	value, ok := payload["error"]
	if !ok {
		return strings.TrimSpace(stringValue(payload["code"]))
	}
	return errorCodeFromValue(value)
}

func errorCodeFromValue(value any) string {
	errObj, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	return strings.TrimSpace(stringValue(errObj["code"]))
}

func UpstreamStatusForErrorCode(code string) int {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "rate_limit_exceeded":
		return http.StatusTooManyRequests
	case "insufficient_balance", "act-err-0":
		return http.StatusPaymentRequired
	case "invalid_api_key", "authentication_error", "unauthorized":
		return http.StatusUnauthorized
	case "insufficient_permissions", "permission_denied", "forbidden":
		return http.StatusForbidden
	case "model_not_found", "cluster_not_found", "not_found":
		return http.StatusNotFound
	case "invalid_request_error", "context_length_exceeded", "unsupported_feature", "unsupported_model", "model_task_mismatch":
		return http.StatusBadRequest
	default:
		return http.StatusServiceUnavailable
	}
}

// FriendlyUpstreamErrorMessage maps upstream status and error codes to text
// that is safe and actionable for end users.
func FriendlyUpstreamErrorMessage(status int, code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "insufficient_balance", "act-err-0":
		return "The model service balance is insufficient. Add funds or contact an administrator."
	case "rate_limit_exceeded":
		return "The model service is busy or its quota has been reached. Please try again later."
	case "invalid_api_key", "authentication_error", "unauthorized":
		return "Model service authentication failed. Check the credentials or contact an administrator."
	case "model_not_found", "cluster_not_found":
		return "The selected model does not exist. Choose another model or check the configuration."
	case "model_not_running", "model_unavailable", "required_upstream_unavailable", "response_route_unavailable":
		return "The selected model is temporarily unavailable. Try again later or choose another model."
	case "model_price_not_configured":
		return "The selected model is not fully configured. Contact an administrator or choose another model."
	case "unsupported_feature", "unsupported_model":
		return "The selected model does not support this feature. Adjust the request or choose another model."
	case "model_task_mismatch":
		return "This model task is not suitable for Agent conversations. Choose a text-generation or code model."
	case "invalid_response_id", "response_id_forbidden":
		return "The previous conversation cannot be continued. Start a new conversation and try again."
	case "content_policy_violation":
		return "The request did not pass the safety check. Change the content and try again."
	case "insufficient_permissions", "permission_denied", "forbidden":
		return "This account does not have permission for this operation. Contact an administrator."
	case "context_length_exceeded":
		return "The conversation exceeds this model's context length. Shorten it or start a new conversation."
	case "not_found":
		return "The requested resource does not exist or has expired. Check it and try again."
	case "video_not_ready":
		return "The video is still being generated. Please try again later."
	case "video_generation_failed":
		return "Video generation failed. Adjust the content and try again."
	case "video_generation_cancelled":
		return "Video generation was cancelled. Start a new request."
	case "invalid_request_error":
		return "The request or model configuration is invalid. Check it and try again."
	case "upstream_response_invalid", "internal_server_error", "internal_error", "moderation_error":
		return "The model service encountered a temporary error. Please try again later."
	}

	switch status {
	case http.StatusBadRequest:
		return "The request or model configuration is invalid. Check it and try again."
	case http.StatusUnauthorized:
		return "Model service authentication failed. Check the credentials or contact an administrator."
	case http.StatusPaymentRequired:
		return "The model service balance is insufficient. Add funds or contact an administrator."
	case http.StatusForbidden:
		return "This account cannot use the selected model. Contact an administrator or choose another model."
	case http.StatusNotFound:
		return "The selected model or requested resource does not exist. Check the configuration and try again."
	case http.StatusTooManyRequests:
		return "The model service is busy or its quota has been reached. Please try again later."
	default:
		if status >= http.StatusInternalServerError {
			return "The model service is temporarily unavailable. Please try again later."
		}
	}
	return "The model request failed. Please try again later."
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}
