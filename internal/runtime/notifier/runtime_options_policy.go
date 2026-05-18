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

func init() {
	agentruntime.RegisterRuntimeOptionsPolicy(agentruntime.KindNotifier, runtimeOptionsPolicy{})
}
