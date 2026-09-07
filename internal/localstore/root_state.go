package localstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const RootStateFileName = "state.json"

var rootStateMu sync.Mutex

func IsRootStatePath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || filepath.Base(path) != RootStateFileName {
		return false
	}
	parent := filepath.Base(filepath.Dir(path))
	return parent != "agents" && parent != "im"
}

func ReadSection(path, section string, target any) (bool, error) {
	rootStateMu.Lock()
	defer rootStateMu.Unlock()

	path = strings.TrimSpace(path)
	section = strings.TrimSpace(section)
	if path == "" || section == "" {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read root state: %w", err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(data, &state); err != nil {
		return false, fmt.Errorf("decode root state: %w", err)
	}
	raw, ok := state[section]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return false, nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return true, fmt.Errorf("decode root state section %q: %w", section, err)
	}
	return true, nil
}

func WriteSection(path, section string, value any) error {
	return updateSection(path, section, func(json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(value)
	})
}

// UpdateObjectSection reads, modifies, and writes a JSON object under the root
// state lock, preserving concurrent updates to other keys and sections.
// The callback must not call other localstore functions.
func UpdateObjectSection(path, section string, update func(map[string]json.RawMessage) error) error {
	return updateSection(path, section, func(raw json.RawMessage) (json.RawMessage, error) {
		value := make(map[string]json.RawMessage)
		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, fmt.Errorf("decode root state section %q: %w", section, err)
			}
		}
		if value == nil {
			value = make(map[string]json.RawMessage)
		}
		if err := update(value); err != nil {
			return nil, err
		}
		if len(value) == 0 && (len(raw) == 0 || string(raw) == "null") {
			return nil, nil
		}
		return json.Marshal(value)
	})
}

func updateSection(path, section string, update func(json.RawMessage) (json.RawMessage, error)) error {
	rootStateMu.Lock()
	defer rootStateMu.Unlock()

	path = strings.TrimSpace(path)
	section = strings.TrimSpace(section)
	if path == "" {
		return nil
	}
	if section == "" {
		return fmt.Errorf("root state section is required")
	}

	state := make(map[string]json.RawMessage)
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &state); err != nil {
			return fmt.Errorf("decode root state: %w", err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read root state: %w", err)
	}

	if state == nil {
		state = make(map[string]json.RawMessage)
	}
	raw, err := update(state[section])
	if err != nil {
		return fmt.Errorf("update root state section %q: %w", section, err)
	}
	if raw == nil {
		return nil
	}
	state["version"] = json.RawMessage("1")
	state[section] = raw

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode root state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create root state dir: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
