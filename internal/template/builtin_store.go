package template

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	hubtemplates "csgclaw/internal/template/embed"
	toml "github.com/pelletier/go-toml/v2"
)

type BuiltinStore struct{}

func NewBuiltinStore() *BuiltinStore {
	return &BuiltinStore{}
}

func (s *BuiltinStore) List(context.Context) ([]Template, error) {
	builtins := hubtemplates.Builtins()
	ids := make([]string, 0, len(builtins))
	for _, item := range builtins {
		id := strings.TrimSpace(item.ID)
		if err := validateLocalTemplateID(id); err != nil {
			return nil, fmt.Errorf("invalid builtin hub template %q: %w", id, err)
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)

	items := make([]Template, 0, len(ids))
	for _, id := range ids {
		item, err := s.Get(context.Background(), id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *BuiltinStore) Get(_ context.Context, id string) (Template, error) {
	id = strings.TrimSpace(id)
	manifest, err := s.loadManifest(id)
	if err != nil {
		return Template{}, err
	}
	updatedAt, err := parseManifestUpdatedAt(manifest.UpdatedAt)
	if err != nil {
		return Template{}, fmt.Errorf("validate builtin hub manifest %q: %w", id, err)
	}
	return Template{
		ID:           id,
		Name:         manifest.Name,
		Description:  manifest.Description,
		Role:         normalizeTemplateRole(manifest.Role),
		RuntimeKind:  normalizeTemplateRuntimeKind(manifest.RuntimeKind),
		Version:      strings.TrimSpace(manifest.Version),
		Image:        manifestImageRef(manifest.Image),
		ImageEnv:     manifestImageEnv(manifest.Image),
		WorkspaceRef: s.workspaceRef(id),
		UpdatedAt:    updatedAt,
	}, nil
}

func (s *BuiltinStore) FetchWorkspace(_ context.Context, id string) (WorkspaceRef, error) {
	id = strings.TrimSpace(id)
	if err := validateLocalTemplateID(id); err != nil {
		return WorkspaceRef{}, err
	}
	if _, err := s.loadManifest(id); err != nil {
		return WorkspaceRef{}, err
	}
	if ref := s.workspaceRef(id); strings.TrimSpace(ref.Path) == "" {
		return WorkspaceRef{}, nil
	}
	root := s.workspacePath(id)
	tmpDir, err := mkdirHubWorkspaceTemp("csgclaw-hub-builtin-*")
	if err != nil {
		return WorkspaceRef{}, fmt.Errorf("create builtin hub workspace temp dir: %w", err)
	}
	if err := copyWorkspaceTreeFS(hubtemplates.FS(), root, tmpDir, "builtin hub workspace"); err != nil {
		_ = os.RemoveAll(tmpDir)
		return WorkspaceRef{}, err
	}
	return WorkspaceRef{Kind: WorkspaceKindDir, Path: tmpDir}, nil
}

func (s *BuiltinStore) Publish(context.Context, PublishSpec) (Template, error) {
	return Template{}, ErrRegistryNotWritable
}

func (s *BuiltinStore) Delete(context.Context, string) error {
	return ErrRegistryNotDeletable
}

func (s *BuiltinStore) loadManifest(id string) (templateManifest, error) {
	if err := validateLocalTemplateID(id); err != nil {
		return templateManifest{}, err
	}
	manifestPath := s.manifestPath(id)
	data, err := fs.ReadFile(hubtemplates.FS(), manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return templateManifest{}, fmt.Errorf("%w: %s", ErrTemplateNotFound, id)
		}
		return templateManifest{}, fmt.Errorf("read builtin manifest %q: %w", id, err)
	}
	var manifest templateManifest
	if err := toml.Unmarshal(data, &manifest); err != nil {
		return templateManifest{}, fmt.Errorf("decode builtin manifest %q: %w", id, err)
	}
	if err := validateManifest(manifest); err != nil {
		return templateManifest{}, fmt.Errorf("validate builtin manifest %q: %w", id, err)
	}
	return manifest, nil
}

func (s *BuiltinStore) manifestPath(id string) string {
	item, ok := hubtemplates.LookupBuiltin(id)
	if !ok {
		return filepath.ToSlash(filepath.Join("builtin", id, localManifestFileName))
	}
	return hubtemplates.ManifestPath(item.Root)
}

func (s *BuiltinStore) workspacePath(id string) string {
	item, ok := hubtemplates.LookupBuiltin(id)
	if !ok {
		return filepath.ToSlash(filepath.Join("builtin", id, localWorkspaceDirName))
	}
	return hubtemplates.WorkspacePath(item.Root)
}

func (s *BuiltinStore) workspaceRef(id string) WorkspaceRef {
	path := s.workspacePath(id)
	info, err := fs.Stat(hubtemplates.FS(), path)
	if err != nil || !info.IsDir() {
		return WorkspaceRef{}
	}
	return WorkspaceRef{Kind: WorkspaceKindDir, Path: path}
}
