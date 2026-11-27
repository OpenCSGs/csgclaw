package llm

import (
	"context"
	"csgclaw/internal/diagnostics"
	"csgclaw/internal/modelprovider"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

func modelRequestFailure(ctx context.Context, response *http.Response, body []byte, err error) diagnostics.ModelRequestStatus {
	status := diagnostics.ModelRequestStatus{State: "failed", Source: "client", Category: "connection"}
	exchange, _ := ctx.Value(modelExchangeKey{}).(*modelExchange)
	if exchange == nil {
		return status
	}
	status.RequestID = exchange.id
	if response != nil {
		status.GatewayRequestID = diagnostics.ProtectText(upstreamRequestID(response.Header))
		status.HTTPStatus = response.StatusCode
		status.Code = strings.ToLower(modelprovider.UpstreamErrorCode(body))
		if status.Code == "" && response.StatusCode == http.StatusRequestEntityTooLarge {
			status.Code = modelprovider.RequestTooLarge
		}
		status.Source, status.Category = "upstream", "server"
		if exchange.openCSG {
			status.Source = "gateway"
		}
		switch {
		case response.StatusCode == http.StatusTooManyRequests || temporaryRateLimit(status.Code):
			status.Category = "rate_limit"
		case response.StatusCode == http.StatusGatewayTimeout || strings.Contains(status.Code, "timeout"):
			status.Category = "timeout"
		case response.StatusCode >= 400 && response.StatusCode < 500:
			status.Category = "rejected"
		}
		switch {
		case strings.HasPrefix(status.Code, "upstream_") || status.Code == "required_upstream_unavailable":
			status.Source = "upstream"
		case strings.HasPrefix(status.Code, "model_") && status.Code != "model_cooldown", status.Code == "unsupported_model", status.Code == "unsupported_feature", status.Code == "context_length_exceeded":
			status.Source = "model"
		}
		status.Details = modelErrorDetails(body)
	}
	if err != nil {
		var timeout net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
			status.Category = "timeout"
		}
		status.Details = err.Error()
	}
	for _, secret := range exchange.secrets {
		if len(secret) >= 4 {
			status.Details = strings.ReplaceAll(status.Details, secret, "[redacted]")
		}
	}
	status.Details = diagnostics.ProtectText(status.Details)
	if len(status.Details) > 2048 {
		status.Details = strings.ToValidUTF8(status.Details[:2048], "") + "…"
	}
	return status
}

func modelErrorDetails(body []byte) string {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	if nested, ok := payload["response"].(map[string]any); ok {
		payload = nested
	}
	if nested, ok := payload["error"].(map[string]any); ok {
		payload = nested
	}
	// Response.failed can carry the complete conversation. Expose the original
	// error fields without copying inputs, outputs, tools, or upstream headers.
	fields := make(map[string]any, 4)
	for _, key := range []string{"code", "type", "param", "message"} {
		if value, ok := payload[key].(string); ok {
			fields[key] = value
		}
	}
	if len(fields) == 0 {
		return ""
	}
	return string(diagnostics.Protect(fields))
}

func reportModelRequestFailure(ctx context.Context, status diagnostics.ModelRequestStatus) {
	if status.RequestID == "" || ctx.Err() != nil {
		return
	}
	diagnostics.ReportModelRequest(ctx, status)
	slog.WarnContext(ctx, "model request failed", "request_id", status.RequestID, "gateway_request_id", status.GatewayRequestID,
		"trace_id", modelTraceID(ctx), "category", status.Category, "source", status.Source, "code", status.Code, "status", status.HTTPStatus)
}
