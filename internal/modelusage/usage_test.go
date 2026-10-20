package modelusage

import (
	"encoding/json"
	"testing"
)

func TestProviderUsage(t *testing.T) {
	for _, tc := range []struct {
		name, raw                        string
		input, output, cached, reasoning int64
	}{
		{"opencsg-chat", `{"completion_tokens":144,"completion_tokens_details":{"reasoning_tokens":140,"text_tokens":144},"prompt_tokens":14,"prompt_tokens_details":{"cached_tokens":0,"text_tokens":14},"total_tokens":158}`, 14, 144, 0, 140},
		{"opencsg-responses", `{"input_tokens":50,"input_tokens_details":{"cached_tokens":0},"output_tokens":139,"output_tokens_details":{"reasoning_tokens":135},"total_tokens":189,"x_details":[{"input_tokens":50,"output_tokens":139,"total_tokens":189}]}`, 50, 139, 0, 135},
		{"opencsg-messages-final", `{"input_tokens":14,"output_tokens":164}`, 14, 164, 0, 0},
		{"anthropic-cache", `{"input_tokens":10,"output_tokens":20,"cache_read_input_tokens":100,"cache_creation_input_tokens":50}`, 160, 20, 100, 0},
		{"codex", `{"inputTokens":1000,"outputTokens":220,"totalTokens":1220,"cachedInputTokens":400,"reasoningOutputTokens":100}`, 1000, 220, 400, 100},
		{"dsh", `{"inputTokens":10,"outputTokens":20,"totalTokens":180,"cacheReadTokens":100,"cacheWriteTokens":50}`, 160, 20, 100, 0},
		{"dsh-no-total", `{"inputTokens":10,"outputTokens":20,"cacheReadTokens":100,"cacheWriteTokens":50}`, 160, 20, 100, 0},
		{"zero", `{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}`, 0, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw map[string]any
			if err := json.Unmarshal([]byte(tc.raw), &raw); err != nil {
				t.Fatal(err)
			}
			u := Parse(raw, "auto")
			if u == nil || !u.Known || u.Input != tc.input || u.Output != tc.output || u.Total != tc.input+tc.output || u.Cached != tc.cached || u.Reasoning != tc.reasoning {
				t.Fatalf("usage=%+v", u)
			}
		})
	}
	for _, raw := range []string{`{}`, `{"input_tokens":-1,"output_tokens":1}`, `{"input_tokens":1.5,"output_tokens":1}`, `{"input_tokens":5,"output_tokens":3,"total_tokens":7}`, `{"input_tokens":0,"output_tokens":0,"output_tokens_details":{"reasoning_tokens":4}}`} {
		var v map[string]any
		_ = json.Unmarshal([]byte(raw), &v)
		if Parse(v, "") != nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
