package modelcap

import "testing"

func TestMetadataResolution(t *testing.T) {
	tests := []struct {
		name, provider, endpoint, model string
		discovered, override            Metadata
		window                          int64
		source                          string
	}{
		{"unknown", "custom", "http://local/v1", "x", Metadata{}, Metadata{}, 200000, "default"},
		{"official", "api", "https://api.openai.com/v1", "gpt-4o", Metadata{}, Metadata{}, 128000, "catalog"},
		{"private deployment", "custom", "http://local/v1", "gpt-4o", Metadata{}, Metadata{}, 128000, "catalog"},
		{"provider wins", "api", "https://api.openai.com/v1", "gpt-4o", Metadata{64000}, Metadata{}, 64000, "provider"},
		{"override wins", "api", "https://api.openai.com/v1", "gpt-4o", Metadata{64000}, Metadata{8192}, 8192, "user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.provider, tt.endpoint, tt.model, tt.discovered, tt.override)
			if got.ContextWindow != tt.window || got.ContextSource != tt.source {
				t.Fatalf("%+v", got)
			}
		})
	}
}
func TestMetadataValidation(t *testing.T) {
	for _, m := range []Metadata{{ContextWindow: -1}, {ContextWindow: 1000000001}} {
		if m.Validate() == nil {
			t.Fatalf("accepted %+v", m)
		}
	}
}
func TestCloneMetadataDoesNotAlias(t *testing.T) {
	original := map[string]Metadata{"m": {ContextWindow: 8000}}
	copy := Clone(original)
	copy["m"] = Metadata{}
	if original["m"].ContextWindow != 8000 {
		t.Fatal("aliased metadata")
	}
}

func TestCommonProviderModelAliases(t *testing.T) {
	for model, want := range map[string]int64{
		"OpenCSG/Qwen3.7-Plus": 1000000, "deepseek-v4-flash-vision-exp": 1000000, "DeepSeek-V3.2": 128000, "csg-gpt4o": 128000, "gpt-5-high": 400000, "openai/gpt-5.4-nano": 400000, "gpt-5.3-codex-spark": 128000,
		"anthropic.claude-opus-4-6-v1:0": 1000000, "claude-opus-4-20250514": 200000, "claude-sonnet-4-5-20250929": 200000, "CLAUDE_SONNET_4_6": 1000000, "glm5.1-fp8": 200000, "glm-5.2": 1000000, "moonshotai/Kimi-K2.5": 262144, "gemini-2.5-pro": 1048576,
		"mygpt-4o": 200000, "gpt-5.60": 200000, "claude-sonnet-4-99": 200000, "mock-responses": 200000, "auto": 200000,
	} {
		t.Run(model, func(t *testing.T) {
			got := Resolve("opencsg", "http://gateway/v1", model, Metadata{}, Metadata{})
			if got.ContextWindow != want {
				t.Fatalf("%s: got %+v want %d", model, got, want)
			}
		})
	}
}

func TestOpenCSGPublicModelCapacities(t *testing.T) {
	for model, want := range map[string]int64{
		"qwen3.8-max": 1000000, "qwen3.8-flash-0902": 1000000,
		"Qwen/Qwen3Guard-Gen-0.6B": 32768, "Qwen/Qwen3Guard-Stream-0.6B": 8192,
		"OpenCSG/Agentic-27B": 262144, "Qwen2-0.5B-Instruct:1mz": 32768,
		"minimax-m3": 1000000, "MiniMax-M2.5": 204800, "MiniMax-M2.7-highspeed": 204800,
		"Qwen3-0.6B": 32768, "Qwen_Qwen3-Embedding-0.6B": 32768,
		"AIWizards/Qwen_Qwen3-Embedding-4B": 32768, "mimo-v2-pro": 1000000,
		"deepseek-v4-flash-message": 1000000,
	} {
		t.Run(model, func(t *testing.T) {
			got := Resolve("opencsg", "", model, Metadata{}, Metadata{})
			if got.ContextWindow != want || got.ContextSource != "catalog" {
				t.Fatalf("got %+v, want %d from catalog", got, want)
			}
		})
	}
}
