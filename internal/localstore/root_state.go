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

	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode root state section %q: %w", section, err)
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
