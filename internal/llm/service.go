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
	"csgclaw/internal/cliproxy"
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

var embeddedCLIProxyProviderBaseURL = func(ctx context.Context, provider string) (string, error) {
	return cliproxy.Default().ProviderBaseURL(ctx, provider)
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
	profile, err := s.resolveProfile(botID)
	if err != nil {
		return nil, 0, "", err
	}
	payload := map[string]any{
		"object": "list",
		"data": []map[string]any{
			{
				"id":       profile.ModelID,
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
	profile, err := s.resolveProfile(botID)
	if err != nil {
		return nil, 0, "", err
	}
	return s.forwardRemoteChat(ctx, profile, body)
}

func (s *Service) resolveProfile(botID string) (agent.AgentProfile, error) {
	if s.agents == nil {
		return agent.AgentProfile{}, &HTTPError{Status: http.StatusServiceUnavailable, Message: "agent service is not configured"}
	}
	profile, err := s.agents.ResolvedAgentProfile(botID)
	if err != nil {
		status := http.StatusNotFound
		if strings.Contains(err.Error(), "profile is incomplete") {
			status = http.StatusConflict
		}
		return agent.AgentProfile{}, &HTTPError{Status: status, Message: err.Error()}
	}
	return profile, nil
}

func (s *Service) forwardRemoteChat(ctx context.Context, profile agent.AgentProfile, body []byte) ([]byte, int, string, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, 0, "", &HTTPError{Status: http.StatusBadRequest, Message: fmt.Sprintf("decode request: %v", err)}
	}
	mergeProfilePayload(payload, profile)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, "", &HTTPError{Status: http.StatusBadRequest, Message: fmt.Sprintf("encode request: %v", err)}
	}

	baseURL, apiKey, err := s.agentProfileTarget(ctx, profile)
	if err != nil {
		return nil, 0, "", err
	}
	if baseURL == "" {
		return nil, 0, "", &HTTPError{Status: http.StatusBadRequest, Message: "profile base_url is required"}
	}
	upstreamURL := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(encoded))
	if err != nil {
		return nil, 0, "", &HTTPError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("build upstream request: %v", err)}
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range profile.Headers {
		key = strings.TrimSpace(key)
		if key == "" || strings.EqualFold(key, "authorization") || strings.EqualFold(key, "content-type") {
			continue
		}
		req.Header.Set(key, value)
	}

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

func mergeProfilePayload(payload map[string]any, profile agent.AgentProfile) {
	payload["model"] = strings.TrimSpace(profile.ModelID)
	applyReasoningEffortDefault(payload, profile.ReasoningEffort)
	for key, value := range profile.RequestOptions {
		key = strings.TrimSpace(key)
		if key == "" || strings.EqualFold(key, "model") {
			continue
		}
		if _, exists := payload[key]; exists {
			continue
		}
		payload[key] = value
	}
	applyProviderPayloadConstraints(payload, profile)
	if !profile.EnableFastMode {
		return
	}
	switch profile.Provider {
	case agent.ProviderClaudeCode:
		if _, exists := payload["speed"]; !exists {
			payload["speed"] = "fast"
		}
	default:
		if _, exists := payload["service_tier"]; !exists {
			payload["service_tier"] = "priority"
		}
	}
}

func applyProviderPayloadConstraints(payload map[string]any, profile agent.AgentProfile) {
	switch profile.Provider {
	case agent.ProviderCodex:
		payload["store"] = false
	}
}

func (s *Service) agentProfileTarget(ctx context.Context, profile agent.AgentProfile) (string, string, error) {
	switch profile.Provider {
	case agent.ProviderCodex, agent.ProviderClaudeCode:
		baseURL, err := embeddedCLIProxyProviderBaseURL(ctx, profile.Provider)
		if err != nil {
			return "", "", &HTTPError{Status: http.StatusBadGateway, Message: fmt.Sprintf("embedded cliproxy unavailable: %v", err)}
		}
		return baseURL, cliproxy.LocalAPIKey, nil
	default:
		return agent.ProfileBaseURL(profile), agent.ProfileAPIKey(profile), nil
	}
}
