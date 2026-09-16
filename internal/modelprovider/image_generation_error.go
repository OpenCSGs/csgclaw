package modelprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode"
)

// ImageGenerationError contains bounded, credential-redacted provider diagnostics.
// It never retains the raw response body, request headers, or provider URL.
type ImageGenerationError struct {
	HTTPStatus int    `json:"http_status,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message"`
	Stage      string `json:"stage,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
}

func (e *ImageGenerationError) Error() string {
	if e.HTTPStatus > 0 {
		return fmt.Sprintf("image generation failed (HTTP %d, %s): %s", e.HTTPStatus, e.Code, e.Message)
	}
	return e.Message
}

var imageErrorSecretPattern = regexp.MustCompile(`(?i)(bearer\s+|(?:api[_-]?key|access[_-]?token|refresh[_-]?token|authorization|secret|password)["']?\s*[:=]\s*["']?)[^\s"',;&]+`)
var imageErrorAPIKeyPattern = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]+`)

func imageErrorText(value string, secrets []string, maxRunes int) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	value = imageErrorSecretPattern.ReplaceAllString(value, "${1}[redacted]")
	value = imageErrorAPIKeyPattern.ReplaceAllString(value, "[redacted]")
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, value)
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "…"
	}
	return string(runes)
}

func imageResponseError(response *http.Response, body []byte, apiKey string, headers map[string]string) *ImageGenerationError {
	secrets := []string{apiKey}
	for _, value := range headers {
		secrets = append(secrets, value)
	}
	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   struct {
			Code       string `json:"code"`
			Message    string `json:"message"`
			RequestID  string `json:"request_id"`
			Moderation struct {
				Stage string `json:"moderation_stage"`
			} `json:"moderation_details"`
		} `json:"error"`
		RequestID string `json:"request_id"`
	}
	_ = json.Unmarshal(body, &payload)
	if payload.Error.Code == "" {
		payload.Error.Code = payload.Code
	}
	if payload.Error.Message == "" {
		payload.Error.Message = payload.Message
	}
	code := imageErrorText(payload.Error.Code, secrets, 128)
	if code == "" {
		code = FallbackUpstreamErrorCode(response.StatusCode)
	}
	message := imageErrorText(payload.Error.Message, secrets, 2048)
	if message == "" {
		message = FriendlyUpstreamErrorMessage(response.StatusCode, code)
	}
	requestID := response.Header.Get("x-request-id")
	if requestID == "" {
		requestID = payload.Error.RequestID
	}
	if requestID == "" {
		requestID = payload.RequestID
	}
	stage := payload.Error.Moderation.Stage
	if stage != "input" && stage != "output" {
		stage = ""
	}
	return &ImageGenerationError{HTTPStatus: response.StatusCode, Code: code, Message: message, Stage: stage, RequestID: imageErrorText(requestID, secrets, 128)}
}

func imageTransportError(err error) *ImageGenerationError {
	if errors.Is(err, context.Canceled) {
		return &ImageGenerationError{Code: "image_generation_canceled", Message: "Image generation was canceled."}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &ImageGenerationError{Code: "upstream_timeout", Message: "The image service request timed out. Please try again later."}
	}
	return &ImageGenerationError{Code: "upstream_unavailable", Message: "Could not connect to the image service. Check its connection and try again."}
}
