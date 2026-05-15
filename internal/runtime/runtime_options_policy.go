package runtime

import (
	"strings"

	"csgclaw/internal/utils"
)

// NormalizeRuntimeKind returns canonical runtime_kind strings used for policy lookup and records.
func NormalizeRuntimeKind(kind string) string {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case KindPicoClawSandbox:
		return KindPicoClawSandbox
	case KindOpenClawSandbox:
		return KindOpenClawSandbox
	case KindCodex:
		return KindCodex
	case KindNotifier:
		return KindNotifier
	default:
		return strings.TrimSpace(kind)
	}
}

// RuntimeOptionsPolicy defines how runtime_options behave for a concrete runtime_kind.
// Implementations register via RegisterRuntimeOptionsPolicy.
type RuntimeOptionsPolicy interface {
	// StripProfileLLMFields clears LLM endpoint fields on runtimes that do not use them (e.g. notifier).
	StripProfileLLMFields(runtimeKind, baseURL, modelID string) (string, string)
	// IsComplete reports whether the agent profile is complete for this runtime_kind.
	// runtimeOptionsAfterPatch is merged agent runtime_options + incoming patch before persist (may be nil).
	IsComplete(llmComplete bool, runtimeOptions, runtimeOptionsAfterPatch map[string]any) bool
	MergeFlatForAgentPatch(agentRuntimeOptions, patchRuntimeOptions map[string]any) map[string]any
	ApplyFlatPersistence(agentRuntimeOptions *map[string]any, profileRuntimeOptions, profileRequestOptions map[string]any, mergedFlat map[string]any) (map[string]any, map[string]any)
}

var (
	runtimeOptionsPolicies   = make(map[string]RuntimeOptionsPolicy)
	defaultRuntimeOptionsPol = defaultRuntimeOptionsPolicy{}
)

// RegisterRuntimeOptionsPolicy binds a policy implementation to a normalized runtime_kind.
func RegisterRuntimeOptionsPolicy(kind string, p RuntimeOptionsPolicy) {
	kind = NormalizeRuntimeKind(kind)
	if kind == "" || p == nil {
		return
	}
	runtimeOptionsPolicies[kind] = p
}

// RuntimeOptionsPolicyForKind returns the registered policy, or a default no-op policy for unknown kinds.
func RuntimeOptionsPolicyForKind(kind string) RuntimeOptionsPolicy {
	kind = NormalizeRuntimeKind(kind)
	p, ok := runtimeOptionsPolicies[kind]
	if ok {
		return p
	}
	return defaultRuntimeOptionsPol
}

type defaultRuntimeOptionsPolicy struct{}

func (defaultRuntimeOptionsPolicy) StripProfileLLMFields(_, baseURL, modelID string) (string, string) {
	return baseURL, modelID
}

func (defaultRuntimeOptionsPolicy) IsComplete(llmComplete bool, _, _ map[string]any) bool {
	return llmComplete
}

func (defaultRuntimeOptionsPolicy) MergeFlatForAgentPatch(agentRuntimeOptions, patchRuntimeOptions map[string]any) map[string]any {
	if len(patchRuntimeOptions) == 0 {
		return utils.CloneAnyMap(agentRuntimeOptions)
	}
	if len(agentRuntimeOptions) == 0 {
		return utils.CloneAnyMap(patchRuntimeOptions)
	}
	return utils.OverlayAnyMap(utils.CloneAnyMap(agentRuntimeOptions), patchRuntimeOptions)
}

func (defaultRuntimeOptionsPolicy) ApplyFlatPersistence(agentRuntimeOptions *map[string]any, profileRuntimeOptions, profileRequestOptions map[string]any, mergedFlat map[string]any) (map[string]any, map[string]any) {
	if agentRuntimeOptions != nil && len(mergedFlat) > 0 {
		*agentRuntimeOptions = utils.CloneAnyMap(mergedFlat)
	}
	return profileRuntimeOptions, profileRequestOptions
}
