package hub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"csgclaw/internal/config"
)

var (
	ErrStoreFactoryRequired = errors.New("hub store factory is required")
	ErrRegistryNotFound     = errors.New("hub registry not found")
	ErrRegistryNotReadable  = errors.New("hub registry is not readable")
	ErrRegistryNotWritable  = errors.New("hub registry is not writable")
)

type Store interface {
	List(ctx context.Context) ([]Template, error)
	Get(ctx context.Context, id string) (Template, error)
	FetchWorkspace(ctx context.Context, id string) (WorkspaceRef, error)
	Publish(ctx context.Context, spec PublishSpec) (Template, error)
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

func (s *Service) resolveRead(id string) (configuredStore, string, error) {
	registryName, templateID := splitTemplateRef(id)
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
	prefix := strings.TrimSpace(registryName) + "/"
	if strings.HasPrefix(id, prefix) {
		return strings.TrimPrefix(id, prefix)
	}
	return id
}

func splitTemplateRef(id string) (string, string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", ""
	}
	left, right, ok := strings.Cut(id, "/")
	if !ok {
		return "", id
	}
	return strings.TrimSpace(left), strings.TrimSpace(right)
}

func namespacedTemplateID(registryName, templateID string) string {
	registryName = strings.TrimSpace(registryName)
	templateID = strings.TrimSpace(templateID)
	if registryName == "" {
		return templateID
	}
	if templateID == "" {
		return registryName + "/"
	}
	return registryName + "/" + templateID
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
