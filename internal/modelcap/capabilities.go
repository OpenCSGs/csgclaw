package modelcap

import "strings"

const (
	OpenClawAPIChatCompletions = "openai-completions"
	OpenClawAPICodexResponses  = "openai-codex-responses"
)

type Capabilities struct {
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

func ForProviderModel(provider, _ string) Capabilities {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex":
		return codexCapabilities()
	default:
		return conservativeCapabilities()
	}
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
