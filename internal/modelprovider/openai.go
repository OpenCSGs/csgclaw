package modelprovider

import (
	"bufio"
	"bytes"
	"context"
	"csgclaw/internal/modelcap"
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
		ID            string `json:"id"`
		ContextWindow int64  `json:"context_window"`
		ContextLength int64  `json:"context_length"`
		Task          any    `json:"task"`
		Tasks         any    `json:"tasks"`
		Availability  *struct {
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
	return UpstreamErrorCodeForResponse(e.StatusCode, []byte(e.Body))
}

func (e *ResponsesAPIStatusError) UserMessage() string {
	return FriendlyUpstreamErrorMessage(e.StatusCode, e.Code())
}

func ListOpenAIModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	return ListOpenAIModelsWithClient(ctx, &http.Client{Timeout: 2 * time.Second}, baseURL, apiKey, nil)
}

func ListOpenAIModelsWithClient(ctx context.Context, client *http.Client, baseURL, apiKey string, headers map[string]string) ([]string, error) {
	directory, err := ListOpenAIModelDirectoryWithClient(ctx, client, baseURL, apiKey, headers)
	if err != nil {
		return nil, err
	}
	if len(directory.Models) == 0 {
		return nil, &UpstreamRequestError{Operation: "decode models response", BaseURL: baseURL, Err: errors.New("no text models returned")}
	}
	return directory.Models, nil
}

// ListOpenAIModelDirectoryWithClient separates declared image generation tasks
// from chat models without probing or generating images.
func ListOpenAIModelDirectoryWithClient(ctx context.Context, client *http.Client, baseURL, apiKey string, headers map[string]string) (ModelDiscoveryResult, error) {
	return listOpenAIModelDirectoryWithClient(ctx, client, baseURL, apiKey, headers, false)
}

// ListOpenCSGModelDirectoryWithClient extends the OpenAI-compatible model
// directory with OpenCSG's plural tasks field.
func ListOpenCSGModelDirectoryWithClient(ctx context.Context, client *http.Client, baseURL, apiKey string, headers map[string]string) (ModelDiscoveryResult, error) {
	return listOpenAIModelDirectoryWithClient(ctx, client, baseURL, apiKey, headers, true)
}

func listOpenAIModelDirectoryWithClient(ctx context.Context, client *http.Client, baseURL, apiKey string, headers map[string]string, includePluralTasks bool) (ModelDiscoveryResult, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return ModelDiscoveryResult{}, fmt.Errorf("base URL is required")
	}
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}

	resp, err := requestOpenAIModels(ctx, client, baseURL+"/models", apiKey, headers)
	if err != nil {
		return ModelDiscoveryResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return ModelDiscoveryResult{}, &ResponsesAPIStatusError{
			Operation:  "models",
			BaseURL:    baseURL,
			Status:     resp.Status,
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(errBody)),
		}
	}

	var payload openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ModelDiscoveryResult{}, &UpstreamRequestError{Operation: "decode models response", BaseURL: baseURL, Err: err}
	}

	metadata := make(map[string]modelcap.Metadata)
	models := make([]string, 0, len(payload.Data))
	imageModels := []string{}
	visionModels := []string{}
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if item.Availability != nil && item.Availability.IsAvailable != nil && !*item.Availability.IsAvailable {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		m := modelcap.Metadata{ContextWindow: item.ContextWindow}
		if m.ContextWindow == 0 {
			m.ContextWindow = item.ContextLength
		}
		if m.Validate() == nil && m != (modelcap.Metadata{}) {
			metadata[id] = m
		}
		var declaredTasks any = item.Task
		if includePluralTasks {
			declaredTasks = append(taskValues(item.Task), taskValues(item.Tasks)...)
		}
		if taskSupportsImageGeneration(declaredTasks) || (!taskPresent(declaredTasks) && IsGPTImageModel(id)) {
			imageModels = append(imageModels, id)
		}
		if taskSupportsVisionInput(declaredTasks) {
			visionModels = append(visionModels, id)
		}
		if !taskPresent(declaredTasks) || taskSupportsTextGeneration(declaredTasks) {
			models = append(models, id)
		}
	}
	if len(models) == 0 && len(imageModels) == 0 {
		return ModelDiscoveryResult{}, &UpstreamRequestError{Operation: "decode models response", BaseURL: baseURL, Err: errors.New("no models returned")}
	}
	return ModelDiscoveryResult{ResolvedBaseURL: baseURL, Models: models, ImageModels: imageModels, VisionModels: visionModels, ModelMetadata: metadata}, nil
}

func taskPresent(task any) bool {
	switch value := task.(type) {
	case string:
		return strings.TrimSpace(value) != ""
	case []any:
		return len(value) > 0
	case []string:
		return len(value) > 0
	default:
		return task != nil
	}
}

func taskSupportsTextGeneration(task any) bool {
	for _, value := range taskValues(task) {
		if strings.EqualFold(strings.TrimSpace(value), "text-generation") {
			return true
		}
	}
	return false
}

func taskSupportsVisionInput(task any) bool {
	values := taskValues(task)
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "image-text-to-text", "image-to-text", "vision":
			return true
		}
	}
	return false
}

func taskValues(task any) []string {
	var raw []string
	switch value := task.(type) {
	case string:
		raw = strings.Split(value, ",")
	case []any:
		for _, entry := range value {
			if text, ok := entry.(string); ok {
				raw = append(raw, text)
			}
		}
	case []string:
		raw = append(raw, value...)
	}
	values := make([]string, 0, len(raw))
	for _, value := range raw {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
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
		if err := validateOpenAIResponsesProbeStream(resp.Body); err != nil {
			return &UpstreamRequestError{Operation: "read responses probe stream", BaseURL: baseURL, Err: err}
		}
		return nil
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

func validateOpenAIResponsesProbeStream(r io.Reader) error {
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
		case "response.failed", "response.incomplete", "error":
			code := UpstreamErrorCode([]byte(data))
			status := UpstreamStatusForErrorCode(code)
			return false, &ResponsesAPIStatusError{Operation: "responses", Status: http.StatusText(status), StatusCode: status, Body: data}
		case "response.created", "response.queued", "response.in_progress", "response.completed":
			if strings.TrimSpace(event.Response.Object) != "response" {
				return false, fmt.Errorf("%s event contains an invalid response", eventType)
			}
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	if found {
		return nil
	}
	return fmt.Errorf("stream ended before the first responses event")
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
		if code, isUpstreamError := UpstreamError([]byte(data)); isUpstreamError {
			if code == "" {
				code = "upstream_unavailable"
			}
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

func taskSupportsImageGeneration(task any) bool {
	values := taskValues(task)
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "text-to-image", "text2image", "image-generation":
			return true
		}
	}
	return false
}
