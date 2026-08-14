package modelprovider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type openAIModelsResponse struct {
	Data []struct {
		ID           string `json:"id"`
		Task         any    `json:"task"`
		Availability *struct {
			IsAvailable *bool `json:"is_available"`
		} `json:"availability"`
	} `json:"data"`
}

type openAIResponsesProbeResponse struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Status string `json:"status"`
}

type openAIChatCompletionsProbeResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"message"`
		FinishReason any `json:"finish_reason"`
	} `json:"choices"`
}

var ErrResponsesAPIUnsupported = errors.New("responses API unsupported")

type ResponsesAPIStatusError struct {
	Operation  string
	BaseURL    string
	Status     string
	StatusCode int
	Body       string
}

func (e *ResponsesAPIStatusError) Error() string {
	operation := strings.TrimSpace(e.Operation)
	if operation == "" {
		operation = "responses"
	}
	msg := fmt.Sprintf("request %s from %s: status %s", operation, e.BaseURL, e.Status)
	if strings.TrimSpace(e.Body) != "" {
		msg += ": " + strings.TrimSpace(e.Body)
	}
	return msg
}

func (e *ResponsesAPIStatusError) Is(target error) bool {
	return target == ErrResponsesAPIUnsupported && strings.TrimSpace(e.Operation) != "chat completions" && (e.StatusCode == http.StatusNotFound || e.StatusCode == http.StatusMethodNotAllowed)
}

func (e *ResponsesAPIStatusError) Code() string {
	return UpstreamErrorCode([]byte(e.Body))
}

func (e *ResponsesAPIStatusError) UserMessage() string {
	return FriendlyUpstreamErrorMessage(e.StatusCode, e.Code())
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

	resp, err := requestOpenAIModels(ctx, client, baseURL+"/models", apiKey, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &ResponsesAPIStatusError{
			Operation:  "models",
			BaseURL:    baseURL,
			Status:     resp.Status,
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(errBody)),
		}
	}

	var payload openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, &UpstreamRequestError{Operation: "decode models response", BaseURL: baseURL, Err: err}
	}

	models := make([]string, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if item.Availability != nil && item.Availability.IsAvailable != nil && !*item.Availability.IsAvailable {
			continue
		}
		if taskPresent(item.Task) && !taskSupportsTextGeneration(item.Task) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}
	if len(models) == 0 {
		return nil, &UpstreamRequestError{Operation: "decode models response", BaseURL: baseURL, Err: errors.New("no models returned")}
	}
	return models, nil
}

func taskPresent(task any) bool {
	switch value := task.(type) {
	case string:
		return strings.TrimSpace(value) != ""
	case []any:
		return len(value) > 0
	default:
		return task != nil
	}
}

func taskSupportsTextGeneration(task any) bool {
	values := make([]string, 0, 2)
	switch value := task.(type) {
	case string:
		values = strings.Split(value, ",")
	case []any:
		for _, entry := range value {
			if text, ok := entry.(string); ok {
				values = append(values, text)
			}
		}
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), "text-generation") {
			return true
		}
	}
	return false
}

func requestOpenAIModels(ctx context.Context, client *http.Client, modelsURL, apiKey string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
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
		return nil, &UpstreamRequestError{Operation: "request models", BaseURL: modelsURL, Err: err}
	}
	return resp, nil
}

func CheckResponsesAPI(ctx context.Context, baseURL, apiKey, modelID string, headers map[string]string) error {
	return CheckResponsesAPIWithClient(ctx, &http.Client{Timeout: 10 * time.Second}, baseURL, apiKey, modelID, headers)
}

func CheckResponsesOrChatCompletionsAPI(ctx context.Context, baseURL, apiKey, modelID string, headers map[string]string) error {
	return CheckResponsesOrChatCompletionsAPIWithClient(ctx, &http.Client{Timeout: 10 * time.Second}, baseURL, apiKey, modelID, headers)
}

func CheckResponsesOrChatCompletionsAPIWithClient(ctx context.Context, client *http.Client, baseURL, apiKey, modelID string, headers map[string]string) error {
	err := CheckResponsesAPIWithClient(ctx, client, baseURL, apiKey, modelID, headers)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrResponsesAPIUnsupported) {
		return err
	}
	if chatErr := CheckChatCompletionsAPIWithClient(ctx, client, baseURL, apiKey, modelID, headers); chatErr != nil {
		return fmt.Errorf("responses API is unsupported and chat completions fallback is unavailable: %w", chatErr)
	}
	return nil
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
		"input":             "Reply with exactly: OK",
		"store":             false,
		"stream":            true,
		"max_output_tokens": 128,
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
	req.Header.Set("Accept", "text/event-stream")
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
		return &UpstreamRequestError{Operation: "request responses", BaseURL: baseURL, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &ResponsesAPIStatusError{
			Operation:  "responses",
			BaseURL:    baseURL,
			Status:     resp.Status,
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(errBody)),
		}
	}

	var probe openAIResponsesProbeResponse
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if mediaType == "text/event-stream" {
		probe, err = decodeOpenAIResponsesProbeStream(resp.Body)
	} else {
		err = json.NewDecoder(resp.Body).Decode(&probe)
	}
	if err != nil {
		return &UpstreamRequestError{Operation: "decode responses probe", BaseURL: baseURL, Err: err}
	}
	if strings.TrimSpace(probe.Object) != "response" {
		return &UpstreamRequestError{Operation: "decode responses probe", BaseURL: baseURL, Err: fmt.Errorf("returned object %q, want response", probe.Object)}
	}
	if strings.TrimSpace(probe.Status) != "completed" {
		return &UpstreamRequestError{Operation: "decode responses probe", BaseURL: baseURL, Err: fmt.Errorf("returned status %q, want completed", probe.Status)}
	}
	return nil
}

func decodeOpenAIResponsesProbeStream(r io.Reader) (openAIResponsesProbeResponse, error) {
	var completed openAIResponsesProbeResponse
	found, err := scanOpenAIProbeSSE(r, func(sseEventType string, data string) (bool, error) {
		if data == "" || data == "[DONE]" {
			return false, nil
		}
		var event struct {
			Type     string                       `json:"type"`
			Response openAIResponsesProbeResponse `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return false, err
		}
		eventType := strings.TrimSpace(event.Type)
		if eventType == "" {
			eventType = strings.TrimSpace(sseEventType)
		}
		switch eventType {
		case "response.completed":
			if strings.TrimSpace(event.Response.Object) != "response" || strings.TrimSpace(event.Response.Status) != "completed" {
				return false, fmt.Errorf("response.completed event contains an incomplete response")
			}
			completed = event.Response
			return true, nil
		case "response.failed", "error":
			code := UpstreamErrorCode([]byte(data))
			status := UpstreamStatusForErrorCode(code)
			return false, &ResponsesAPIStatusError{Operation: "responses", Status: http.StatusText(status), StatusCode: status, Body: data}
		}
		return false, nil
	})
	if err != nil {
		return openAIResponsesProbeResponse{}, err
	}
	if found {
		return completed, nil
	}
	return openAIResponsesProbeResponse{}, fmt.Errorf("stream ended before response.completed")
}

func CheckChatCompletionsAPIWithClient(ctx context.Context, client *http.Client, baseURL, apiKey, modelID string, headers map[string]string) error {
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
		"model": modelID,
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
		"stream":     true,
		"max_tokens": 16,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode chat completions probe request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build chat completions probe request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
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
		return &UpstreamRequestError{Operation: "request chat completions", BaseURL: baseURL, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &ResponsesAPIStatusError{
			Operation:  "chat completions",
			BaseURL:    baseURL,
			Status:     resp.Status,
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(errBody)),
		}
	}

	var probe openAIChatCompletionsProbeResponse
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if mediaType == "text/event-stream" {
		probe, err = decodeOpenAIChatCompletionsProbeStream(resp.Body)
	} else {
		err = json.NewDecoder(resp.Body).Decode(&probe)
	}
	if err != nil {
		return &UpstreamRequestError{Operation: "decode chat completions probe", BaseURL: baseURL, Err: err}
	}
	if len(probe.Choices) == 0 {
		return &UpstreamRequestError{Operation: "decode chat completions probe", BaseURL: baseURL, Err: errors.New("returned no choices")}
	}
	return nil
}

func decodeOpenAIChatCompletionsProbeStream(r io.Reader) (openAIChatCompletionsProbeResponse, error) {
	var last openAIChatCompletionsProbeResponse
	sawChoices := false
	found, err := scanOpenAIProbeSSE(r, func(_ string, data string) (bool, error) {
		if data == "" {
			return false, nil
		}
		if data == "[DONE]" {
			if sawChoices {
				return true, nil
			}
			return false, fmt.Errorf("stream completed without chat choices")
		}
		if code := UpstreamErrorCode([]byte(data)); code != "" {
			status := UpstreamStatusForErrorCode(code)
			return false, &ResponsesAPIStatusError{Operation: "chat completions", Status: http.StatusText(status), StatusCode: status, Body: data}
		}
		var probe openAIChatCompletionsProbeResponse
		if err := json.Unmarshal([]byte(data), &probe); err != nil {
			return false, err
		}
		if len(probe.Choices) > 0 {
			last = probe
			sawChoices = true
			for _, choice := range probe.Choices {
				if choice.FinishReason != nil {
					return true, nil
				}
			}
		}
		return false, nil
	})
	if err != nil {
		return openAIChatCompletionsProbeResponse{}, err
	}
	if found {
		return last, nil
	}
	return openAIChatCompletionsProbeResponse{}, fmt.Errorf("stream ended before chat completion finished")
}

func scanOpenAIProbeSSE(r io.Reader, visit func(eventType, data string) (bool, error)) (bool, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	eventType := ""
	dataLines := make([]string, 0, 2)
	flush := func() (bool, error) {
		if eventType == "" && len(dataLines) == 0 {
			return false, nil
		}
		done, err := visit(eventType, strings.Join(dataLines, "\n"))
		eventType = ""
		dataLines = dataLines[:0]
		return done, err
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			if done, err := flush(); done || err != nil {
				return done, err
			}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return flush()
}
