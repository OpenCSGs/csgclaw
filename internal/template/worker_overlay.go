package template

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"csgclaw/internal/migrations/retiredclaws"
	toml "github.com/pelletier/go-toml/v2"
)

// WorkerOverlay contains only fields explicitly present in the downloaded
// manifest. Catalog display metadata and creation defaults do not participate.
type WorkerOverlay struct {
	Workspace      WorkspaceRef   `toml:"-"`
	RuntimeKind    string         `toml:"runtime_kind"`
	Description    *string        `toml:"description"`
	Avatar         *string        `toml:"avatar"`
	RuntimeOptions map[string]any `toml:"runtime_options"`
	Model          struct {
		ID              *string `toml:"model_id"`
		ReasoningEffort *string `toml:"reasoning_effort"`
	} `toml:"model_config"`
	Image struct {
		Ref *string `toml:"ref"`
		Env []struct {
			Name    string  `toml:"name"`
			Default *string `toml:"default"`
		} `toml:"env"`
	} `toml:"image"`
}

func (s *Service) FetchWorkerOverlay(ctx context.Context, ref string) (WorkerOverlay, error) {
	store, id, err := s.resolveRead(ref)
	if err != nil {
		return WorkerOverlay{}, err
	}
	var data []byte
	var workspace WorkspaceRef
	if remote, ok := store.store.(*RemoteStore); ok {
		// Read the manifest and the files from one snapshot of the default branch.
		root, err := remote.downloadTemplateDir(ctx, id)
		if err != nil {
			return WorkerOverlay{}, err
		}
		defer os.RemoveAll(root)
		data, err = os.ReadFile(filepath.Join(root, remoteManifestFileName))
		if err != nil {
			return WorkerOverlay{}, err
		}
		_, manifest, err := loadManifestFS(os.DirFS(filepath.Dir(root)), templateManifestFSPath(root), "worker template")
		if err != nil {
			return WorkerOverlay{}, err
		}
		if !templateLayoutExists(root) && legacyTemplateWorkspaceExists(root) {
			workspace, err = materializeLegacyTemplateWorkspace(root)
		} else {
			workspace, err = materializeWorkspaceFiles(os.DirFS(root), ".", manifest.RuntimeKind, false)
		}
		if err != nil {
			return WorkerOverlay{}, err
		}
	} else {
		if store.ref.Kind == RegistryKindLocal {
			localTemplateReplaceMu.Lock()
			defer localTemplateReplaceMu.Unlock()
		}
		file, err := s.ReadWorkspaceFile(ctx, ref, localManifestFileName)
		if err != nil {
			return WorkerOverlay{}, err
		}
		data = []byte(file.Content)
		workspace, err = store.store.FetchWorkspace(ctx, id)
		if err != nil {
			return WorkerOverlay{}, err
		}
	}
	var overlay WorkerOverlay
	if err := toml.Unmarshal(data, &overlay); err != nil {
		if workspace.Temporary {
			os.RemoveAll(workspace.Path)
		}
		return WorkerOverlay{}, fmt.Errorf("decode worker template: %w", err)
	}
	var image string
	if overlay.Image.Ref != nil {
		image = *overlay.Image.Ref
	}
	retiredclaws.TemplateManifest(&overlay.RuntimeKind, &overlay.RuntimeOptions, &image)
	if overlay.Image.Ref != nil {
		overlay.Image.Ref = &image
	}
	overlay.RuntimeKind = normalizeTemplateRuntimeKind(overlay.RuntimeKind)
	overlay.Workspace = workspace
	return overlay, nil
}
