package hub

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"csgclaw/internal/runtime"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	localTemplatesDirName  = "templates"
	localManifestFileName  = "agent.toml"
	localWorkspaceDirName  = "workspace"
	localPublishTempPrefix = ".hub-template-"
)

var (
	ErrTemplateNotFound       = errors.New("hub template not found")
	ErrTemplateIDRequired     = errors.New("hub template id is required")
	ErrTemplateNameRequired   = errors.New("hub template name is required")
	ErrRuntimeKindRequired    = errors.New("hub runtime kind is required")
	ErrWorkspaceDirRequired   = errors.New("hub workspace directory is required")
	ErrWorkspacePathUnsafe    = errors.New("hub workspace path is unsafe")
	ErrWorkspaceSymlinkDenied = errors.New("hub workspace symlinks are not supported")
)

type LocalStore struct {
	root string
}

type localTemplateManifest struct {
	Name        string `toml:"name"`
	Description string `toml:"description,omitempty"`
	RuntimeKind string `toml:"runtime_kind"`
	Image       string `toml:"image,omitempty"`
	UpdatedAt   string `toml:"updated_at,omitempty"`
}

func NewLocalStore(root string) *LocalStore {
	return &LocalStore{root: strings.TrimSpace(root)}
}

func (s *LocalStore) List(context.Context) ([]Template, error) {
	entries, err := os.ReadDir(s.templatesRoot())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read local hub templates: %w", err)
	}

	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := strings.TrimSpace(entry.Name())
		if err := validateLocalTemplateID(id); err != nil {
			return nil, fmt.Errorf("invalid local hub template %q: %w", id, err)
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

func (s *LocalStore) Get(_ context.Context, id string) (Template, error) {
	id, manifest, err := s.loadTemplate(id)
	if err != nil {
		return Template{}, err
	}
	updatedAt, err := parseManifestUpdatedAt(manifest.UpdatedAt)
	if err != nil {
		return Template{}, fmt.Errorf("validate local hub manifest %q: %w", id, err)
	}
	return Template{
		ID:           id,
		Name:         manifest.Name,
		Description:  manifest.Description,
		RuntimeKind:  manifest.RuntimeKind,
		Image:        manifest.Image,
		WorkspaceRef: s.workspaceRef(id),
		UpdatedAt:    updatedAt,
	}, nil
}

func (s *LocalStore) FetchWorkspace(_ context.Context, id string) (WorkspaceRef, error) {
	id = strings.TrimSpace(id)
	if err := validateLocalTemplateID(id); err != nil {
		return WorkspaceRef{}, err
	}
	workspace := s.workspaceRoot(id)
	info, err := os.Stat(workspace)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return WorkspaceRef{}, nil
		}
		return WorkspaceRef{}, fmt.Errorf("stat local hub workspace %q: %w", workspace, err)
	}
	if !info.IsDir() {
		return WorkspaceRef{}, fmt.Errorf("local hub workspace %q is not a directory", workspace)
	}
	return WorkspaceRef{Kind: WorkspaceKindDir, Path: workspace}, nil
}

func (s *LocalStore) Publish(_ context.Context, spec PublishSpec) (Template, error) {
	normalized, err := normalizePublishSpec(spec)
	if err != nil {
		return Template{}, err
	}

	if err := os.MkdirAll(s.templatesRoot(), 0o755); err != nil {
		return Template{}, fmt.Errorf("create local hub templates dir: %w", err)
	}

	tmpDir, err := os.MkdirTemp(s.templatesRoot(), localPublishTempPrefix)
	if err != nil {
		return Template{}, fmt.Errorf("create local hub template temp dir: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	if err := s.writeManifest(filepath.Join(tmpDir, localManifestFileName), normalized); err != nil {
		return Template{}, err
	}
	if normalized.WorkspaceRef.Kind == WorkspaceKindDir {
		if err := os.MkdirAll(filepath.Join(tmpDir, localWorkspaceDirName), 0o755); err != nil {
			return Template{}, fmt.Errorf("create local hub temp workspace dir: %w", err)
		}
		if err := copyWorkspaceTree(normalized.WorkspaceRef.Path, filepath.Join(tmpDir, localWorkspaceDirName)); err != nil {
			return Template{}, err
		}
	}

	targetDir := s.templateRoot(normalized.ID)
	if err := os.RemoveAll(targetDir); err != nil {
		return Template{}, fmt.Errorf("replace local hub template %q: %w", normalized.ID, err)
	}
	if err := os.Rename(tmpDir, targetDir); err != nil {
		return Template{}, fmt.Errorf("replace local hub template %q: %w", normalized.ID, err)
	}
	cleanup = false

	return s.Get(context.Background(), normalized.ID)
}

func (s *LocalStore) templatesRoot() string {
	return filepath.Join(s.root, localTemplatesDirName)
}

func (s *LocalStore) templateRoot(id string) string {
	return filepath.Join(s.templatesRoot(), id)
}

func (s *LocalStore) manifestPath(id string) string {
	return filepath.Join(s.templateRoot(id), localManifestFileName)
}

func (s *LocalStore) workspaceRoot(id string) string {
	return filepath.Join(s.templateRoot(id), localWorkspaceDirName)
}

func (s *LocalStore) workspaceRef(id string) WorkspaceRef {
	workspace := s.workspaceRoot(id)
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		return WorkspaceRef{}
	}
	return WorkspaceRef{Kind: WorkspaceKindDir, Path: workspace}
}

func (s *LocalStore) loadTemplate(id string) (string, localTemplateManifest, error) {
	id = strings.TrimSpace(id)
	if err := validateLocalTemplateID(id); err != nil {
		return "", localTemplateManifest{}, err
	}
	return loadManifestFS(os.DirFS(s.root), filepath.ToSlash(filepath.Join(localTemplatesDirName, id, localManifestFileName)), "local hub")
}

func (s *LocalStore) writeManifest(path string, spec PublishSpec) error {
	manifest := localTemplateManifest{
		Name:        spec.Name,
		Description: spec.Description,
		RuntimeKind: spec.RuntimeKind,
		Image:       spec.Image,
		UpdatedAt:   spec.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	data, err := toml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode local hub manifest: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write local hub manifest: %w", err)
	}
	return nil
}

func normalizePublishSpec(spec PublishSpec) (PublishSpec, error) {
	spec.ID = strings.TrimSpace(spec.ID)
	if spec.ID == "" {
		spec.ID = strings.TrimSpace(spec.Name)
	}
	if err := validateLocalTemplateID(spec.ID); err != nil {
		return PublishSpec{}, err
	}

	spec.Name = strings.TrimSpace(spec.Name)
	if spec.Name == "" {
		return PublishSpec{}, ErrTemplateNameRequired
	}
	spec.Description = strings.TrimSpace(spec.Description)
	if spec.RuntimeKind == "" {
		return PublishSpec{}, ErrRuntimeKindRequired
	}
	spec.WorkspaceRef.Kind = strings.TrimSpace(spec.WorkspaceRef.Kind)
	spec.WorkspaceRef.Path = strings.TrimSpace(spec.WorkspaceRef.Path)
	if spec.WorkspaceRef.Kind == "" && spec.WorkspaceRef.Path == "" {
		if spec.UpdatedAt.IsZero() {
			spec.UpdatedAt = time.Now().UTC()
		} else {
			spec.UpdatedAt = spec.UpdatedAt.UTC()
		}
		return spec, nil
	}
	if spec.WorkspaceRef.Kind == "" {
		spec.WorkspaceRef.Kind = WorkspaceKindDir
	}
	if spec.WorkspaceRef.Kind != WorkspaceKindDir {
		return PublishSpec{}, ErrWorkspaceDirRequired
	}
	if spec.WorkspaceRef.Path == "" {
		return PublishSpec{}, ErrWorkspaceDirRequired
	}
	info, err := os.Stat(spec.WorkspaceRef.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PublishSpec{}, fmt.Errorf("%w: %s", ErrWorkspaceDirRequired, spec.WorkspaceRef.Path)
		}
		return PublishSpec{}, fmt.Errorf("stat hub workspace: %w", err)
	}
	if !info.IsDir() {
		return PublishSpec{}, ErrWorkspaceDirRequired
	}
	if spec.UpdatedAt.IsZero() {
		spec.UpdatedAt = time.Now().UTC()
	} else {
		spec.UpdatedAt = spec.UpdatedAt.UTC()
	}
	return spec, nil
}

func validateManifest(manifest localTemplateManifest) error {
	manifest.Name = strings.TrimSpace(manifest.Name)
	if manifest.Name == "" {
		return ErrTemplateNameRequired
	}
	switch manifest.RuntimeKind {
	case runtime.KindPicoClawSandbox, runtime.KindOpenClawSandbox, runtime.KindCodex:
	default:
		return fmt.Errorf("%w: %s", ErrRuntimeKindRequired, manifest.RuntimeKind)
	}
	if _, err := parseManifestUpdatedAt(manifest.UpdatedAt); err != nil {
		return err
	}
	return nil
}

func parseManifestUpdatedAt(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid updated_at %q", value)
	}
	return parsed.UTC(), nil
}

func validateLocalTemplateID(id string) error {
	id = strings.TrimSpace(id)
	switch {
	case id == "":
		return ErrTemplateIDRequired
	case id == "." || id == "..":
		return ErrWorkspacePathUnsafe
	}
	if strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return ErrWorkspacePathUnsafe
	}
	if filepath.Base(id) != id || filepath.Clean(id) != id {
		return ErrWorkspacePathUnsafe
	}
	return nil
}
