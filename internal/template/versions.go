package template

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"csgclaw/internal/agentworkspace"
	"csgclaw/internal/apitypes"
	"github.com/google/uuid"
	toml "github.com/pelletier/go-toml/v2"
)

const versionStateFile = ".versions.json"
const versionSnapshotsDir = ".versions"
const retainedTemplateVersions = 10

type VersionAuthor struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type TemplateVersion struct {
	Revision     int            `json:"revision"`
	Message      string         `json:"message,omitempty"`
	Author       *VersionAuthor `json:"author,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	RestoredFrom int            `json:"restored_from,omitempty"`
}

type TemplateRollback struct {
	From      int            `json:"from"`
	To        int            `json:"to"`
	Message   string         `json:"message,omitempty"`
	Author    *VersionAuthor `json:"author,omitempty"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type templateVersionState struct {
	LastRollback  *TemplateRollback              `json:"last_rollback,omitempty"`
	Publications  []apitypes.TemplatePublication `json:"publications,omitempty"`
	SourceAgentID string                         `json:"source_agent_id,omitempty"`
	Versions      []TemplateVersion              `json:"versions"`
}

func readVersionState(root string) (templateVersionState, error) {
	var state templateVersionState
	data, err := os.ReadFile(filepath.Join(root, versionStateFile))
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("read template versions: %w", err)
	}
	return state, nil
}

// SaveAgent commits a complete snapshot and its association as one directory swap.
// The name is only used to claim an unassociated legacy template. Subsequent
// saves resolve the persisted agent association even after a display-name change.
func (s *LocalStore) SaveAgent(ctx context.Context, spec PublishSpec) (Template, error) {
	localTemplateReplaceMu.Lock()
	defer localTemplateReplaceMu.Unlock()
	spec.SourceAgentID = strings.TrimSpace(spec.SourceAgentID)
	if spec.SourceAgentID == "" {
		return Template{}, fmt.Errorf("source agent id is required")
	}
	items, err := s.List(ctx)
	if err != nil {
		return Template{}, err
	}
	for _, item := range items {
		state, err := readVersionState(s.templateRoot(item.ID))
		if err != nil {
			return Template{}, err
		}
		if state.SourceAgentID == spec.SourceAgentID {
			spec.ID = item.ID
			break
		}
	}
	// Resolve the final target before checking ownership: an existing Agent
	// association takes precedence over the requested name or ID.
	normalized, err := normalizePublishSpec(spec)
	if err != nil {
		return Template{}, err
	}
	current, err := s.Get(ctx, normalized.ID)
	if err == nil {
		if current.SourceAgentID != "" && current.SourceAgentID != normalized.SourceAgentID {
			return Template{}, fmt.Errorf("%w: template %q is associated with another agent", ErrTemplateAlreadyExists, normalized.ID)
		}
	} else if !errors.Is(err, ErrTemplateNotFound) && !errors.Is(err, os.ErrNotExist) {
		return Template{}, err
	}
	return s.commitVersion(ctx, normalized, 0)
}

func copyVersionContent(source, target string) error {
	return copyWorkspaceTreeWithFilter(source, target, func(path string, _ fs.DirEntry) bool {
		return path == versionStateFile || path == versionSnapshotsDir
	})
}

// preparedRoot optionally supplies an edited canonical layout so manifests and
// files outside the runtime workspace survive instruction-only saves.
func (s *LocalStore) commitVersion(ctx context.Context, spec PublishSpec, restoredFrom int, preparedRoot ...string) (Template, error) {
	normalized, err := normalizePublishSpec(spec)
	if err != nil {
		return Template{}, err
	}
	spec = normalized
	current, err := s.Get(ctx, spec.ID)
	exists := err == nil
	if err != nil && !errors.Is(err, ErrTemplateNotFound) && !errors.Is(err, os.ErrNotExist) {
		return Template{}, err
	}
	state := templateVersionState{}
	if exists {
		state, err = readVersionState(s.templateRoot(spec.ID))
		if err != nil {
			return Template{}, err
		}
		spec.UID = current.UID
	}
	if spec.UID == "" {
		spec.UID = uuid.NewString()
	}
	if exists && len(state.Versions) == 0 {
		revision := current.Revision
		if revision < 1 {
			revision = 1
		}
		state.Versions = append(state.Versions, TemplateVersion{Revision: revision, CreatedAt: current.UpdatedAt})
	}
	spec.Revision = 1
	if len(state.Versions) > 0 {
		spec.Revision = state.Versions[len(state.Versions)-1].Revision + 1
	}
	spec.Version = "V" + strconv.Itoa(spec.Revision)
	spec.UpdatedAt = time.Now().UTC()
	if spec.SourceAgentID != "" {
		state.SourceAgentID = spec.SourceAgentID
	}
	state.Versions = append(state.Versions, TemplateVersion{Revision: spec.Revision, Message: strings.TrimSpace(spec.CommitMessage), Author: spec.Author, CreatedAt: spec.UpdatedAt, RestoredFrom: restoredFrom})
	if len(state.Versions) > retainedTemplateVersions {
		state.Versions = state.Versions[len(state.Versions)-retainedTemplateVersions:]
	}
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return Template{}, err
	}
	stagingRoot, err := os.MkdirTemp(s.root, ".template-version-")
	if err != nil {
		return Template{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(stagingRoot)
		}
	}()
	staging := NewLocalStore(stagingRoot)
	target := staging.templateRoot(spec.ID)
	if len(preparedRoot) > 0 {
		if err := copyVersionContent(preparedRoot[0], target); err != nil {
			return Template{}, err
		}
		_, manifest, err := s.loadTemplate(spec.ID)
		if err != nil {
			return Template{}, err
		}
		manifest.UID, manifest.Revision, manifest.Version = spec.UID, spec.Revision, spec.Version
		manifest.UpdatedAt = spec.UpdatedAt.Format(time.RFC3339Nano)
		data, err := toml.Marshal(manifest)
		if err != nil {
			return Template{}, err
		}
		if err := os.WriteFile(filepath.Join(target, localManifestFileName), data, 0o644); err != nil {
			return Template{}, err
		}
	} else if _, err := staging.Publish(ctx, spec); err != nil {
		return Template{}, err
	}

	for _, version := range state.Versions {
		snapshot := filepath.Join(target, versionSnapshotsDir, strconv.Itoa(version.Revision))
		source := target
		if version.Revision != spec.Revision {
			source = filepath.Join(s.templateRoot(spec.ID), versionSnapshotsDir, strconv.Itoa(version.Revision))
			if len(state.Versions) == 2 && version.Revision == state.Versions[0].Revision {
				if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
					source = s.templateRoot(spec.ID)
				}
			}
		}
		if err := copyVersionContent(source, snapshot); err != nil {
			return Template{}, err
		}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return Template{}, err
	}
	if err := os.WriteFile(filepath.Join(target, versionStateFile), data, 0o600); err != nil {
		return Template{}, err
	}
	if err := os.MkdirAll(s.templatesRoot(), 0o755); err != nil {
		return Template{}, err
	}
	original := s.templateRoot(spec.ID)
	backup := filepath.Join(stagingRoot, "previous")
	if exists {
		if err := os.Rename(original, backup); err != nil {
			return Template{}, err
		}
	}
	if err := os.Rename(target, original); err != nil {
		if exists {
			if restoreErr := os.Rename(backup, original); restoreErr != nil {
				cleanup = false
				return Template{}, fmt.Errorf("commit failed: %w; restore failed: %v; backup: %s", err, restoreErr, backup)
			}
		}
		return Template{}, err
	}
	return s.Get(ctx, spec.ID)
}

func (s *LocalStore) Versions(ctx context.Context, id string) ([]TemplateVersion, error) {
	localTemplateReplaceMu.Lock()
	defer localTemplateReplaceMu.Unlock()
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	state, err := readVersionState(s.templateRoot(id))
	if state.Versions == nil {
		state.Versions = []TemplateVersion{}
	}
	return state.Versions, err
}

func (s *LocalStore) versionRoot(ctx context.Context, id string, revision int) (string, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return "", err
	}
	state, err := readVersionState(s.templateRoot(id))
	if err != nil {
		return "", err
	}
	for _, version := range state.Versions {
		if version.Revision == revision {
			return filepath.Join(s.templateRoot(id), versionSnapshotsDir, strconv.Itoa(revision)), nil
		}
	}
	return "", ErrTemplateNotFound
}

func (s *LocalStore) ListVersionWorkspace(ctx context.Context, id string, revision int, path string) (apitypes.WorkspaceListing, error) {
	localTemplateReplaceMu.Lock()
	defer localTemplateReplaceMu.Unlock()
	root, err := s.versionRoot(ctx, id, revision)
	if err != nil {
		return apitypes.WorkspaceListing{}, err
	}
	return agentworkspace.ListDirectory(root, path)
}

func (s *LocalStore) ReadVersionFile(ctx context.Context, id string, revision int, path string) (apitypes.WorkspaceFile, error) {
	localTemplateReplaceMu.Lock()
	defer localTemplateReplaceMu.Unlock()
	root, err := s.versionRoot(ctx, id, revision)
	if err != nil {
		return apitypes.WorkspaceFile{}, err
	}
	return agentworkspace.ReadFile(root, path)
}

func (s *LocalStore) RestoreVersion(ctx context.Context, id string, revision int, message string, author *VersionAuthor) (Template, error) {
	localTemplateReplaceMu.Lock()
	defer localTemplateReplaceMu.Unlock()
	root, err := s.versionRoot(ctx, id, revision)
	if err != nil {
		return Template{}, err
	}
	_, manifest, err := loadManifestFS(os.DirFS(filepath.Dir(root)), filepath.Join(filepath.Base(root), localManifestFileName), "template version")
	if err != nil {
		return Template{}, err
	}
	current, err := s.Get(ctx, id)
	if err != nil {
		return Template{}, err
	}
	if current.Revision == revision {
		return current, nil
	}
	if err := ctx.Err(); err != nil {
		return Template{}, err
	}
	state, err := readVersionState(s.templateRoot(id))
	if err != nil {
		return Template{}, err
	}
	now := time.Now().UTC()
	state.LastRollback = &TemplateRollback{From: current.Revision, To: revision, Message: strings.TrimSpace(message), Author: author, UpdatedAt: now}
	stagingRoot, err := os.MkdirTemp(s.root, ".template-rollback-")
	if err != nil {
		return Template{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(stagingRoot)
		}
	}()
	target := filepath.Join(stagingRoot, "current")
	if err := copyVersionContent(root, target); err != nil {
		return Template{}, err
	}
	// Active content identifies the historical revision. History and the highest
	// allocated revision remain unchanged, so the next save cannot reuse a number.
	manifest.UID = current.UID
	manifest.Revision = revision
	manifest.Version = "V" + strconv.Itoa(revision)
	manifest.UpdatedAt = now.Format(time.RFC3339Nano)
	data, err := toml.Marshal(manifest)
	if err != nil {
		return Template{}, err
	}
	if err := os.WriteFile(filepath.Join(target, localManifestFileName), data, 0o644); err != nil {
		return Template{}, err
	}
	for _, version := range state.Versions {
		source := filepath.Join(s.templateRoot(id), versionSnapshotsDir, strconv.Itoa(version.Revision))
		destination := filepath.Join(target, versionSnapshotsDir, strconv.Itoa(version.Revision))
		if err := copyVersionContent(source, destination); err != nil {
			return Template{}, err
		}
	}
	data, err = json.MarshalIndent(state, "", "  ")
	if err != nil {
		return Template{}, err
	}
	if err := os.WriteFile(filepath.Join(target, versionStateFile), data, 0o600); err != nil {
		return Template{}, err
	}
	if err := ctx.Err(); err != nil {
		return Template{}, err
	}
	original := s.templateRoot(id)
	backup := filepath.Join(stagingRoot, "previous")
	if err := os.Rename(original, backup); err != nil {
		return Template{}, err
	}
	if err := os.Rename(target, original); err != nil {
		if restoreErr := os.Rename(backup, original); restoreErr != nil {
			cleanup = false
			return Template{}, fmt.Errorf("rollback failed: %w; restore failed: %v; backup retained at %s", err, restoreErr, backup)
		}
		return Template{}, err
	}
	return s.Get(ctx, id)
}

func (s *LocalStore) recordPublication(ctx context.Context, id, expectedUID string, publication apitypes.TemplatePublication) error {
	localTemplateReplaceMu.Lock()
	defer localTemplateReplaceMu.Unlock()
	current, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if current.UID != expectedUID {
		return fmt.Errorf("local template identity changed during publication")
	}
	root := s.templateRoot(id)
	state, err := readVersionState(root)
	if err != nil {
		return err
	}
	replaced := false
	for i := range state.Publications {
		if state.Publications[i].Site == publication.Site && state.Publications[i].Namespace == publication.Namespace {
			state.Publications[i] = publication
			replaced = true
			break
		}
	}
	if !replaced {
		state.Publications = append(state.Publications, publication)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(root, ".publication-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(root, versionStateFile))
}

// ensureIdentityLocked upgrades legacy manifests without associating an Agent.
// The caller holds localTemplateReplaceMu.
func (s *LocalStore) ensureIdentityLocked(id string) error {
	_, manifest, err := s.loadTemplate(id)
	if err != nil {
		return err
	}
	if manifest.UID != "" {
		return nil
	}
	manifest.UID = uuid.NewString()
	data, err := toml.Marshal(manifest)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(s.templateRoot(id), ".identity-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), s.manifestPath(id))
}
