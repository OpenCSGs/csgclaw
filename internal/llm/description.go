package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"csgclaw/internal/modelprovider"
)

// CompleteDescription uses the selected Agent model without a runtime session or tools.
func (s *Service) CompleteDescription(ctx context.Context, agentID, prompt string, source json.RawMessage) (string, error) {
	profile, err := s.resolveProfile(ctx, agentID)
	if err != nil {
		return "", err
	}
	profile.RequestOptions = nil
	profile.ReasoningEffort = ""
	profile.EnableFastMode = false
	payload, err := json.Marshal(map[string]any{
		"messages": []map[string]string{
			{"role": "system", "content": prompt},
			{"role": "user", "content": string(source)},
		},
		"stream":                false,
		"max_completion_tokens": 1024,
	})
	if err != nil {
		return "", err
	}
	body, status, _, err := s.ChatCompletions(WithTaskProfile(ctx, profile), agentID, payload)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", &HTTPError{Status: status, Code: modelprovider.UpstreamErrorCodeForResponse(status, body)}
	}
	var response struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &response); err == nil && len(response.Choices) > 0 {
		choice := response.Choices[0]
		text := strings.TrimSpace(choice.Message.Content)
		if choice.FinishReason != "length" && choice.FinishReason != "content_filter" && choice.FinishReason != "tool_calls" && text != "" && utf8.RuneCountInString(text) <= 1000 {
			return text, nil
		}
	}
	return "", &HTTPError{Status: http.StatusBadGateway, Message: "Model returned an invalid description"}
}
