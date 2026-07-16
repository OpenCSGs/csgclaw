package agent

import "testing"

func TestAgentMetadataReturnsPersistedFieldsThroughLegacyAlias(t *testing.T) {
	svc := &Service{agents: map[string]Agent{
		"agent-writer": {
			ID:          "agent-writer",
			Name:        " Writer ",
			Description: " Release writer ",
			Role:        " worker ",
		},
	}}

	got, ok := svc.AgentMetadata("u-writer")
	if !ok {
		t.Fatal("AgentMetadata() ok = false, want true")
	}
	if got.ID != "agent-writer" || got.Name != "Writer" || got.Description != "Release writer" || got.Role != "worker" {
		t.Fatalf("AgentMetadata() = %+v, want trimmed persisted metadata", got)
	}
}
