package larkcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentruntime "csgclaw/internal/runtime"
)

type LegacyProjection struct {
	Directory  string                           `json:"directory"`
	Projection agentruntime.ExtensionProjection `json:"projection"`
}

// CopyPrivateTree is used only for migration recovery material, outside normal
// Runtime generation cleanup. Symlinks and special files are rejected.
func CopyPrivateTree(source, target string) error {
	return filepath.WalkDir(source, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, name)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("migration source contains a symlink")
		}
		if entry.IsDir() {
			return os.MkdirAll(dest, 0700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("migration source contains a special file")
		}
		if info.Size() > 32<<20 {
			return errors.New("migration file exceeds size limit")
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		mode := os.FileMode(0600)
		if info.Mode()&0111 != 0 {
			mode = 0700
		}
		return os.WriteFile(dest, data, mode)
	})
}
func SaveLegacyProjection(current agentruntime.ExtensionProjection, directory string) (*LegacyProjection, error) {
	source := filepath.Dir(current.Environment["LARKSUITE_CLI_CONFIG_DIR"])
	if !filepath.IsAbs(source) || filepath.Base(source) != current.Root {
		return nil, errors.New("legacy CLI generation is invalid")
	}
	if data, err := os.ReadFile(filepath.Join(directory, "migration-projection.json")); err == nil {
		var saved LegacyProjection
		if err := json.Unmarshal(data, &saved); err != nil {
			return nil, err
		}
		if saved.Directory != directory || saved.Projection.Generation != current.Generation || saved.Projection.SourceRevision != current.SourceRevision {
			return nil, errors.New("migration snapshot identity changed")
		}
		return &saved, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	stage := directory + ".preparing"
	_ = os.RemoveAll(stage)
	if err := CopyPrivateTree(source, stage); err != nil {
		return nil, err
	}
	restored := current
	restored.Environment = maps.Clone(current.Environment)
	for key, value := range restored.Environment {
		if value == source || strings.HasPrefix(value, source+string(filepath.Separator)) {
			restored.Environment[key] = directory + strings.TrimPrefix(value, source)
		}
	}
	saved := &LegacyProjection{Directory: directory, Projection: restored}
	data, err := json.Marshal(saved)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(stage, "migration-projection.json"), data, 0600); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(directory), 0700); err != nil {
		return nil, err
	}
	if err := os.Rename(stage, directory); err != nil {
		return nil, err
	}
	return saved, nil
}
func (d *Driver) prepareLegacy(ctx context.Context, agentID string, desired agentruntime.ExtensionDesired, p Payload) (agentruntime.PreparedExtension, agentruntime.ExtensionResult, error) {
	now := time.Now().UTC()
	legacy := p.Legacy
	if legacy == nil || p.AgentID != agentID || !filepath.IsAbs(legacy.Directory) {
		return nil, errorResult("migration_failed", errors.New("invalid migration recovery material"), now), errors.New("invalid migration recovery material")
	}
	cfg := filepath.Join(legacy.Projection.Environment["LARKSUITE_CLI_CONFIG_DIR"], WorkspaceName, ConfigFileName)
	if appID, ok := readConfigAppID(cfg); !ok || appID != p.AppID {
		return nil, errorResult("migration_failed", errors.New("original CLI configuration is unavailable"), now), errors.New("original CLI configuration is unavailable")
	}
	_, store, err := d.store(agentID)
	if err != nil {
		return nil, agentruntime.ExtensionResult{}, err
	}
	change, err := store.Stage(desired.Name)
	if err != nil {
		return nil, agentruntime.ExtensionResult{}, err
	}
	current := legacy.Projection
	current.Name = desired.Name
	current.Generation = desired.Generation
	current.SourceRevision = desired.SourceRevision
	change.SetProjection(current)
	return change, agentruntime.ExtensionResult{State: agentruntime.ExtensionStateConfigured, Reason: "migration_recovered", CheckedAt: now}, nil
}
func (d *Driver) observeLegacy(_ context.Context, agentID string, desired agentruntime.ExtensionDesired, p Payload) (agentruntime.ExtensionResult, error) {
	_, store, err := d.store(agentID)
	if err != nil {
		return agentruntime.ExtensionResult{}, err
	}
	current, found, err := store.Load(desired.Name)
	if err != nil {
		return agentruntime.ExtensionResult{}, err
	}
	if !found || current.SourceRevision != desired.SourceRevision || current.Generation != desired.Generation {
		return errorResult("migration_failed", fmt.Errorf("original CLI configuration is not active"), time.Now().UTC()), nil
	}
	return agentruntime.ExtensionResult{State: agentruntime.ExtensionStateConfigured, Reason: "migration_recovered", CheckedAt: time.Now().UTC(), RuntimeLoaded: d.options.RuntimeLoaded != nil && d.options.RuntimeLoaded(agentID, current)}, nil
}
