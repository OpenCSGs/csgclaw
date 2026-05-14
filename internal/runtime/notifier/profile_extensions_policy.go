package notifier

import agentruntime "csgclaw/internal/runtime"

type profileExtensionsPolicy struct{}

func (profileExtensionsPolicy) FlatFromExtensionsMap(ext map[string]any) map[string]any {
	return NotifierFlatFromRuntimeExtensionsMap(ext)
}

func (profileExtensionsPolicy) DetachPayloadFromProfileExtensions(profileExt map[string]any) map[string]any {
	return ProfileRuntimeExtensionsWithoutNotifierPayload(profileExt)
}

func (profileExtensionsPolicy) StripNestedFromRequestOptions(ro map[string]any) {
	StripNestedNotifier(ro)
}

func (profileExtensionsPolicy) ProfileLLMFields(runtimeKind, baseURL, modelID string) (string, string) {
	return ProfileLLMEndpointFieldsForRuntime(runtimeKind, baseURL, modelID)
}

func (profileExtensionsPolicy) LooksLikeFlatStorageAtRoot(m map[string]any) bool {
	return IsNotifierFlatRoot(m)
}

func (profileExtensionsPolicy) ProfileComplete(isGatewayRuntime, llmComplete bool, agentExt map[string]any, runtimeKind string, mergedFlat map[string]any) bool {
	return ProfileCompleteFromAgentExtensions(isGatewayRuntime, llmComplete, agentExt, runtimeKind, mergedFlat)
}

func (profileExtensionsPolicy) RequestOptionsForAgentProfileView(agentExt, requestOptions map[string]any) map[string]any {
	return requestOptionsForAgentProfileView(agentExt, requestOptions)
}

func (profileExtensionsPolicy) ViewRuntimeExtensionsForAPI(agentExt, profileExt map[string]any) map[string]any {
	return ViewRuntimeExtensionsForAPIUnified(agentExt, profileExt)
}

func (profileExtensionsPolicy) MergeFlatForAgentPatch(agentExt, patchProfileExt map[string]any) map[string]any {
	return mergeFlatForAgentPatch(agentExt, patchProfileExt)
}

func (profileExtensionsPolicy) ApplyFlatPersistence(agentRE *map[string]any, profileRE, profileRO map[string]any, mergedFlat map[string]any) (map[string]any, map[string]any) {
	return applyNotifierFlatPersistence(agentRE, profileRE, profileRO, mergedFlat)
}

func (profileExtensionsPolicy) WithPullSubscriptionDefaults(flat map[string]any) map[string]any {
	return EnsurePullRemoteSubscriptionInNotifierDetails(flat)
}

func (profileExtensionsPolicy) RequestOptionsWithoutNested(ro map[string]any) map[string]any {
	return RequestOptionsWithoutNestedNotifier(ro)
}

func init() {
	agentruntime.RegisterProfileExtensionsPolicy(agentruntime.KindNotifier, profileExtensionsPolicy{})
}
