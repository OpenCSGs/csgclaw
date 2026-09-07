package agents

import (
	"csgclaw/internal/agentengine/contract"
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
