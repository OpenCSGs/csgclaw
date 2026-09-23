package modelcap

import "strings"

const (
	OpenClawAPIChatCompletions = "openai-completions"
	OpenClawAPICodexResponses  = "openai-codex-responses"
)

type Capabilities struct {
	SupportsSearchTool              bool
	OpenClawAPI                     string
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
		// Explicitly verified Codex models. Unknown models and third-party
		// providers retain full MCP tool definitions instead of deferred search.
		switch strings.ToLower(strings.TrimSpace(model)) {
		case "gpt-6-astra", "gpt-6-sol", "gpt-6-luna", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.3-codex-spark":
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
	return model == "qwen3.7-plus" || strings.HasPrefix(model, "qwen3.7-plus-") ||
		(strings.Contains(model, "qwen") && (strings.Contains(model, "-vl") || strings.Contains(model, "_vl")))
}

func conservativeCapabilities() Capabilities {
	return Capabilities{
		OpenClawAPI:               OpenClawAPIChatCompletions,
		InputModalities:           []string{"text"},
		SupportedReasoningEfforts: []string{},
		ReasoningEffortMap:        map[string]string{},
	}
}

func codexCapabilities() Capabilities {
	return Capabilities{
		OpenClawAPI:                     OpenClawAPICodexResponses,
		InputModalities:                 []string{"text", "image"},
		SupportsReasoningEffort:         true,
		SupportedReasoningEfforts:       []string{"minimal", "low", "medium", "high", "xhigh"},
		ReasoningEffortMap:              map[string]string{"minimal": "minimal", "low": "low", "medium": "medium", "high": "high", "xhigh": "xhigh"},
		SupportsStreamingUsage:          true,
		UseCodexMetadata:                true,
		ResponsesReasoningInputKnown:    true,
		SupportsResponsesReasoningInput: false,
	}
}
