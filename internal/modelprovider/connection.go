package modelprovider

import (
	"bytes"
	"context"
	"csgclaw/internal/modelpreset"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

type ServiceConnection struct {
	BaseURL, APIKey, Protocol, Preset string
	Headers                           map[string]string
	Models                            []string
}

type ServiceDirectory struct {
	ModelDiscoveryResult
	Source string
}

type ConnectionError struct {
	Status        int
	Code, Message string
}

func (e *ConnectionError) Error() string { return e.Message }
func ConnectionErrorCode(err error) string {
	var e *ConnectionError
	if errors.As(err, &e) {
		return e.Code
	}
	return "network_error"
}

func serviceError(status int, body []byte, key string) error {
	code := "upstream_error"
	message := "The model service rejected the request. Check the selected service and region."
	switch status {
	case 400, 422:
		code, message = "invalid_inference_request", "The model service rejected the connection check. Check the protocol and model ID."
	case 401:
		code, message = "invalid_api_key", "API Key is invalid for this service or region."
	case 402:
		code, message = "insufficient_quota", "The account has insufficient balance or plan quota."
	case 403:
		code, message = "permission_denied", "This Key does not have access to the selected service, region, or model."
	case 404:
		code, message = "model_not_found", "The endpoint or model is unavailable. Check the service, region, and model."
	case 429:
		code, message = "rate_limit", "The service is rate limited or the plan quota is exhausted."
	}
	// Classify quota failures without exposing upstream bodies that can echo credentials.
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "insufficient") || strings.Contains(lower, "quota") || strings.Contains(lower, "balance") || strings.Contains(lower, "额度") || strings.Contains(lower, "余额") {
		code, message = "insufficient_quota", "The account has insufficient balance or plan quota."
	}
	return &ConnectionError{status, code, strings.ReplaceAll(message, key, "[redacted]")}
}

func (c ServiceConnection) Request(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	r, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("User-Agent", "CSGClaw/1.0")
	for k, v := range c.Headers {
		if !strings.EqualFold(k, "authorization") && !strings.EqualFold(k, "x-api-key") && !strings.EqualFold(k, "api-key") {
			r.Header.Set(k, v)
		}
	}
	if c.Protocol == modelpreset.AnthropicMessages {
		if messagesUsesBearer(r.URL.Hostname()) {
			r.Header.Set("Authorization", "Bearer "+c.APIKey)
		} else {
			r.Header.Set("x-api-key", c.APIKey)
		}
		r.Header.Set("anthropic-version", "2023-06-01")
	} else {
		r.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	return r, nil
}

func messagesUsesBearer(host string) bool {
	host = strings.ToLower(host)
	if strings.HasSuffix(host, ".xiaomimimo.com") {
		return true
	}
	switch host {
	case "api.moonshot.cn", "api.moonshot.ai", "api.kimi.com", "api.kimi.ai", "ark.cn-beijing.volces.com",
		"api.minimax.cn", "api.minimax.io":
		return true
	default:
		return false
	}
}

// Use the service's catalog endpoint when the selected protocol has a distinct route.
func catalogConnection(c ServiceConnection) ServiceConnection {
	if preset, ok := modelpreset.Find(c.Preset); ok && c.Protocol != preset.Protocol {
		for _, region := range preset.Regions {
			if url := region.ProtocolURLs[c.Protocol]; url != "" && strings.EqualFold(strings.TrimRight(c.BaseURL, "/"), url) {
				c.BaseURL, c.Protocol = region.BaseURL, preset.Protocol
				break
			}
		}
	}
	return c
}

// IsZhipuResponsesURL identifies the documented native Responses base URL.
func IsZhipuResponsesURL(base string) bool {
	parsed, err := url.Parse(base)
	if err != nil || strings.TrimRight(parsed.Path, "/") != "/api/v1" {
		return false
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "open.bigmodel.cn", "api.z.ai":
		return true
	default:
		return false
	}
}

// Only use vendor model lists for the official endpoint they describe.
func documentedResponsesModels(c ServiceConnection) []string {
	if c.Protocol != modelpreset.Responses {
		return nil
	}
	switch c.Preset {
	case "kimi", "deepseek", "mimo", "mimo-plan", "zhipu":
	default:
		return nil
	}
	preset, ok := modelpreset.Find(c.Preset)
	if !ok {
		return nil
	}
	for _, region := range preset.Regions {
		if endpoint := region.ProtocolURLs[modelpreset.Responses]; endpoint != "" &&
			strings.EqualFold(endpoint, strings.TrimRight(c.BaseURL, "/")) {
			if preset.ID == "zhipu" {
				return []string{"glm-5.3"}
			}
			return preset.Models
		}
	}
	return nil
}

// ZhipuResponsesGatewayStatus normalizes HTTP 200 business errors.
func ZhipuResponsesGatewayStatus(baseURL string, status int, body []byte) int {
	if status != http.StatusOK || !IsZhipuResponsesURL(baseURL) {
		return status
	}
	var gateway struct {
		Success *bool           `json:"success"`
		Code    json.RawMessage `json:"code"`
	}
	if json.Unmarshal(body, &gateway) != nil || gateway.Success == nil || *gateway.Success {
		return status
	}
	switch strings.Trim(string(gateway.Code), "\"") {
	case "401", "1001", "1002", "1003", "1004", "1005":
		return http.StatusUnauthorized
	default:
		return http.StatusBadRequest
	}
}

// CheckService falls back to documented model IDs only when discovery is unsupported.
// A successful inference is required before documented/manual candidates become a connected service.
func CheckService(ctx context.Context, c ServiceConnection, verify bool) (ServiceDirectory, error) {
	if c.APIKey == "" {
		return ServiceDirectory{}, &ConnectionError{401, "invalid_api_key", "API Key is required."}
	}
	client := &http.Client{Timeout: 20 * time.Second}
	directoryConnection := catalogConnection(c)
	request, err := directoryConnection.Request(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return ServiceDirectory{}, &ConnectionError{400, "invalid_endpoint", "Invalid model service URL."}
	}
	resp, err := client.Do(request)
	if err != nil {
		return ServiceDirectory{}, connectionNetworkError(err)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	resp.Body.Close()
	if err != nil {
		return ServiceDirectory{}, connectionNetworkError(err)
	}
	out := ServiceDirectory{Source: "upstream"}
	discovered := false
	isArk := c.Preset == "volcengine" || c.Preset == "volcengine-coding" || c.Preset == "volcengine-agent"
	protocolSupport := make(map[string]bool)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var list openAIModelsResponse
		if json.Unmarshal(body, &list) != nil || list.Data == nil {
			return out, &ConnectionError{502, "invalid_response", "The model service returned an invalid model list."}
		}
		complete := list
		// Anthropic uses cursor pagination, so never silently truncate its model directory.
		for list.HasMore && len(list.Data) > 0 {
			last := list.Data[len(list.Data)-1].ID
			req, e := directoryConnection.Request(ctx, http.MethodGet, "/models", nil)
			if e != nil {
				return out, e
			}
			q := req.URL.Query()
			q.Set("after_id", last)
			req.URL.RawQuery = q.Encode()
			r, e := client.Do(req)
			if e != nil {
				return out, connectionNetworkError(e)
			}
			b, e := io.ReadAll(io.LimitReader(r.Body, 4<<20))
			r.Body.Close()
			if e != nil {
				return out, e
			}
			if r.StatusCode != 200 {
				return out, serviceError(r.StatusCode, b, c.APIKey)
			}
			list.Data = nil
			list.HasMore = false
			if json.Unmarshal(b, &list) != nil || list.Data == nil {
				return out, &ConnectionError{502, "invalid_response", "Invalid paginated model list."}
			}
			complete.Data = append(complete.Data, list.Data...)
			if len(list.Data) > 0 && list.Data[len(list.Data)-1].ID == last {
				return out, &ConnectionError{502, "invalid_response", "Model pagination did not advance."}
			}
		}
		directory, err := openAIModelDirectoryFromResponse(complete, c.BaseURL, false)
		if err != nil {
			return out, &ConnectionError{502, "invalid_response", "The model service returned no usable models."}
		}
		if isArk {
			for _, model := range complete.Data {
				if supported, known := arkModelSupportsProtocol(model.APISupported, c.Protocol); known {
					protocolSupport[model.ID] = supported
				}
			}
		}
		out.ModelDiscoveryResult = directory
		discovered = true

	} else if resp.StatusCode != 404 && resp.StatusCode != 405 && resp.StatusCode != 501 {
		return out, serviceError(resp.StatusCode, body, c.APIKey)
	}
	documented := documentedResponsesModels(c)
	chatModels := out.Models[:0]
	mismatched := false
	for _, m := range out.Models {
		lower := strings.ToLower(m)
		if IsImageGenerationModel(m) || strings.Contains(lower, "embedding") || strings.Contains(lower, "tts") || strings.Contains(lower, "whisper") || strings.Contains(lower, "-asr") || strings.Contains(lower, "_asr") || strings.Contains(lower, "audio") || strings.Contains(lower, "transcrib") || strings.Contains(lower, "moderation") || strings.HasPrefix(lower, "sora") {
			continue
		}
		if len(documented) > 0 && !slices.Contains(documented, m) {
			mismatched = true
			continue
		}
		// Older Ark 3D entries can omit both task and capability fields.
		if isArk && (strings.HasPrefix(lower, "hitem3d-") || strings.HasPrefix(lower, "hyper3d-")) {
			continue
		}
		if supported, known := protocolSupport[m]; known && !supported {
			continue
		}
		chatModels = append(chatModels, m)
	}
	out.Models = chatModels

	// An authenticated directory can describe a valid generation-only service.
	// Do not replace its media catalog with chat presets or issue a paid image/
	// video generation request merely to verify the connection.
	if len(out.Models) == 0 && (!isArk || len(c.Models) == 0) && (len(out.ImageModels) > 0 || len(out.VideoModels) > 0) {
		return out, nil
	}
	// A read-only discovery check should reflect an authoritative empty catalog.
	if discovered && !verify && len(out.Models) == 0 {
		return out, nil
	}
	if len(out.Models) == 0 {
		if mismatched && len(c.Models) == 0 {
			return out, &ConnectionError{400, "model_not_found", "No documented Responses models are available in this service's model list."}
		}
		out.Models = append([]string(nil), c.Models...)
		out.Source = "manual"
		if len(out.Models) == 0 {
			if p, ok := modelpreset.Find(c.Preset); ok {
				out.Models = append([]string(nil), p.Models...)
				if len(documented) > 0 {
					out.Models = documented
				}
				out.Source = "preset"
			}
		}
		if len(out.Models) == 0 {
			return out, &ConnectionError{400, "models_required", "This service does not list models. Add a model ID in advanced settings."}
		}
		verify = true
	}
	if !verify {
		return out, nil
	}

	preferred := append([]string(nil), c.Models...)
	if c.Protocol == modelpreset.Responses && IsZhipuResponsesURL(c.BaseURL) && slices.Contains(out.Models, "glm-5.3") {
		preferred = append([]string{"glm-5.3"}, preferred...)
	}
	if p, ok := modelpreset.Find(c.Preset); ok {
		preferred = append(preferred, p.Models...)
	}
	if isArk {
		// User-created endpoints are not returned by the public model directory.
		for _, model := range c.Models {
			if strings.HasPrefix(model, "ep-") && !slices.Contains(out.Models, model) {
				out.Models = append(out.Models, model)
			}
		}
		for _, model := range out.Models {
			if protocolSupport[model] {
				preferred = append(preferred, model)
			}
		}
	}
	for _, model := range out.Models {
		if strings.HasPrefix(model, "gpt-") || strings.HasPrefix(model, "claude-") || strings.HasPrefix(model, "kimi-") || strings.HasPrefix(model, "glm-") || strings.HasPrefix(model, "qwen") || strings.HasPrefix(model, "mimo-") || strings.HasPrefix(model, "MiniMax-") || strings.HasPrefix(model, "deepseek-") {
			preferred = append(preferred, model)
		}
	}
	candidates := make([]string, 0, len(out.Models))
	for _, model := range append(preferred, out.Models...) {
		supported, declared := protocolSupport[model]
		if (!declared || supported) && slices.Contains(out.Models, model) && !slices.Contains(candidates, model) {
			candidates = append(candidates, model)
		}
	}
	if len(candidates) == 0 {
		return out, arkModelUnavailable()
	}

	// A visible Ark model can still require account activation. Only skip that
	// specific failure, and bound the number of inference requests.
	attempts := 1
	if isArk {
		attempts = min(3, len(candidates))
	}
	for _, model := range candidates[:attempts] {
		if err := probeServiceModel(ctx, client, c, model, isArk); err != nil {
			if isArk && ConnectionErrorCode(err) == "ark_model_unavailable" {
				continue
			}
			return out, err
		}
		ordered := []string{model}
		for _, id := range out.Models {
			if id != model {
				ordered = append(ordered, id)
			}
		}
		out.Models = ordered
		return out, nil
	}
	return out, arkModelUnavailable()
}

// Ark returns the supported route names; an absent route in a populated
// capability map is unavailable. Older entries may omit the map altogether.
func arkModelSupportsProtocol(raw json.RawMessage, protocol string) (bool, bool) {
	var apis map[string]*bool
	if json.Unmarshal(raw, &apis) != nil || len(apis) == 0 {
		return false, false
	}
	names := []string{"chat/completions"}
	switch protocol {
	case modelpreset.Responses:
		names = []string{"responses", "responsesapi", "responses_api"}
	case modelpreset.AnthropicMessages:
		names = []string{"messages"}
	}
	for api, enabled := range apis {
		if slices.Contains(names, strings.ToLower(api)) && enabled != nil {
			return *enabled, true
		}
	}
	return false, true
}

func arkModelUnavailable() *ConnectionError {
	return &ConnectionError{http.StatusNotFound, "ark_model_unavailable", "Activate a model in the Ark console or add an enabled inference endpoint ID in Advanced settings."}
}

func probeServiceModel(ctx context.Context, client *http.Client, c ServiceConnection, probeModel string, ark bool) error {
	path := "/chat/completions"
	payload := map[string]any{"model": probeModel, "messages": []map[string]string{{"role": "user", "content": "Reply OK."}}, "max_tokens": 32}
	if c.Protocol == modelpreset.Responses {
		path = "/responses"
		payload = map[string]any{"model": probeModel, "input": "Reply OK.", "max_output_tokens": 32}
	}
	if c.Protocol == modelpreset.AnthropicMessages {
		path = "/messages"
	}
	encoded, _ := json.Marshal(payload)
	req, e := c.Request(ctx, http.MethodPost, path, encoded)
	if e != nil {
		return e
	}
	r, e := client.Do(req)
	if e != nil {
		return connectionNetworkError(e)
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	r.Body.Close()
	if e != nil {
		return e
	}
	status := ZhipuResponsesGatewayStatus(c.BaseURL, r.StatusCode, b)
	if status < 200 || status >= 300 {
		if c.Protocol == modelpreset.Responses && ResponsesEndpointUnsupported(status, b) {
			return &ConnectionError{status, "unsupported_protocol", "This endpoint does not support OpenAI Responses. Select a supported protocol or correct the endpoint URL."}
		}
		if ark && r.StatusCode == http.StatusNotFound {
			switch UpstreamErrorCode(b) {
			case "ModelNotOpen", "InvalidEndpointOrModel.NotFound":
				return arkModelUnavailable()
			case "InvalidEndpointOrModel.ModelIDAccessDisabled":
				return &ConnectionError{http.StatusNotFound, "ark_endpoint_required", "This Ark account requires an inference endpoint ID. Add its ep- ID in Advanced settings."}
			}
		}
		return serviceError(status, b, c.APIKey)
	}
	var decoded map[string]json.RawMessage
	decodeErr := json.Unmarshal(b, &decoded)
	// Successful Responses payloads can explicitly include "error": null.
	upstreamError := decoded["error"]
	if decodeErr != nil || len(decoded) == 0 || (upstreamError != nil && !bytes.Equal(upstreamError, []byte("null"))) {
		return &ConnectionError{502, "invalid_response", "The model service returned an invalid inference response."}
	}
	field := "choices"
	switch c.Protocol {
	case modelpreset.Responses:
		field = "output"
		var status string
		_ = json.Unmarshal(decoded["status"], &status)
		if status == "failed" || status == "cancelled" {
			return &ConnectionError{502, "invalid_inference_request", "The model could not complete the connection check."}
		}
	case modelpreset.AnthropicMessages:
		field = "content"
	}
	if value := decoded[field]; len(value) == 0 || value[0] != '[' {
		return &ConnectionError{502, "invalid_response", "The model service response does not match the selected protocol."}
	}
	return nil
}

func connectionNetworkError(err error) error {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) && netErr.Timeout() {
		return &ConnectionError{504, "timeout", "The model service timed out. Check the selected region and network."}
	}
	return &ConnectionError{502, "network_error", fmt.Sprintf("Cannot reach the model service (%s). Check the selected region and network.", http.StatusText(http.StatusBadGateway))}
}
