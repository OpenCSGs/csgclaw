// Package modelusage normalizes provider-reported consumption, never context
// occupancy or estimates based on visible text.
package modelusage

import (
	"encoding/json"
)

const Kind = "token_usage"

type Usage struct {
	Known     bool   `json:"known"`
	ModelID   string `json:"model_id"`
	Input     int64  `json:"input_tokens"`
	Output    int64  `json:"output_tokens"`
	Total     int64  `json:"total_tokens"`
	Cached    int64  `json:"cached_input_tokens,omitempty"`
	Reasoning int64  `json:"reasoning_tokens,omitempty"`
	Partial   bool   `json:"partial,omitempty"`
}

// Decode accepts both in-process events and JSON-decoded Engine HTTP events.
func Decode(value any) *Usage {
	if usage, ok := value.(Usage); ok {
		if usage.Valid() {
			return &usage
		}
		return nil
	}
	raw, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	model, ok := raw["model_id"].(string)
	if !ok {
		return nil
	}
	known, ok := raw["known"].(bool)
	if !ok {
		return nil
	}
	usage := Parse(raw, model)
	if usage == nil {
		return nil
	}
	usage.Known = known
	usage.Partial, _ = raw["partial"].(bool)
	return usage
}

func Number(v any) (int64, bool) {
	var n int64
	switch v := v.(type) {
	case float64:
		if v < 0 || v > 1e12 || v != float64(int64(v)) {
			return 0, false
		}
		n = int64(v)
	case int:
		n = int64(v)
	case int64:
		n = v
	case json.Number:
		var err error
		n, err = v.Int64()
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return n, n >= 0 && n <= 1e12
}

// Parse accepts aggregate usage counters from Chat, Responses, Codex and DSH.
// A provider total can include cache tokens excluded from its input counter.
// Reasoning and cache details are subsets and are never added to the total.
func Parse(raw map[string]any, model string) *Usage {
	read := func(keys ...string) (int64, bool) {
		for _, key := range keys {
			if v, ok := raw[key]; ok {
				return Number(v)
			}
		}
		return 0, false
	}
	i, iok := read("input_tokens", "prompt_tokens", "inputTokens")
	o, ook := read("output_tokens", "completion_tokens", "outputTokens")
	if !iok || !ook {
		return nil
	}
	t := i + o
	if _, hasTotal := raw["total_tokens"]; !hasTotal {
		if _, hasTotal = raw["totalTokens"]; !hasTotal {
			// Messages separates uncached input from cache read/write input.
			for _, key := range []string{"cache_read_input_tokens", "cache_creation_input_tokens", "cacheReadTokens", "cacheWriteTokens"} {
				if v, ok := raw[key]; ok {
					n, valid := Number(v)
					if !valid {
						return nil
					}
					i += n
					t += n
				}
			}
		}
	}
	if v, ok := read("total_tokens", "totalTokens"); ok {
		if v < t {
			return nil
		}
		t = v
		i = t - o
	} else if raw["total_tokens"] != nil || raw["totalTokens"] != nil {
		return nil
	}
	u := &Usage{Known: true, ModelID: model, Input: i, Output: o, Total: t}
	u.Cached, _ = read("cached_input_tokens", "cachedInputTokens", "cacheReadTokens", "cache_read_input_tokens")
	u.Reasoning, _ = read("reasoning_tokens", "reasoningOutputTokens", "reasoningTokens")
	for _, key := range []string{"input_tokens_details", "prompt_tokens_details"} {
		if details, ok := raw[key].(map[string]any); ok {
			u.Cached, _ = Number(details["cached_tokens"])
		}
	}
	for _, key := range []string{"output_tokens_details", "completion_tokens_details"} {
		if details, ok := raw[key].(map[string]any); ok {
			u.Reasoning, _ = Number(details["reasoning_tokens"])
		}
	}
	if !u.Valid() {
		return nil
	}
	return u
}

func (u Usage) Valid() bool {
	return len(u.ModelID) <= 1024 && u.Input >= 0 && u.Output >= 0 && u.Total == u.Input+u.Output && u.Total <= 1e12 && u.Cached >= 0 && u.Cached <= u.Input && u.Reasoning >= 0 && u.Reasoning <= u.Output
}

func (u *Usage) Add(v Usage) {
	u.Known = u.Known || v.Known
	u.Input += v.Input
	u.Output += v.Output
	u.Total += v.Total
	u.Cached += v.Cached
	u.Reasoning += v.Reasoning
	u.Partial = u.Partial || v.Partial
}

func (u Usage) Sub(v Usage) Usage {
	u.Input -= v.Input
	u.Output -= v.Output
	u.Total -= v.Total
	u.Cached -= v.Cached
	u.Reasoning -= v.Reasoning
	return u
}

// Responses translates Chat counters to the Responses schema while preserving
// optional detail fields. Gateway-specific billing breakdowns are not summed.
func Responses(raw map[string]any) map[string]any {
	u := Parse(raw, "")
	if u == nil {
		return nil
	}
	return map[string]any{"input_tokens": u.Input, "output_tokens": u.Output, "total_tokens": u.Total,
		"input_tokens_details":  map[string]any{"cached_tokens": u.Cached},
		"output_tokens_details": map[string]any{"reasoning_tokens": u.Reasoning}}
}
