package runtime

// RuntimeOptionsPolicy defines how runtime_options behave for a concrete runtime_kind.
// Implementations register via RegisterRuntimeOptionsPolicy.
type RuntimeOptionsPolicy interface {
	// StripProfileLLMFields clears LLM endpoint fields on runtimes that do not use them (e.g. notifier).
	StripProfileLLMFields(runtimeKind, baseURL, modelID string) (string, string)
	// IsComplete reports whether the agent profile is complete for this runtime_kind.
	// runtimeOptionsAfterPatch is merged agent runtime_options + incoming patch before persist (may be nil).
	IsComplete(llmComplete bool, runtimeOptions, runtimeOptionsAfterPatch map[string]any) bool
}

var (
	runtimeOptionsPolicies   = make(map[string]RuntimeOptionsPolicy)
	defaultRuntimeOptionsPol = defaultRuntimeOptionsPolicy{}
)

// RegisterRuntimeOptionsPolicy binds a policy implementation to a normalized runtime_kind.
func RegisterRuntimeOptionsPolicy(kind string, p RuntimeOptionsPolicy) {
	if kind == "" || p == nil {
		return
	}
	runtimeOptionsPolicies[kind] = p
}

// RuntimeOptionsPolicyForKind returns the registered policy, or a default no-op policy for unknown kinds.
func RuntimeOptionsPolicyForKind(kind string) RuntimeOptionsPolicy {
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
