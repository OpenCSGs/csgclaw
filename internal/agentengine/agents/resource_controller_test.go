package agents

import (
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelprovider"
	"testing"
)

func TestPreserveWriteOnlyFieldsDistinguishesOmittedAndExplicitClear(t *testing.T) {
	current := contract.AgentSpec{
		Runtime: contract.RuntimeSpec{Credentials: map[string]string{"auth.json": "secret"}},
		Model:   contract.ModelSpec{APIKey: "model-secret"},
	}
	preserved := preserveWriteOnlyFields(current, contract.AgentSpec{})
	if preserved.Runtime.Credentials["auth.json"] != "secret" || preserved.Model.APIKey != "model-secret" {
		t.Fatalf("preserved write-only fields = %+v", preserved)
	}
	cleared := preserveWriteOnlyFields(current, contract.AgentSpec{Runtime: contract.RuntimeSpec{Credentials: map[string]string{}}})
	if cleared.Runtime.Credentials == nil || len(cleared.Runtime.Credentials) != 0 {
		t.Fatalf("explicit empty credentials = %#v, want explicit clear", cleared.Runtime.Credentials)
	}
}

func TestSpecFromServiceIncludesVideoGenerationModel(t *testing.T) {
	controller := &Controller{}
	selected := Agent{
		ID:          "agent-manager",
		Name:        "manager",
		Role:        RoleManager,
		RuntimeName: RuntimeNameCodex,
		AgentProfile: AgentProfile{
			VideoGeneration: &modelprovider.VideoGenerationConfig{ProviderID: ModelProviderIDOpenCSG, ModelID: "MiniMax-Hailuo-2.3"},
		},
	}
	spec, err := controller.specFromService(selected, nil, false)
	if err != nil {
		t.Fatalf("specFromService() error = %v", err)
	}
	if spec.Model.VideoGeneration == nil || spec.Model.VideoGeneration.ProviderID != ModelProviderIDOpenCSG || spec.Model.VideoGeneration.ModelID != "MiniMax-Hailuo-2.3" {
		t.Fatalf("video generation model = %#v", spec.Model.VideoGeneration)
	}
}
