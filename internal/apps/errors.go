package apps

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ConnectionError exposes a safe cause and action, never upstream response text.
type ConnectionError struct {
	Code           string
	Message        string
	HTTPStatus     int
	Authentication bool
}

func (e *ConnectionError) Error() string { return e.Message }
func (e *ConnectionError) Is(target error) bool {
	return target == errAuthentication && e.Authentication
}
func connectionError(code, message string, status int, authentication bool) *ConnectionError {
	return &ConnectionError{Code: code, Message: message, HTTPStatus: status, Authentication: authentication}
}
func expiredPlatformToken(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		Expires float64 `json:"exp"`
	}
	return json.Unmarshal(data, &claims) == nil && claims.Expires > 0 && float64(time.Now().Unix()) >= claims.Expires
}
func setConnectionError(item *Installation, err error) {
	item.Status = "error"
	item.LastError = err.Error()
	item.LastErrorCode = ""
	item.LastErrorHTTPStatus = 0
	var detail *ConnectionError
	if errors.As(err, &detail) {
		item.LastErrorCode = detail.Code
		item.LastErrorHTTPStatus = detail.HTTPStatus
		if detail.Authentication {
			item.Status = "authorization_required"
		}
	}
	if errors.Is(err, errAuthentication) {
		item.Status = "authorization_required"
	}
}
func upstreamHTTPError(status int, body []byte, login bool) *ConnectionError {
	switch status {
	case 401:
		if login {
			return connectionError("app_opencsg_login_rejected", "OpenCSG rejected the current login (HTTP 401). Sign in again in Settings and reconnect the App.", status, true)
		}
		return connectionError("app_platform_unauthorized", "MCP platform authentication failed (HTTP 401). Update the platform token or reuse your current OpenCSG login.", status, true)
	case 403:
		return connectionError("app_platform_access_denied", "MCP platform access denied (HTTP 403). Check this account's access to the service and its token permissions.", status, true)
	case 404:
		return connectionError("app_mcp_not_found", "MCP endpoint not found (HTTP 404). Check the service URL and MCP path.", status, false)
	default:
		var response struct {
			Message string `json:"msg"`
		}
		_ = json.Unmarshal(body, &response)
		if status >= 500 && strings.Contains(response.Message, "endpoint of deploy") && strings.Contains(response.Message, "is empty") {
			return connectionError("app_mcp_backend_unavailable", "MCP service has no running backend. It may be sleeping or stopped; start the service and reconnect.", status, false)
		}
		return connectionError("app_mcp_http_error", fmt.Sprintf("MCP service returned HTTP %d. Check the upstream service and reconnect.", status), status, false)
	}
}
