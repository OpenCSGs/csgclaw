package modelprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	CSGHubLiteProviderName   = "csghub-lite"
	CSGHubLiteDefaultBaseURL = "http://127.0.0.1:11435/v1"
	CSGHubLiteDefaultAPIKey  = "local"
)

type openAIModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

type openAIResponsesProbeResponse struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Status string `json:"status"`
}

var ErrResponsesAPIUnsupported = errors.New("responses API unsupported")

type ResponsesAPIStatusError struct {
	BaseURL    string
	Status     string
	StatusCode int
	Body       string
}

func (e *ResponsesAPIStatusError) Error() string {
	msg := fmt.Sprintf("request responses from %s: status %s", e.BaseURL, e.Status)
	if strings.TrimSpace(e.Body) != "" {
		msg += ": " + strings.TrimSpace(e.Body)
	}
	return msg
}

func (e *ResponsesAPIStatusError) Is(target error) bool {
	return target == ErrResponsesAPIUnsupported && (e.StatusCode == http.StatusNotFound || e.StatusCode == http.StatusMethodNotAllowed)
}

func ListOpenAIModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	return ListOpenAIModelsWithClient(ctx, &http.Client{Timeout: 2 * time.Second}, baseURL, apiKey, nil)
}

func ListOpenAIModelsWithClient(ctx context.Context, client *http.Client, baseURL, apiKey string, headers map[string]string) ([]string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openAIModelsURL(baseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("build models request: %w", err)
	}
	if apiKey = strings.TrimSpace(apiKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" || strings.EqualFold(key, "authorization") || strings.EqualFold(key, "content-type") {
			continue
		}
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request models from %s: %w", baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("request models from %s: status %s", baseURL, resp.Status)
	}

	var payload openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode models response from %s: %w", baseURL, err)
	}

	models := make([]string, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("no models returned from %s", baseURL)
	}
	return models, nil
}

func openAIModelsURL(baseURL string) string {
	modelsURL := baseURL + "/models"
	if !strings.Contains(strings.ToLower(modelsURL), "opencsg") {
		return modelsURL
	}
	// Temporary OpenCSG AIGateway pagination workaround. Remove this after
	// OpenCSG AIGateway returns the full model list without pagination.
	parsed, err := url.Parse(modelsURL)
	if err != nil {
		return modelsURL
	}
	query := parsed.Query()
	query.Set("per", "100")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func CheckResponsesAPI(ctx context.Context, baseURL, apiKey, modelID string, headers map[string]string) error {
	return CheckResponsesAPIWithClient(ctx, &http.Client{Timeout: 10 * time.Second}, baseURL, apiKey, modelID, headers)
}

func CheckResponsesAPIWithClient(ctx context.Context, client *http.Client, baseURL, apiKey, modelID string, headers map[string]string) error {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	modelID = strings.TrimSpace(modelID)
	if baseURL == "" {
		return fmt.Errorf("base URL is required")
	}
	if modelID == "" {
		return fmt.Errorf("model ID is required")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	payload := map[string]any{
		"model":             modelID,
		"input":             "ping",
		"store":             false,
		"stream":            false,
		"max_output_tokens": 16,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode responses probe request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build responses probe request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey = strings.TrimSpace(apiKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" || strings.EqualFold(key, "authorization") || strings.EqualFold(key, "content-type") {
			continue
		}
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request responses from %s: %w", baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &ResponsesAPIStatusError{
			BaseURL:    baseURL,
			Status:     resp.Status,
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(errBody)),
		}
	}

	var probe openAIResponsesProbeResponse
	if err := json.NewDecoder(resp.Body).Decode(&probe); err != nil {
		return fmt.Errorf("decode responses probe from %s: %w", baseURL, err)
	}
	if strings.TrimSpace(probe.Object) != "response" {
		return fmt.Errorf("responses probe from %s returned object %q, want response", baseURL, probe.Object)
	}
	return nil
}
