package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"csgclaw/internal/knowledgebase"
	"csgclaw/internal/mcpschema"
)

var (
	ErrServerExists   = errors.New("mcp server already exists")
	ErrServerNotFound = errors.New("mcp server not found")
)

var serverDocumentMu sync.Mutex

type Service struct {
	prober               ServerProber
	store                ServerStore
	availabilityOnce     sync.Once
	availabilityMu       sync.Mutex
	availability         map[string]availabilityEntry
	availabilityInflight map[string]*availabilityProbe
	availabilitySem      chan struct{}
}

type ServiceOption func(*Service)

func WithServerStore(store ServerStore) ServiceOption {
	return func(s *Service) {
		if store != nil {
			s.store = store
		}
	}
}

func WithServerProber(prober ServerProber) ServiceOption {
	return func(s *Service) {
		if prober != nil {
			s.prober = prober
		}
	}
}

func NewService(options ...ServiceOption) *Service {
	svc := &Service{prober: defaultServerProber{}, store: defaultServerStore()}
	for _, option := range options {
		if option != nil {
			option(svc)
		}
	}
	return svc
}

func (s *Service) ListServers(ctx context.Context) (map[string]any, error) {
	serverDocumentMu.Lock()
	defer serverDocumentMu.Unlock()

	servers, err := s.serverStore().ReadServers(ctx)
	if err != nil {
		return nil, err
	}
	return cloneMap(servers), nil
}

func (s *Service) CreateServer(ctx context.Context, name string, config map[string]any) (map[string]any, error) {
	name, config, err := normalizeServerInput(name, config)
	if err != nil {
		return nil, err
	}
	return s.updateServers(ctx, func(servers map[string]any) error {
		if findDisplayName(servers, name) != "" {
			return fmt.Errorf("%w: %s", ErrServerExists, name)
		}
		if metadata, ok := knowledgebase.ManagedMetadataFromServer(config); ok {
			if existing := knowledgebase.FindConfiguredServer(servers, metadata.ContentID); existing != "" {
				return fmt.Errorf("%w: %s", ErrServerExists, existing)
			}
		}
		id := mcpschema.NewServerID(name, func(id string) bool { _, exists := servers[id]; return exists })
		config[mcpschema.DisplayNameKey] = name
		servers[id] = config
		return nil
	})
}

// InstallRemoteServer retains the local identity and display name on reinstall.
// Marketplace identity, when available, survives upstream and local renames.
func (s *Service) InstallRemoteServer(ctx context.Context, server RemoteServer) (string, error) {
	name, config, err := normalizeServerInput(server.Name, server.Config())
	if err != nil {
		return "", err
	}
	var id string
	_, err = s.updateServers(ctx, func(servers map[string]any) error {
		if server.ID != "" {
			for key, raw := range servers {
				if entry, ok := raw.(map[string]any); ok && sameMarketplaceSource(entry, config) {
					id = key
					break
				}
			}
		}
		if id == "" {
			candidate := findDisplayName(servers, name)
			if entry, ok := servers[candidate].(map[string]any); ok {
				// Old installations have no source ID; only those may be adopted by name.
				if marketplaceSource(entry) == nil {
					id = candidate
				}
			}
		}
		label := name
		if id != "" {
			label = mcpschema.ServerDisplayName(id, servers[id])
		} else {
			id = mcpschema.NewServerID(name, func(id string) bool { _, exists := servers[id]; return exists })
		}
		config[mcpschema.DisplayNameKey] = label
		servers[id] = config
		return nil
	})
	if err != nil {
		return "", err
	}
	s.initAvailability()
	hash, hashErr := availabilityConfigHash(config)
	if hashErr == nil {
		s.startAvailabilityProbe(id, config, hash)
	}
	return id, nil
}

func (s *Service) UpdateServer(ctx context.Context, currentName, nextName string, config map[string]any) (map[string]any, error) {
	currentName = strings.TrimSpace(currentName)
	nextName = strings.TrimSpace(nextName)
	_, config, err := normalizeServerInput(currentName, config)
	if err != nil {
		return nil, err
	}
	if currentName == "" {
		return nil, fmt.Errorf("mcp server name is required")
	}
	return s.updateServers(ctx, func(servers map[string]any) error {
		if _, exists := servers[currentName]; !exists {
			return fmt.Errorf("%w: %s", ErrServerNotFound, currentName)
		}
		if nextName == "" {
			nextName = mcpschema.ServerDisplayName(mcpschema.ServerDisplayName(currentName, servers[currentName]), config)
		}
		if metadata, ok := knowledgebase.ManagedMetadataFromServer(config); ok {
			if existing := knowledgebase.FindConfiguredServer(servers, metadata.ContentID); existing != "" && existing != currentName {
				return fmt.Errorf("%w: %s", ErrServerExists, existing)
			}
		}
		if existing := findDisplayName(servers, nextName); existing != "" && existing != currentName {
			return fmt.Errorf("%w: %s", ErrServerExists, nextName)
		}
		config[mcpschema.DisplayNameKey] = nextName
		servers[currentName] = config
		return nil
	})
}

func (s *Service) DeleteServer(ctx context.Context, name string) (map[string]any, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("mcp server name is required")
	}
	return s.updateServers(ctx, func(servers map[string]any) error {
		if _, exists := servers[name]; !exists {
			return fmt.Errorf("%w: %s", ErrServerNotFound, name)
		}
		delete(servers, name)
		return nil
	})
}

func (s *Service) updateServers(ctx context.Context, update func(map[string]any) error) (map[string]any, error) {
	serverDocumentMu.Lock()
	defer serverDocumentMu.Unlock()

	store := s.serverStore()
	servers, err := store.ReadServers(ctx)
	if err != nil {
		return nil, err
	}
	if servers == nil {
		servers = map[string]any{}
	}
	if err := update(servers); err != nil {
		return nil, err
	}
	if err := store.WriteServers(ctx, servers); err != nil {
		return nil, err
	}
	return map[string]any{ServersKey: cloneMap(servers)}, nil
}

func (s *Service) serverStore() ServerStore {
	if s == nil || s.store == nil {
		return defaultServerStore()
	}
	return s.store
}

func findDisplayName(servers map[string]any, name string) string {
	for id, raw := range servers {
		if mcpschema.ServerDisplayName(id, raw) == name {
			return id
		}
	}
	return ""
}

func marketplaceSource(config map[string]any) map[string]any {
	meta, _ := config["_meta"].(map[string]any)
	source, _ := meta[mcpschema.MarketplaceMetaKey].(map[string]any)
	return source
}

func sameMarketplaceSource(a, b map[string]any) bool {
	left, right := marketplaceSource(a), marketplaceSource(b)
	return left != nil && right != nil && left["server_id"] == right["server_id"] && left["hub_url"] == right["hub_url"]
}
