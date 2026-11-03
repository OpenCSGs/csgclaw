package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
)

// controlPlane normalizes business failures at the HTTP boundary. Successful
// responses, including followed logs, are written directly to the client.
// Authentication and protocol handlers retain their own response formats.
func controlPlane(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := &controlPlaneResponse{ResponseWriter: w}
		next(response, r)
		if response.status < http.StatusBadRequest {
			return
		}
		payload := map[string]any{}
		_ = json.Unmarshal(response.body.Bytes(), &payload)
		if payload == nil {
			payload = map[string]any{}
		}
		failure, _ := payload["error"].(map[string]any)
		if failure == nil {
			failure = map[string]any{}
			if message, ok := payload["error"].(string); ok {
				failure["message"] = message
			}
			for _, key := range []string{"code", "message"} {
				if value, ok := payload[key]; ok {
					failure[key] = value
					delete(payload, key)
				}
			}
		}
		if code, _ := failure["code"].(string); code == "" {
			failure["code"] = controlPlaneErrorCode(response.status)
		}
		if message, _ := failure["message"].(string); message == "" {
			message = strings.TrimSpace(response.body.String())
			if message == "" {
				message = http.StatusText(response.status)
			}
			failure["message"] = message
		}
		payload["error"] = failure
		w.Header().Del("Content-Length")
		writeJSON(w, response.status, payload)
	}
}

type controlPlaneResponse struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *controlPlaneResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *controlPlaneResponse) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	if status < http.StatusOK {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	w.status = status
	if status < http.StatusBadRequest {
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *controlPlaneResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if w.status >= http.StatusBadRequest {
		return w.body.Write(data)
	}
	return w.ResponseWriter.Write(data)
}

func (w *controlPlaneResponse) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if w.status < http.StatusBadRequest {
		_ = http.NewResponseController(w.ResponseWriter).Flush()
	}
}

func controlPlaneErrorCode(status int) string {
	switch status {
	case 400:
		return "invalid_request"
	case 401:
		return "unauthorized"
	case 403:
		return "forbidden"
	case 404:
		return "not_found"
	case 405:
		return "method_not_allowed"
	case 409:
		return "conflict"
	case 410:
		return "gone"
	case 413:
		return "payload_too_large"
	case 415:
		return "unsupported_media_type"
	case 422:
		return "unprocessable_entity"
	case 429:
		return "rate_limited"
	case 501:
		return "not_implemented"
	case 502:
		return "bad_gateway"
	case 503:
		return "service_unavailable"
	case 504:
		return "gateway_timeout"
	default:
		return "internal_error"
	}
}
