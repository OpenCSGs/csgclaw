package runtime

import (
	"strings"
	"sync"
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

// ProfileExtensionsPolicy defines how runtime_extensions (and related request_options) behave
// for a concrete runtime_kind. Implementations register via RegisterProfileExtensionsPolicy.
type ProfileExtensionsPolicy interface {
	FlatFromExtensionsMap(ext map[string]any) map[string]any
	DetachPayloadFromProfileExtensions(profileExt map[string]any) map[string]any
	StripNestedFromRequestOptions(ro map[string]any)
	ProfileLLMFields(runtimeKind, baseURL, modelID string) (string, string)
	// LooksLikeFlatStorageAtRoot detects notifier-shaped flat keys at the map root (used for legacy load heuristics).
	LooksLikeFlatStorageAtRoot(m map[string]any) bool
	ProfileComplete(isGatewayRuntime, llmComplete bool, agentExt map[string]any, runtimeKind string, mergedFlat map[string]any) bool
	RequestOptionsForAgentProfileView(agentExt, requestOptions map[string]any) map[string]any
	ViewRuntimeExtensionsForAPI(agentExt, profileExt map[string]any) map[string]any
	MergeFlatForAgentPatch(agentExt, patchProfileExt map[string]any) map[string]any
	ApplyFlatPersistence(agentRE *map[string]any, profileRE, profileRO map[string]any, mergedFlat map[string]any) (map[string]any, map[string]any)
	WithPullSubscriptionDefaults(flat map[string]any) map[string]any
	RequestOptionsWithoutNested(ro map[string]any) map[string]any
}

var (
	profileExtPolicyMu   sync.RWMutex
	profileExtPolicies   = make(map[string]ProfileExtensionsPolicy)
	defaultProfilePolicy = defaultProfileExtensionsPolicy{}
)

// RegisterProfileExtensionsPolicy binds a policy implementation to a normalized runtime_kind.
func RegisterProfileExtensionsPolicy(kind string, p ProfileExtensionsPolicy) {
	kind = NormalizeRuntimeKind(kind)
	if kind == "" || p == nil {
		return
	}
	profileExtPolicyMu.Lock()
	profileExtPolicies[kind] = p
	profileExtPolicyMu.Unlock()
}

// ProfileExtensionsPolicyForKind returns the registered policy, or a default no-op policy for unknown kinds.
func ProfileExtensionsPolicyForKind(kind string) ProfileExtensionsPolicy {
	kind = NormalizeRuntimeKind(kind)
	profileExtPolicyMu.RLock()
	p, ok := profileExtPolicies[kind]
	profileExtPolicyMu.RUnlock()
	if ok {
		return p
	}
	return defaultProfilePolicy
}

type defaultProfileExtensionsPolicy struct{}

func (defaultProfileExtensionsPolicy) FlatFromExtensionsMap(map[string]any) map[string]any {
	return nil
}

func (defaultProfileExtensionsPolicy) DetachPayloadFromProfileExtensions(ext map[string]any) map[string]any {
	if len(ext) == 0 {
		return nil
	}
	return CloneAnyMap(ext)
}

func (defaultProfileExtensionsPolicy) StripNestedFromRequestOptions(map[string]any) {}

func (defaultProfileExtensionsPolicy) ProfileLLMFields(_, baseURL, modelID string) (string, string) {
	return baseURL, modelID
}

func (defaultProfileExtensionsPolicy) LooksLikeFlatStorageAtRoot(m map[string]any) bool {
	return ExtensionsHaveNotifierFlatKeys(m)
}

func (defaultProfileExtensionsPolicy) ProfileComplete(isGatewayRuntime, llmComplete bool, _ map[string]any, _ string, _ map[string]any) bool {
	if isGatewayRuntime {
		return llmComplete
	}
	return llmComplete
}

func (defaultProfileExtensionsPolicy) RequestOptionsForAgentProfileView(_ map[string]any, requestOptions map[string]any) map[string]any {
	return RedactedRequestOptionsForAPIView(requestOptions)
}

func (defaultProfileExtensionsPolicy) ViewRuntimeExtensionsForAPI(agentExt, profileExt map[string]any) map[string]any {
	return MergeRuntimeExtensionMapsForView(agentExt, profileExt)
}

func (defaultProfileExtensionsPolicy) MergeFlatForAgentPatch(_, _ map[string]any) map[string]any {
	return nil
}

func (defaultProfileExtensionsPolicy) ApplyFlatPersistence(_ *map[string]any, profileRE, profileRO map[string]any, mergedFlat map[string]any) (map[string]any, map[string]any) {
	if len(mergedFlat) == 0 {
		return profileRE, profileRO
	}
	return profileRE, profileRO
}

func (defaultProfileExtensionsPolicy) WithPullSubscriptionDefaults(flat map[string]any) map[string]any {
	return flat
}

func (defaultProfileExtensionsPolicy) RequestOptionsWithoutNested(ro map[string]any) map[string]any {
	out := CloneAnyMap(ro)
	if len(out) == 0 {
		return nil
	}
	delete(out, "notifier")
	if len(out) == 0 {
		return nil
	}
	return out
}
