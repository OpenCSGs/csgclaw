package config

import "strings"

const (
	ReasoningEffortAuto    = "auto"
	ReasoningEffortNone    = "none"
	ReasoningEffortMinimal = "minimal"
	ReasoningEffortLow     = "low"
	ReasoningEffortMedium  = "medium"
	ReasoningEffortHigh    = "high"
	ReasoningEffortXHigh   = "xhigh"
)

// NormalizeReasoningEffort keeps the cross-runtime profile contract stable.
// OpenClaw's "off" spelling is accepted at input boundaries, while profiles
// persist "none", which is also the native Codex spelling.
func NormalizeReasoningEffort(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "off" {
		return ReasoningEffortNone
	}
	return value
}

// UsesModelReasoningDefault reports whether the runtime should avoid sending
// an explicit reasoning effort and let the selected model choose its default.
func UsesModelReasoningDefault(value string) bool {
	value = NormalizeReasoningEffort(value)
	return value == "" || value == ReasoningEffortAuto
}

// HasExplicitReasoningEffort reports whether a profile explicitly requests a
// non-disabled effort. Runtime adapters use this as a user capability claim
// for OpenAI-compatible models that do not advertise reasoning metadata.
func HasExplicitReasoningEffort(value string) bool {
	value = NormalizeReasoningEffort(value)
	if value == "" || value == ReasoningEffortAuto || value == ReasoningEffortNone {
		return false
	}
	for _, effort := range CommonReasoningEfforts() {
		if value == effort {
			return true
		}
	}
	return false
}

func CommonReasoningEfforts() []string {
	return []string{
		ReasoningEffortMinimal,
		ReasoningEffortLow,
		ReasoningEffortMedium,
		ReasoningEffortHigh,
		ReasoningEffortXHigh,
	}
}
