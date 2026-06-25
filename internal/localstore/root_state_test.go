package localstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteSectionPreservesOtherRootStateSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	writeJSON(t, path, map[string]any{
		"version":         1,
		"model_providers": map[string]any{"items": map[string]any{"openai": map[string]any{}}},
		"participants":    map[string]any{"items": []any{}},
	})

	if err := WriteSection(path, "agents", map[string]any{"items": []map[string]any{{"id": "agent-manager"}}}); err != nil {
		t.Fatalf("WriteSection() error = %v", err)
	}

	var state map[string]any
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if _, ok := state["model_providers"]; !ok {
		t.Fatalf("model_providers section was not preserved: %s", data)
	}
	if _, ok := state["participants"]; !ok {
		t.Fatalf("participants section was not preserved: %s", data)
	}

	var agents struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	ok, err := ReadSection(path, "agents", &agents)
	if err != nil {
		t.Fatalf("ReadSection() error = %v", err)
	}
	if !ok || len(agents.Items) != 1 || agents.Items[0].ID != "agent-manager" {
		t.Fatalf("agents section = %#v, ok=%v", agents, ok)
	}
}
