package notifier

import agentruntime "csgclaw/internal/runtime"

type runtimeOptionsPolicy struct{}

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

func (runtimeOptionsPolicy) MergeFlatForAgentPatch(agentRuntimeOptions, patchRuntimeOptions map[string]any) map[string]any {
	out := mergeFlatForAgentPatch(agentRuntimeOptions, patchRuntimeOptions)
	return EnsurePullRemoteSubscriptionInNotifierDetails(out)
}

func (runtimeOptionsPolicy) ApplyFlatPersistence(agentRuntimeOptions *map[string]any, profileRuntimeOptions, profileRequestOptions map[string]any, mergedFlat map[string]any) (map[string]any, map[string]any) {
	return applyNotifierFlatPersistence(agentRuntimeOptions, profileRuntimeOptions, profileRequestOptions, mergedFlat)
}

func init() {
	agentruntime.RegisterRuntimeOptionsPolicy(agentruntime.KindNotifier, runtimeOptionsPolicy{})
}
