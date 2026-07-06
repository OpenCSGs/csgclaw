package template

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"csgclaw/internal/agentworkspace"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
)

var (
	ErrStoreFactoryRequired = errors.New("hub store factory is required")
	ErrRegistryNotFound     = errors.New("hub registry not found")
	ErrRegistryNotReadable  = errors.New("hub registry is not readable")
	ErrRegistryNotWritable  = errors.New("hub registry is not writable")
	ErrRegistryNotDeletable = errors.New("hub registry is not deletable")
)

const templateIDNamespaceSeparator = "."

type Store interface {
	List(ctx context.Context) ([]Template, error)
	Get(ctx context.Context, id string) (Template, error)
	FetchWorkspace(ctx context.Context, id string) (WorkspaceRef, error)
	Publish(ctx context.Context, spec PublishSpec) (Template, error)
	Delete(ctx context.Context, id string) error
}

type WorkspaceBrowser interface {
	ListWorkspace(ctx context.Context, id, workspacePath string) (apitypes.WorkspaceListing, error)
	ReadWorkspaceFile(ctx context.Context, id, workspacePath string) (apitypes.WorkspaceFile, error)
}

type StoreFactory func(cfg config.HubRegistryConfig) (Store, error)

type Service struct {
	defaultRegistry        string
	defaultPublishRegistry string
	stores                 map[string]configuredStore
	order                  []string
}

type configuredStore struct {
	ref   RegistryRef
	store Store
}

func NewService(cfg config.HubConfig, factory StoreFactory) (*Service, error) {
	if factory == nil {
		return nil, ErrStoreFactoryRequired
	}

	resolved := cfg.Resolved()
	svc := &Service{
		defaultRegistry:        resolved.DefaultRegistry,
		defaultPublishRegistry: resolved.DefaultPublishRegistry,
		stores:                 make(map[string]configuredStore, len(resolved.Registries)),
		order:                  make([]string, 0, len(resolved.Registries)),
	}

	for _, registry := range resolved.Registries {
		if !registry.Enabled {
			continue
		}
		store, err := factory(registry)
		if err != nil {
			return nil, fmt.Errorf("create hub store %q: %w", registry.Name, err)
		}
		svc.stores[registry.Name] = configuredStore{
			ref: RegistryRef{
				Name: registry.Name,
				Kind: normalizeRegistryKind(registry.Kind),
			},
			store: store,
		}
		svc.order = append(svc.order, registry.Name)
	}

	return svc, nil
}

func (s *Service) List(ctx context.Context) ([]Template, error) {
	var out []Template
	var listErrs []error
	for _, name := range s.order {
		cfgStore, ok := s.stores[name]
		if !ok || !isReadableKind(cfgStore.ref.Kind) {
			continue
		}
		items, err := cfgStore.store.List(ctx)
		if err != nil {
			listErr := fmt.Errorf("list hub registry %q: %w", name, err)
			listErrs = append(listErrs, listErr)
			slog.Warn("hub registry list failed", "registry", name, "kind", cfgStore.ref.Kind, "error", err)
			continue
		}
		for _, item := range items {
			out = append(out, decorateTemplate(cfgStore.ref, item))
		}
	}
	if len(listErrs) > 0 {
		joined := errors.Join(listErrs...)
		if len(out) == 0 {
			return nil, joined
		}
		slog.Warn("hub list returned partial results", "template_count", len(out), "error", joined)
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id string) (Template, error) {
	cfgStore, templateID, err := s.resolveRead(id)
	if err != nil {
		return Template{}, err
	}
	item, err := cfgStore.store.Get(ctx, templateID)
	if err != nil {
		return Template{}, fmt.Errorf("get hub template %q from %q: %w", templateID, cfgStore.ref.Name, err)
	}
	return decorateTemplate(cfgStore.ref, item), nil
}

func (s *Service) FetchWorkspace(ctx context.Context, id string) (WorkspaceRef, error) {
	cfgStore, templateID, err := s.resolveRead(id)
	if err != nil {
		return WorkspaceRef{}, err
	}
	workspace, err := cfgStore.store.FetchWorkspace(ctx, templateID)
	if err != nil {
		return WorkspaceRef{}, fmt.Errorf("fetch hub workspace %q from %q: %w", templateID, cfgStore.ref.Name, err)
	}
	return workspace, nil
}

func (s *Service) ListWorkspace(ctx context.Context, id, workspacePath string) (apitypes.WorkspaceListing, error) {
	cfgStore, templateID, err := s.resolveRead(id)
	if err != nil {
		return apitypes.WorkspaceListing{}, err
	}
	if browser, ok := cfgStore.store.(WorkspaceBrowser); ok {
		return browser.ListWorkspace(ctx, templateID, workspacePath)
	}
	workspace, err := cfgStore.store.FetchWorkspace(ctx, templateID)
	if err != nil {
		return apitypes.WorkspaceListing{}, err
	}
	if normalizeRegistryKind(cfgStore.ref.Kind) != RegistryKindLocal {
		defer func() { _ = os.RemoveAll(workspace.Path) }()
	}
	if strings.TrimSpace(workspace.Path) == "" {
		if strings.TrimSpace(workspacePath) == "" {
			return apitypes.WorkspaceListing{Kind: WorkspaceKindDir}, nil
		}
		return apitypes.WorkspaceListing{}, ErrWorkspaceDirRequired
	}
	return agentworkspace.ListDirectory(workspace.Path, workspacePath)
}

func (s *Service) ReadWorkspaceFile(ctx context.Context, id, workspacePath string) (apitypes.WorkspaceFile, error) {
	cfgStore, templateID, err := s.resolveRead(id)
	if err != nil {
		return apitypes.WorkspaceFile{}, err
	}
	if browser, ok := cfgStore.store.(WorkspaceBrowser); ok {
		return browser.ReadWorkspaceFile(ctx, templateID, workspacePath)
	}
	workspace, err := cfgStore.store.FetchWorkspace(ctx, templateID)
	if err != nil {
		return apitypes.WorkspaceFile{}, err
	}
	if normalizeRegistryKind(cfgStore.ref.Kind) != RegistryKindLocal {
		defer func() { _ = os.RemoveAll(workspace.Path) }()
	}
	if strings.TrimSpace(workspace.Path) == "" {
		return apitypes.WorkspaceFile{}, ErrWorkspaceDirRequired
	}
	return agentworkspace.ReadFile(workspace.Path, workspacePath)
}

func (s *Service) Publish(ctx context.Context, spec PublishSpec) (Template, error) {
	target := strings.TrimSpace(spec.Registry)
	if target == "" {
		target = s.defaultPublishRegistry
	}
	cfgStore, ok := s.stores[target]
	if !ok {
		return Template{}, fmt.Errorf("%w: %s", ErrRegistryNotFound, target)
	}
	if !isWritableKind(cfgStore.ref.Kind) {
		return Template{}, fmt.Errorf("%w: %s", ErrRegistryNotWritable, target)
	}

	spec.Registry = cfgStore.ref.Name
	item, err := cfgStore.store.Publish(ctx, spec)
	if err != nil {
		return Template{}, fmt.Errorf("publish hub template to %q: %w", cfgStore.ref.Name, err)
	}
	return decorateTemplate(cfgStore.ref, item), nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	cfgStore, templateID, err := s.resolveRead(id)
	if err != nil {
		return err
	}
	if normalizeRegistryKind(cfgStore.ref.Kind) != RegistryKindLocal {
		return fmt.Errorf("%w: %s", ErrRegistryNotDeletable, cfgStore.ref.Name)
	}
	if err := cfgStore.store.Delete(ctx, templateID); err != nil {
		return fmt.Errorf("delete hub template %q from %q: %w", templateID, cfgStore.ref.Name, err)
	}
	return nil
}

func (s *Service) resolveRead(id string) (configuredStore, string, error) {
	registryName, templateID := s.splitTemplateRef(id)
	if registryName == "" {
		registryName = s.defaultRegistry
	}
	cfgStore, ok := s.stores[registryName]
	if !ok {
		return configuredStore{}, "", fmt.Errorf("%w: %s", ErrRegistryNotFound, registryName)
	}
	if !isReadableKind(cfgStore.ref.Kind) {
		return configuredStore{}, "", fmt.Errorf("%w: %s", ErrRegistryNotReadable, registryName)
	}
	return cfgStore, templateID, nil
}

func decorateTemplate(source RegistryRef, item Template) Template {
	item.Source = source
	item.ID = namespacedTemplateID(source.Name, localTemplateID(source.Name, item))
	return item
}

func localTemplateID(registryName string, item Template) string {
	id := strings.TrimSpace(item.ID)
	if id == "" {
		id = strings.TrimSpace(item.Name)
	}
	registryName = strings.TrimSpace(registryName)
	for _, prefix := range []string{registryName + templateIDNamespaceSeparator, registryName + "/"} {
		if registryName != "" && strings.HasPrefix(id, prefix) {
			return strings.TrimPrefix(id, prefix)
		}
	}
	return id
}

func (s *Service) splitTemplateRef(id string) (string, string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", ""
	}
	var (
		matchRegistry string
		matchTemplate string
	)
	for registryName := range s.stores {
		registryName = strings.TrimSpace(registryName)
		if registryName == "" {
			continue
		}
		prefix := registryName + templateIDNamespaceSeparator
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		templateID := strings.TrimSpace(strings.TrimPrefix(id, prefix))
		if templateID == "" {
			continue
		}
		if len(registryName) > len(matchRegistry) {
			matchRegistry = registryName
			matchTemplate = templateID
		}
	}
	if matchRegistry == "" {
		return "", id
	}
	return matchRegistry, matchTemplate
}

func namespacedTemplateID(registryName, templateID string) string {
	registryName = strings.TrimSpace(registryName)
	templateID = strings.TrimSpace(templateID)
	if registryName == "" {
		return templateID
	}
	if templateID == "" {
		return registryName + templateIDNamespaceSeparator
	}
	return registryName + templateIDNamespaceSeparator + templateID
}

func normalizeRegistryKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case RegistryKindBuiltin:
		return RegistryKindBuiltin
	case RegistryKindLocal:
		return RegistryKindLocal
	case RegistryKindRemote:
		return RegistryKindRemote
	default:
		return strings.ToLower(strings.TrimSpace(kind))
	}
}

func isReadableKind(kind string) bool {
	switch normalizeRegistryKind(kind) {
	case RegistryKindBuiltin, RegistryKindLocal, RegistryKindRemote:
		return true
	default:
		return false
	}
}

func isWritableKind(kind string) bool {
	switch normalizeRegistryKind(kind) {
	case RegistryKindLocal, RegistryKindRemote:
		return true
	default:
		return false
	}
}
