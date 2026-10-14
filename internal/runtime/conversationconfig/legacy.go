package conversationconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ImportLegacy moves the unpublished per-room layout back into the Agent's
// native store. Original files remain available until migration is verified.
func ImportLegacy(agentHome, runtimeDir, metadataFile string, fields, nativePaths []string) error {
	legacyHomes, err := filepath.Glob(filepath.Join(agentHome, "task-sessions", "*", runtimeDir))
	if err != nil {
		return err
	}
	for _, legacy := range legacyHomes {
		marker := filepath.Join(legacy, "imported-into-agent")
		if _, err := os.Stat(marker); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		old, err := readObject(filepath.Join(legacy, metadataFile))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		root := filepath.Join(agentHome, runtimeDir)
		current, err := readObject(filepath.Join(root, metadataFile))
		if os.IsNotExist(err) {
			current = map[string]json.RawMessage{}
		} else if err != nil {
			return err
		}
		for _, relative := range nativePaths {
			if err := copyNativeFiles(filepath.Join(legacy, relative), filepath.Join(root, relative)); err != nil {
				return err
			}
		}
		for _, field := range fields {
			merged := map[string]json.RawMessage{}
			if data := current[field]; len(data) > 0 {
				if err := json.Unmarshal(data, &merged); err != nil {
					return err
				}
			}
			if merged == nil {
				merged = map[string]json.RawMessage{}
			}
			previous := map[string]json.RawMessage{}
			if data := old[field]; len(data) > 0 {
				if err := json.Unmarshal(data, &previous); err != nil {
					return err
				}
			}
			for key, value := range previous {
				merged[key] = value
			}
			current[field], err = json.Marshal(merged)
			if err != nil {
				return err
			}
		}
		data, err := json.Marshal(current)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(root, 0700); err != nil {
			return err
		}
		if err := writeAtomic(filepath.Join(root, metadataFile), data); err != nil {
			return err
		}
		if err := writeAtomic(marker, []byte("1")); err != nil {
			return err
		}
	}
	return nil
}

func readObject(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var object map[string]json.RawMessage
	err = json.Unmarshal(data, &object)
	if err == nil && object == nil {
		err = fmt.Errorf("invalid runtime metadata: %s", path)
	}
	return object, err
}

func copyNativeFiles(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if path == source && os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported native session file: %s", path)
		}
		if filepath.Ext(path) == ".lock" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if existing, err := os.ReadFile(target); err == nil {
			if !bytes.Equal(existing, data) {
				return fmt.Errorf("native session migration conflict: %s", target)
			}
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		return writeAtomic(target, data)
	})
}

func writeAtomic(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".session-import-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}
