package modelcap

import (
	"regexp"
	"strings"
)

// Reference capacities, not deployment guarantees. Provider metadata and user
// overrides always win. Specific aliases precede their shorter family names.
// Sources: developers.openai.com/api/docs/models; platform.claude.com/docs/en/models;
// api-docs.deepseek.com/quick_start/pricing; alibabacloud.com/help/en/model-studio;
// docs.z.ai/guides/llm; ai.google.dev/gemini-api/docs/models;
// kimi.com/help/kimi-api/api-troubleshooting.
var contextCatalog = []struct {
	aliases []string
	tokens  int64
}{
	{[]string{"gpt-6-astra", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.6", "gpt-5.5", "gpt-5.4"}, 1050000},
	{[]string{"gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.3-codex", "gpt-5.2", "gpt-5.1", "gpt-5-mini", "gpt-5-nano", "gpt-5"}, 400000},
	{[]string{"gpt-5.3-codex-spark"}, 128000},
	{[]string{"gpt-4.1-mini", "gpt-4.1-nano", "gpt-4.1"}, 1047576},
	{[]string{"gpt-4o-mini", "gpt-4o", "gpt4o"}, 128000},
	{[]string{"claude-fable-5.1", "claude-opus-5", "claude-sonnet-5", "claude-opus-4.8", "claude-opus-4.7", "claude-opus-4.6", "claude-sonnet-4.6"}, 1000000},
	{[]string{"claude-opus-4.5", "claude-opus-4.1", "claude-opus-4", "claude-sonnet-4.5", "claude-sonnet-4", "claude-haiku-4.5", "claude-3.7-sonnet", "claude-3.5-sonnet", "claude-3.5-haiku", "claude-3-opus", "claude-3-sonnet", "claude-3-haiku"}, 200000},
	{[]string{"deepseek-v4.1", "deepseek-v4", "deepseek-flash"}, 1000000},
	{[]string{"deepseek-v3.2", "deepseek-v3.1", "deepseek-v3", "deepseek-r1", "deepseek-chat", "deepseek-reasoner"}, 128000},
	{[]string{"qwen3.8-max", "qwen3.8-flash", "qwen3.7-plus", "qwen3.7-max", "qwen3.6-plus", "qwen3.5-plus", "qwen-plus"}, 1000000},
	{[]string{"qwen3.6-max", "qwen3-max", "qwen3-coder-plus", "qwen3-coder-flash"}, 262144},
	{[]string{"qwen-max"}, 32768},
	{[]string{"glm-5.2", "glm5.2", "glm-5.3", "glm5.3"}, 1000000},
	{[]string{"glm-5.1", "glm5.1", "glm-5", "glm5", "glm-4.7", "glm-4.6"}, 200000},
	{[]string{"gemini-3.8-flash", "gemini-3.7-flash", "gemini-3.6-flash", "gemini-3.5-flash", "gemini-3.1-pro", "gemini-3.1-flash-lite", "gemini-3-pro", "gemini-3-flash", "gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.0-flash"}, 1048576},
	{[]string{"kimi-k3"}, 1048576},
	{[]string{"kimi-k2.6", "kimi-k2.5", "kimi-k2"}, 262144},
	// OpenCSG's current public catalog. Capacities come from the publishers' API
	// specifications or config.json, not the smaller evaluation deployment limits.
	// huggingface.co/opencsg/Agentic-27B; huggingface.co/Qwen/*/raw/main/config.json
	{[]string{"opencsg/agentic-27b"}, 262144},
	{[]string{"qwen3guard-gen-0.6b"}, 32768},
	{[]string{"qwen3guard-stream-0.6b"}, 8192},
	{[]string{"qwen2-0.5b-instruct", "qwen3-embedding-0.6b"}, 32768},
	{[]string{"qwen3-0.6b"}, 32768},
	{[]string{"qwen3-embedding-4b"}, 32768},
	// platform.minimax.io/docs/guides/text-generation
	{[]string{"minimax-m3"}, 1000000},
	{[]string{"minimax-m2.7", "minimax-m2.5", "minimax-m2.1", "minimax-m2"}, 204800},
	// mimo.mi.com/docs/en-US/news/previous-news/v2-pro-release
	{[]string{"mimo-v2-pro"}, 1000000},
}
var modelSeparators = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeModelName(s string) string {
	return strings.Trim(modelSeparators.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

func catalog(_, _, model string) Metadata {
	name := normalizeModelName(model)
	var best Metadata
	longest := 0
	for _, entry := range contextCatalog {
		for _, raw := range entry.aliases {
			alias := normalizeModelName(raw)
			if len(alias) > longest && matchesModelAlias(name, alias) {
				best = Metadata{ContextWindow: entry.tokens}
				longest = len(alias)
			}
		}
	}
	return best
}

// Permit namespaces, deployment prefixes, dated snapshots and familiar serving
// suffixes, without matching arbitrary unknown versions or unrelated words.
func matchesModelAlias(name, alias string) bool {
	index := strings.Index(name, alias)
	if index < 0 || (index > 0 && name[index-1] != '-') {
		return false
	}
	tail := name[index+len(alias):]
	if tail == "" {
		return true
	}
	if tail[0] != '-' {
		return false
	}
	suffix := strings.Split(strings.TrimPrefix(tail, "-"), "-")[0]
	if len(suffix) >= 4 && (strings.HasPrefix(suffix, "20")) {
		return true
	}
	switch suffix {
	case "latest", "preview", "exp", "vision", "thinking", "instruct", "fp8", "fp16", "bf16", "int4", "int8", "q4", "q5", "q8", "high", "medium", "low", "xhigh", "max", "fast", "turbo", "pro", "flash", "v1", "v2", "1m", "1mz", "message", "highspeed", "0902":
		return true
	}
	return false
}
