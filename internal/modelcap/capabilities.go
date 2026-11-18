package modelcap

import (
	"strings"
	"time"
)

type Capabilities struct {
	SupportsSearchTool              bool
	InputModalities                 []string
	SupportsReasoningEffort         bool
	SupportedReasoningEfforts       []string
	ReasoningEffortMap              map[string]string
	SupportsStreamingUsage          bool
	UseCodexMetadata                bool
	ResponsesReasoningInputKnown    bool
	SupportsResponsesReasoningInput bool
}

func ForProviderModel(provider, model string) Capabilities {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex":
		caps := codexCapabilities()
		caps.SupportedReasoningEfforts = codexReasoningEfforts(model)
		caps.SupportsReasoningEffort = len(caps.SupportedReasoningEfforts) > 0
		for _, effort := range caps.SupportedReasoningEfforts {
			caps.ReasoningEffortMap[effort] = effort
		}
		// Explicitly verified Codex models. Unknown models and third-party
		// providers retain full MCP tool definitions instead of deferred search.
		switch strings.ToLower(strings.TrimSpace(model)) {
		case "gpt-6.2-astra", "gpt-6.2-sol", "gpt-6.1-sol", "gpt-6.1-terra", "gpt-6-astra", "gpt-6-sol", "gpt-6-luna", "gpt-6-chat-thinking", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.3-codex-spark":
			caps.SupportsSearchTool = true
		}
		return caps
	default:
		caps := conservativeCapabilities()
		if isKnownVisionModel(model) {
			caps.InputModalities = []string{"text", "image"}
		}
		return caps
	}
}

func isKnownVisionModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return model == "gpt-6.2-astra" || strings.HasPrefix(model, "gpt-6.2-astra-") ||
		model == "gpt-6.2-sol" || strings.HasPrefix(model, "gpt-6.2-sol-") ||
		model == "qwen3.7-plus" || strings.HasPrefix(model, "qwen3.7-plus-") ||
		(strings.Contains(model, "qwen") && (strings.Contains(model, "-vl") || strings.Contains(model, "_vl")))
}

func conservativeCapabilities() Capabilities {
	return Capabilities{
		InputModalities:           []string{"text"},
		SupportedReasoningEfforts: []string{},
		ReasoningEffortMap:        map[string]string{},
	}
}

func codexCapabilities() Capabilities {
	return Capabilities{
		InputModalities:                 []string{"text", "image"},
		ReasoningEffortMap:              map[string]string{},
		SupportsStreamingUsage:          true,
		UseCodexMetadata:                true,
		ResponsesReasoningInputKnown:    true,
		SupportsResponsesReasoningInput: false,
	}
}

// Keep explicit levels aligned with the OpenAI and CLIProxyAPI model catalogs.
func codexReasoningEfforts(model string) []string {
	model = strings.ToLower(strings.TrimSpace(model))
	// Dated snapshots have the same reasoning contract as their model alias.
	if len(model) > 11 {
		if _, err := time.Parse("2006-01-02", model[len(model)-10:]); err == nil && model[len(model)-11] == '-' {
			model = model[:len(model)-11]
		}
	}
	switch model {
	case "gpt-6.2-astra", "gpt-6.2-sol", "gpt-6.1-sol", "gpt-6.1-terra", "gpt-6-astra", "gpt-6-chat-thinking":
		// The Codex Ultra preset depends on native multi-agent delegation, which
		// CSGClaw disables. The Responses endpoint accepts efforts through max.
		return []string{"low", "medium", "high", "xhigh", "max"}
	case "gpt-6-sol", "gpt-6-luna", "gpt-5.6", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna":
		return []string{"none", "low", "medium", "high", "xhigh", "max"}
	case "gpt-5.5", "gpt-5.4", "gpt-5.4-mini":
		return []string{"none", "low", "medium", "high", "xhigh"}
	case "gpt-5.3-codex-spark":
		// The bundled CLIProxy model registry supplies Spark's native levels.
		return []string{"low", "medium", "high", "xhigh"}
	default:
		return []string{}
	}
}
