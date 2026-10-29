package template

import (
	"context"
	"csgclaw/internal/apitypes"
)

func (s *Service) SaveAgentTemplate(ctx context.Context, spec PublishSpec) (Template, error) {
	registry := spec.Registry
	if registry == "" {
		registry = s.defaultPublishRegistry
	}
	store, ok := s.stores[registry]
	if !ok {
		return Template{}, ErrRegistryNotFound
	}
	local, ok := store.store.(*LocalStore)
	if !ok {
		return s.Publish(ctx, spec)
	}
	item, err := local.SaveAgent(ctx, spec)
	if err != nil {
		return Template{}, err
	}
	return decorateTemplate(store, item), nil
}

func (s *Service) localVersionStore(id string) (*LocalStore, string, error) {
	store, rawID, err := s.resolveRead(id)
	if err != nil {
		return nil, "", err
	}
	local, ok := store.store.(*LocalStore)
	if !ok || store.ref.Kind != RegistryKindLocal {
		return nil, "", ErrRegistryNotWritable
	}
	return local, rawID, nil
}

func (s *Service) Versions(ctx context.Context, id string) ([]TemplateVersion, error) {
	local, rawID, err := s.localVersionStore(id)
	if err != nil {
		return nil, err
	}
	return local.Versions(ctx, rawID)
}
func (s *Service) ListVersionWorkspace(ctx context.Context, id string, revision int, path string) (apitypes.WorkspaceListing, error) {
	local, rawID, err := s.localVersionStore(id)
	if err != nil {
		return apitypes.WorkspaceListing{}, err
	}
	return local.ListVersionWorkspace(ctx, rawID, revision, path)
}
func (s *Service) ReadVersionFile(ctx context.Context, id string, revision int, path string) (apitypes.WorkspaceFile, error) {
	local, rawID, err := s.localVersionStore(id)
	if err != nil {
		return apitypes.WorkspaceFile{}, err
	}
	return local.ReadVersionFile(ctx, rawID, revision, path)
}
func (s *Service) RestoreVersion(ctx context.Context, id string, revision int, message string, author *VersionAuthor) (Template, error) {
	local, rawID, err := s.localVersionStore(id)
	if err != nil {
		return Template{}, err
	}
	item, err := local.RestoreVersion(ctx, rawID, revision, message, author)
	if err != nil {
		return Template{}, err
	}
	store, _, _ := s.resolveRead(id)
	return decorateTemplate(store, item), nil
}
