package mcp

import (
	"context"
	"fmt"

	"csgclaw/internal/config"
	"csgclaw/internal/localstore"
)

type ServerStore interface {
	ReadServers(ctx context.Context) (map[string]any, error)
	WriteServers(ctx context.Context, servers map[string]any) error
}

type localServerStore struct {
	statePath func() (string, error)
}

func defaultServerStore() ServerStore {
	return localServerStore{statePath: config.DefaultStatePath}
}

func (s localServerStore) ReadServers(context.Context) (map[string]any, error) {
	path, err := s.rootStatePath()
	if err != nil {
		return nil, err
	}
	var servers map[string]any
	ok, err := localstore.ReadSection(path, ServersKey, &servers)
	if err != nil {
		return nil, fmt.Errorf("read mcp servers from root state: %w", err)
	}
	if !ok || servers == nil {
		return map[string]any{}, nil
	}
	return servers, nil
}

func (s localServerStore) WriteServers(_ context.Context, servers map[string]any) error {
	path, err := s.rootStatePath()
	if err != nil {
		return err
	}
	if servers == nil {
		servers = map[string]any{}
	}
	if err := localstore.WriteSection(path, ServersKey, servers); err != nil {
		return fmt.Errorf("write mcp servers to root state: %w", err)
	}
	return nil
}

func (s localServerStore) rootStatePath() (string, error) {
	if s.statePath != nil {
		return s.statePath()
	}
	return config.DefaultStatePath()
}
