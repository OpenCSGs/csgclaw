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
		r.Header.Set("x-api-key", c.APIKey)
		r.Header.Set("anthropic-version", "2023-06-01")
	} else {
		r.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	return r, nil
}

// CheckService falls back to documented model IDs only when discovery is unsupported.
// A successful inference is required before documented/manual candidates become a connected service.
func CheckService(ctx context.Context, c ServiceConnection, verify bool) (ServiceDirectory, error) {
	if c.APIKey == "" {
		return ServiceDirectory{}, &ConnectionError{401, "invalid_api_key", "API Key is required."}
	}
	client := &http.Client{Timeout: 20 * time.Second}
	request, err := c.Request(ctx, http.MethodGet, "/models", nil)
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
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var list openAIModelsResponse
		if json.Unmarshal(body, &list) != nil {
			return out, &ConnectionError{502, "invalid_response", "The model service returned an invalid model list."}
		}
		complete := list
		// Anthropic uses cursor pagination, so never silently truncate its model directory.
		for list.HasMore && len(list.Data) > 0 {
			last := list.Data[len(list.Data)-1].ID
			req, e := c.Request(ctx, http.MethodGet, "/models", nil)
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
			if json.Unmarshal(b, &list) != nil {
				return out, &ConnectionError{502, "invalid_response", "Invalid paginated model list."}
			}
			complete.Data = append(complete.Data, list.Data...)
			if len(list.Data) > 0 && list.Data[len(list.Data)-1].ID == last {
				return out, &ConnectionError{502, "invalid_response", "Model pagination did not advance."}
			}
		}
		if len(complete.Data) > 0 {
			directory, e := openAIModelDirectoryFromResponse(complete, c.BaseURL, false)
			if e != nil {
				return out, &ConnectionError{502, "invalid_response", "The model service returned no usable models."}
			}
			out.ModelDiscoveryResult = directory
		}

	} else if resp.StatusCode != 404 && resp.StatusCode != 405 && resp.StatusCode != 501 {
		return out, serviceError(resp.StatusCode, body, c.APIKey)
	}
	chatModels := out.Models[:0]
	for _, m := range out.Models {
		lower := strings.ToLower(m)
		if IsImageGenerationModel(m) || strings.Contains(lower, "embedding") || strings.Contains(lower, "tts") || strings.Contains(lower, "whisper") || strings.Contains(lower, "audio") || strings.Contains(lower, "transcrib") || strings.Contains(lower, "moderation") || strings.HasPrefix(lower, "sora") {
			continue
		}
		chatModels = append(chatModels, m)
	}
	out.Models = chatModels

	if len(out.Models) == 0 {
		out.Models = append([]string(nil), c.Models...)
		out.Source = "manual"
		if len(out.Models) == 0 {
			if p, ok := modelpreset.Find(c.Preset); ok {
				out.Models = append([]string(nil), p.Models...)
				out.Source = "preset"
			}
		}
		if len(out.Models) == 0 {
			return out, &ConnectionError{400, "models_required", "This service does not list models. Add a model ID in advanced settings."}
		}
		verify = true
	}
	if verify {
		probeModel := out.Models[0]
		preferred := append([]string{}, c.Models...)
		if p, ok := modelpreset.Find(c.Preset); ok {
			preferred = append(preferred, p.Models...)
		}
		for _, model := range out.Models {
			if strings.HasPrefix(model, "gpt-") || strings.HasPrefix(model, "claude-") || strings.HasPrefix(model, "kimi-") || strings.HasPrefix(model, "glm-") || strings.HasPrefix(model, "qwen") || strings.HasPrefix(model, "mimo-") || strings.HasPrefix(model, "MiniMax-") || strings.HasPrefix(model, "deepseek-") {
				preferred = append(preferred, model)
			}
		}
		for _, want := range preferred {
			found := false
			for _, m := range out.Models {
				if m == want {
					probeModel = m
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		ordered := []string{probeModel}
		for _, m := range out.Models {
			if m != probeModel {
				ordered = append(ordered, m)
			}
		}
		out.Models = ordered

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
			return out, e
		}
		r, e := client.Do(req)
		if e != nil {
			return out, connectionNetworkError(e)
		}
		b, e := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		r.Body.Close()
		if e != nil {
			return out, e
		}
		if r.StatusCode < 200 || r.StatusCode >= 300 {
			return out, serviceError(r.StatusCode, b, c.APIKey)
		}
		var decoded map[string]json.RawMessage
		if json.Unmarshal(b, &decoded) != nil || len(decoded) == 0 || decoded["error"] != nil {
			return out, &ConnectionError{502, "invalid_response", "The model service returned an invalid inference response."}
		}
		if c.Protocol == modelpreset.Responses && decoded["output"] == nil || c.Protocol == modelpreset.AnthropicMessages && decoded["content"] == nil || (c.Protocol == "" || c.Protocol == modelpreset.ChatCompletions) && decoded["choices"] == nil {
			return out, &ConnectionError{502, "invalid_response", "The model service response does not match the selected protocol."}
		}
	}
	return out, nil
}

func connectionNetworkError(err error) error {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) && netErr.Timeout() {
		return &ConnectionError{504, "timeout", "The model service timed out. Check the selected region and network."}
	}
	return &ConnectionError{502, "network_error", fmt.Sprintf("Cannot reach the model service (%s). Check the selected region and network.", http.StatusText(http.StatusBadGateway))}
}
