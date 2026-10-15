package llm

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
)

func init() {
	// The SDK registers Chat -> Codex, whose wire format is Responses, but not
	// Chat -> Responses. Adapt that converter without inheriting Codex-only
	// generation defaults or its terminal-event envelope for JSON responses.
	translator.Register(translator.FormatOpenAI, translator.FormatOpenAIResponse, chatRequestToResponses, translator.ResponseTransform{
		Stream:    responsesStreamToChat,
		NonStream: responsesJSONToChat,
	})
}

func chatRequestToResponses(model string, raw []byte, stream bool) []byte {
	converted := translator.TranslateRequest(translator.FormatOpenAI, translator.FormatCodex, model, raw, stream)
	var source, target map[string]json.RawMessage
	_ = json.Unmarshal(raw, &source)
	_ = json.Unmarshal(converted, &target)
	// These parameters are supported by ordinary Responses services. The Codex
	// converter either drops them or supplies Codex-specific defaults.
	for _, field := range []string{"temperature", "top_p", "parallel_tool_calls", "service_tier", "metadata", "user", "safety_identifier", "prompt_cache_key", "prompt_cache_retention", "include"} {
		if value, ok := source[field]; ok {
			target[field] = value
		} else {
			delete(target, field)
		}
	}
	if value, ok := source["store"]; ok {
		target["store"] = value
	}
	if value, ok := source["max_completion_tokens"]; ok {
		target["max_output_tokens"] = value
	} else if value, ok = source["max_tokens"]; ok {
		target["max_output_tokens"] = value
	}
	if source["reasoning_effort"] == nil {
		delete(target, "reasoning")
	}
	encoded, _ := json.Marshal(target)
	return encoded
}

func responsesJSONToChat(ctx context.Context, model string, original, request, raw []byte, state *any) []byte {
	var response struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(raw, &response)
	kind := "response.completed"
	if response.Status == "incomplete" {
		kind = "response.incomplete"
	}
	event, _ := json.Marshal(struct {
		Type     string          `json:"type"`
		Response json.RawMessage `json:"response"`
	}{kind, raw})
	return translator.TranslateNonStream(ctx, translator.FormatCodex, translator.FormatOpenAI, model, original, request, event, state)
}

func responsesStreamToChat(ctx context.Context, model string, original, request, raw []byte, state *any) [][]byte {
	chunks := translator.TranslateStream(ctx, translator.FormatCodex, translator.FormatOpenAI, model, original, request, raw, state)
	var event struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(bytes.TrimSpace(bytes.TrimPrefix(raw, []byte("data:"))), &event)
	if event.Type == "response.completed" || event.Type == "response.incomplete" {
		chunks = append(chunks, []byte("data: [DONE]\n\n"))
	}
	return chunks
}
