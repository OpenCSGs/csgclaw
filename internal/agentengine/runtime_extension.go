package agentengine

import (
	"context"
	"strings"
)

func normalizeRuntimeExtensionSpec(spec RuntimeExtensionSpec) RuntimeExtensionSpec {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Kind = strings.TrimSpace(spec.Kind)
	spec.Source.Provider = strings.TrimSpace(spec.Source.Provider)
	spec.Source.Ref = strings.TrimSpace(spec.Source.Ref)
	if spec.FailurePolicy == "" {
		spec.FailurePolicy = RuntimeExtensionOptional
	}
	return spec
}

func validateRuntimeExtensionSpec(spec RuntimeExtensionSpec) error {
	if spec.Name == "" || spec.Kind == "" || spec.Source.Provider == "" || spec.Source.Ref == "" {
		return &TurnError{Code: ErrorInvalidRequest, Message: "runtime extension name, kind, source provider, and source ref are required"}
	}
	if spec.FailurePolicy != RuntimeExtensionOptional && spec.FailurePolicy != RuntimeExtensionBlockRuntime {
		return &TurnError{Code: ErrorInvalidRequest, Message: "runtime extension failure policy must be optional or block_runtime"}
	}
	return nil
}

type unavailableRuntimeExtensions struct{}

func (unavailableRuntimeExtensions) Apply(context.Context, RuntimeExtensionApplyRequest) (RuntimeExtension, error) {
	return RuntimeExtension{}, &TurnError{Code: ErrorAgentUnavailable, Message: "runtime extension service is unavailable"}
}
func (unavailableRuntimeExtensions) Get(context.Context, string) (RuntimeExtension, error) {
	return RuntimeExtension{}, &TurnError{Code: ErrorAgentUnavailable, Message: "runtime extension service is unavailable"}
}
func (unavailableRuntimeExtensions) List(context.Context) ([]RuntimeExtension, error) {
	return nil, &TurnError{Code: ErrorAgentUnavailable, Message: "runtime extension service is unavailable"}
}
func (unavailableRuntimeExtensions) Delete(context.Context, string) error {
	return &TurnError{Code: ErrorAgentUnavailable, Message: "runtime extension service is unavailable"}
}
