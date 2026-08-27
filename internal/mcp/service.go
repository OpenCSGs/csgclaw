package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"csgclaw/internal/knowledgebase"
)

var (
	ErrServerExists   = errors.New("mcp server already exists")
	ErrServerNotFound = errors.New("mcp server not found")
)

var serverDocumentMu sync.Mutex

type Service struct {
	prober ServerProber
	store  ServerStore
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
		if _, exists := servers[name]; exists {
			return fmt.Errorf("%w: %s", ErrServerExists, name)
		}
		if metadata, ok := knowledgebase.ManagedMetadataFromServer(config); ok {
			if existing := knowledgebase.FindConfiguredServer(servers, metadata.KnowledgeBaseID); existing != "" {
				return fmt.Errorf("%w: %s", ErrServerExists, existing)
			}
		}
		servers[name] = config
		return nil
	})
}

// InstallRemoteServer writes a Hub-resolved server into the local catalog.
// Remote discovery and authentication stay outside this service, while the
// server configuration follows the same schema validation and persistence
// path as locally authored catalog entries.
//
// Reinstalling replaces a same-named catalog entry. This is intentional: the
// Hub UI explicitly presents that operation as a replacement.
func (s *Service) InstallRemoteServer(ctx context.Context, server RemoteServer) (string, error) {
	name, config, err := normalizeServerInput(server.Name, server.Config())
	if err != nil {
		return "", err
	}
	_, err = s.updateServers(ctx, func(servers map[string]any) error {
		servers[name] = config
		return nil
	})
	if err != nil {
		return "", err
	}
	return name, nil
}

func (s *Service) UpdateServer(ctx context.Context, currentName, nextName string, config map[string]any) (map[string]any, error) {
	currentName = strings.TrimSpace(currentName)
	nextName, config, err := normalizeServerInput(nextName, config)
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
		if metadata, ok := knowledgebase.ManagedMetadataFromServer(config); ok {
			if existing := knowledgebase.FindConfiguredServer(servers, metadata.KnowledgeBaseID); existing != "" && existing != currentName {
				return fmt.Errorf("%w: %s", ErrServerExists, existing)
			}
		}
		if nextName != currentName {
			if _, exists := servers[nextName]; exists {
				return fmt.Errorf("%w: %s", ErrServerExists, nextName)
			}
			delete(servers, currentName)
		}
		servers[nextName] = config
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
