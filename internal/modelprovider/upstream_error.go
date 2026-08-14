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
			return http.StatusGatewayTimeout, "upstream_timeout", "连接模型服务超时，请稍后重试。", true
		}
		return http.StatusBadGateway, "upstream_unavailable", "无法连接模型服务，请检查服务配置或稍后重试。", true
	}
	status = statusErr.StatusCode
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	return status, statusErr.Code(), statusErr.UserMessage(), true
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
		return "模型服务余额不足，请充值或联系管理员后重试。"
	case "rate_limit_exceeded":
		return "当前请求较多或使用额度已达上限，请稍后再试。"
	case "invalid_api_key", "authentication_error", "unauthorized":
		return "模型服务认证失败，请检查密钥配置或联系管理员。"
	case "model_not_found", "cluster_not_found":
		return "当前选择的模型不存在，请更换模型或检查模型配置。"
	case "model_not_running", "model_unavailable", "required_upstream_unavailable", "response_route_unavailable":
		return "当前模型暂时不可用，请稍后重试或更换模型。"
	case "model_price_not_configured":
		return "当前模型尚未完成服务配置，请联系管理员或更换模型。"
	case "unsupported_feature", "unsupported_model":
		return "当前模型不支持这项功能，请调整请求或更换模型。"
	case "model_task_mismatch":
		return "当前模型的任务类型不适用于 Agent 对话，请选择文本生成或代码模型。"
	case "invalid_response_id", "response_id_forbidden":
		return "无法继续之前的对话，请新建对话后重试。"
	case "content_policy_violation":
		return "请求内容未通过安全检查，请修改内容后重试。"
	case "insufficient_permissions", "permission_denied", "forbidden":
		return "当前账号没有执行此操作的权限，请联系管理员。"
	case "context_length_exceeded":
		return "对话内容超过当前模型的上下文长度，请缩短对话或新建对话后重试。"
	case "not_found":
		return "请求的资源不存在或已失效，请确认后重试。"
	case "video_not_ready":
		return "视频仍在生成中，请稍后再试。"
	case "video_generation_failed":
		return "视频生成失败，请调整内容后重试。"
	case "video_generation_cancelled":
		return "视频生成任务已取消，请重新发起。"
	case "invalid_request_error":
		return "请求内容或模型配置有误，请检查后重试。"
	case "upstream_response_invalid", "internal_server_error", "internal_error", "moderation_error":
		return "模型服务暂时出现异常，请稍后重试。"
	}

	switch status {
	case http.StatusBadRequest:
		return "请求内容或模型配置有误，请检查后重试。"
	case http.StatusUnauthorized:
		return "模型服务认证失败，请检查密钥配置或联系管理员。"
	case http.StatusPaymentRequired:
		return "模型服务余额不足，请充值或联系管理员后重试。"
	case http.StatusForbidden:
		return "当前账号无权使用该模型，请联系管理员或更换模型。"
	case http.StatusNotFound:
		return "当前模型或请求的资源不存在，请检查配置后重试。"
	case http.StatusTooManyRequests:
		return "当前请求较多或使用额度已达上限，请稍后再试。"
	default:
		if status >= http.StatusInternalServerError {
			return "模型服务暂时不可用，请稍后重试。"
		}
	}
	return "模型请求失败，请稍后重试。"
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
