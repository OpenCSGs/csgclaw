package notifier

import agentruntime "csgclaw/internal/runtime"

type runtimeOptionsPolicy struct{}

func (runtimeOptionsPolicy) StripNestedFromRequestOptions(ro map[string]any) {
	StripNestedNotifier(ro)
}

func (runtimeOptionsPolicy) StripProfileLLMFields(runtimeKind, baseURL, modelID string) (string, string) {
	return StripProfileLLMFieldsForRuntime(runtimeKind, baseURL, modelID)
}

func (runtimeOptionsPolicy) IsComplete(_ bool, runtimeOptions, runtimeOptionsAfterPatch map[string]any) bool {
	opts := runtimeOptionsAfterPatch
	if len(opts) == 0 {
		opts = NotifierFlatFromRuntimeOptionsMap(runtimeOptions)
	}
	return ProfileDeliveryComplete(opts)
}

func (runtimeOptionsPolicy) RequestOptionsForAgentProfileView(agentExt, requestOptions map[string]any) map[string]any {
	return requestOptionsForAgentProfileView(agentExt, requestOptions)
}

func (runtimeOptionsPolicy) MergeFlatForAgentPatch(agentRuntimeOptions, patchRuntimeOptions map[string]any) map[string]any {
	return mergeFlatForAgentPatch(agentRuntimeOptions, patchRuntimeOptions)
}

func (runtimeOptionsPolicy) ApplyFlatPersistence(agentRE *map[string]any, profileRE, profileRO map[string]any, mergedFlat map[string]any) (map[string]any, map[string]any) {
	return applyNotifierFlatPersistence(agentRE, profileRE, profileRO, mergedFlat)
}

func (runtimeOptionsPolicy) WithPullSubscriptionDefaults(flat map[string]any) map[string]any {
	return EnsurePullRemoteSubscriptionInNotifierDetails(flat)
}

func (runtimeOptionsPolicy) RequestOptionsWithoutNested(ro map[string]any) map[string]any {
	return RequestOptionsWithoutNestedNotifier(ro)
}

func init() {
	agentruntime.RegisterRuntimeOptionsPolicy(agentruntime.KindNotifier, runtimeOptionsPolicy{})
}
