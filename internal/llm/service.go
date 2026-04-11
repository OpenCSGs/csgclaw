package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
)

type Service struct {
	defaults config.ModelConfig
	agents   *agent.Service
	client   *http.Client
}

type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func NewService(defaults config.ModelConfig, agents *agent.Service) *Service {
	return &Service{
		defaults: defaults.Resolved(),
		agents:   agents,
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

func (s *Service) Models(_ context.Context, botID string) ([]byte, int, string, error) {
	cfg, err := s.resolveModelConfig(botID)
	if err != nil {
		return nil, 0, "", err
	}
	payload := map[string]any{
		"object": "list",
		"data": []map[string]any{
			{
				"id":       cfg.ModelID,
				"object":   "model",
				"created":  0,
				"owned_by": "csgclaw",
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, "", &HTTPError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("encode models response: %v", err)}
	}
	return body, http.StatusOK, "application/json", nil
}

func (s *Service) ChatCompletions(ctx context.Context, botID string, body []byte) ([]byte, int, string, error) {
	cfg, err := s.resolveModelConfig(botID)
	if err != nil {
		return nil, 0, "", err
	}
	return s.forwardRemoteChat(ctx, cfg, body)
}

func (s *Service) resolveModelConfig(botID string) (config.ModelConfig, error) {
	if s.agents == nil {
		return config.ModelConfig{}, &HTTPError{Status: http.StatusServiceUnavailable, Message: "agent service is not configured"}
	}
	cfg, err := s.agents.ResolvedModelConfig(botID)
	if err != nil {
		return config.ModelConfig{}, &HTTPError{Status: http.StatusNotFound, Message: err.Error()}
	}
	return cfg.Resolved(), nil
}

func (s *Service) forwardRemoteChat(ctx context.Context, cfg config.ModelConfig, body []byte) ([]byte, int, string, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, 0, "", &HTTPError{Status: http.StatusBadRequest, Message: fmt.Sprintf("decode request: %v", err)}
	}
	payload["model"] = cfg.ModelID
	applyReasoningEffortDefault(payload, cfg.ReasoningEffort)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, "", &HTTPError{Status: http.StatusBadRequest, Message: fmt.Sprintf("encode request: %v", err)}
	}

	upstreamURL := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(encoded))
	if err != nil {
		return nil, 0, "", &HTTPError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("build upstream request: %v", err)}
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, "", &HTTPError{Status: http.StatusBadGateway, Message: fmt.Sprintf("send upstream request: %v", err)}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, 0, "", &HTTPError{Status: http.StatusBadGateway, Message: fmt.Sprintf("read upstream response: %v", err)}
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	return respBody, resp.StatusCode, contentType, nil
}

func applyReasoningEffortDefault(payload map[string]any, defaultEffort string) {
	defaultEffort = strings.ToLower(strings.TrimSpace(defaultEffort))
	if defaultEffort == "" {
		return
	}
	if value, ok := payload["reasoning_effort"]; ok {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return
		}
		if value != nil {
			return
		}
	}
	payload["reasoning_effort"] = defaultEffort
}
