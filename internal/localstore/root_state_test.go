package localstore

import (
	"encoding/json"
	"errors"
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

func TestUpdateObjectSectionFailurePreservesState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := WriteSection(path, "auth", map[string]string{"existing": "test-value"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("mutation failed")
	err = UpdateObjectSection(path, "auth", func(value map[string]json.RawMessage) error {
		delete(value, "existing")
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("UpdateObjectSection() error = %v, want mutation error", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("failed mutation changed the persisted state")
	}
}

func TestUpdateObjectSectionRejectsInvalidObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := WriteSection(path, "auth", []string{"invalid"}); err != nil {
		t.Fatal(err)
	}
	err := UpdateObjectSection(path, "auth", func(map[string]json.RawMessage) error {
		t.Fatal("update callback ran with malformed state")
		return nil
	})
	if err == nil {
		t.Fatal("UpdateObjectSection() accepted a non-object section")
	}
}

func TestUpdateObjectSectionMissingAndNullState(t *testing.T) {
	for _, initial := range []string{"", "null", `{"auth":null}`} {
		t.Run(initial, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			if initial != "" {
				if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := UpdateObjectSection(path, "auth", func(value map[string]json.RawMessage) error {
				value["test-provider"] = json.RawMessage(`{"value":"test"}`)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			var state map[string]json.RawMessage
			found, err := ReadSection(path, "auth", &state)
			var provider map[string]string
			if err != nil || !found || json.Unmarshal(state["test-provider"], &provider) != nil || provider["value"] != "test" {
				t.Fatalf("updated section missing, found=%v, err=%v", found, err)
			}
		})
	}
}

func TestUpdateObjectSectionDeleteMissingDoesNotCreateState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := UpdateObjectSection(path, "auth", func(value map[string]json.RawMessage) error {
		delete(value, "missing")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleting missing entry created state: %v", err)
	}
}
