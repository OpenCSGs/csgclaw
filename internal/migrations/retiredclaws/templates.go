package retiredclaws

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	toml "github.com/pelletier/go-toml/v2"
)

var runtimeTemplateMigrationMu sync.Mutex

// Existing remote templates cannot be rewritten locally. Adapt their manifest
// on read; validation for new publications still rejects retired runtimes.
func TemplateManifest(kind *string, options *map[string]any, imageRef *string) {
	if !isRetiredRuntimeKind(*kind) {
		return
	}
	*kind = "codex"
	*options = nil
	*imageRef = ""
}

// Upgrade local manifests before either default-template resolution or Hub
// listing. Backups stay outside the registry so they cannot reappear as templates.
func LocalTemplates(root string) error {
	runtimeTemplateMigrationMu.Lock()
	defer runtimeTemplateMigrationMu.Unlock()
	entries, err := os.ReadDir(filepath.Join(root, "templates"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read local templates for runtime migration: %w", err)
	}
	type update struct {
		path         string
		data         []byte
		mode         os.FileMode
		instructions string
	}
	var updates []update
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, "templates", entry.Name(), "agent.toml")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		} // Invalid/unreadable entries retain the existing List/Get diagnostics.
		var manifest map[string]any
		if err := toml.Unmarshal(data, &manifest); err != nil {
			continue
		}
		kind, _ := manifest["runtime_kind"].(string)
		if !isRetiredRuntimeKind(kind) {
			continue
		}
		manifest["runtime_kind"] = "codex"
		delete(manifest, "runtime_options")
		if image, ok := manifest["image"].(map[string]any); ok {
			delete(image, "ref")
			if len(image) == 0 {
				delete(manifest, "image")
			}
		}
		next, err := toml.Marshal(manifest)
		if err != nil {
			return err
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		instructions := ""
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(kind)), "picoclaw") {
			for _, dir := range []string{"instructions", "workspace"} {
				candidate := filepath.Join(filepath.Dir(path), dir, "AGENT.md")
				if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
					if _, err := os.Stat(filepath.Join(filepath.Dir(candidate), "AGENTS.md")); errors.Is(err, os.ErrNotExist) {
						instructions = candidate
						break
					} else if err != nil {
						return err
					}
				} else if err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
		}
		updates = append(updates, update{path, next, info.Mode().Perm(), instructions})
	}
	if len(updates) == 0 {
		return nil
	}
	if err := backupOnce(root); err != nil {
		return fmt.Errorf("backup retired local templates: %w", err)
	}
	for _, item := range updates {
		if item.instructions != "" {
			if err := os.Rename(item.instructions, filepath.Join(filepath.Dir(item.instructions), "AGENTS.md")); err != nil {
				return err
			}
		}
		if err := writeTemplateMigrationFile(item.path, item.data, item.mode); err != nil {
			return err
		}
	}
	return nil
}

func writeTemplateMigrationFile(path string, data []byte, mode os.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".runtime-migration-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(mode); err == nil {
		_, err = temp.Write(data)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temp.Name(), path)
}
