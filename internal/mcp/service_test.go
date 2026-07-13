package mcp

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/localstore"
)

func TestServiceUsesInjectedServerStore(t *testing.T) {
	store := &memoryServerStore{
		servers: map[string]any{
			"filesystem": map[string]any{"command": "npx", "args": []any{"-y", "@modelcontextprotocol/server-filesystem", "/workspace"}},
		},
	}
	svc := NewService(WithServerStore(store))

	listed, err := svc.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers() error = %v", err)
	}
	if _, ok := listed["filesystem"]; !ok {
		t.Fatalf("ListServers() missing filesystem server: %#v", listed)
	}

	if _, err := svc.CreateServer(context.Background(), "github", map[string]any{
		"command": "docker",
		"args":    []any{"run", "mcp/github"},
	}); err != nil {
		t.Fatalf("CreateServer() error = %v", err)
	}
	if _, ok := store.servers["github"]; !ok {
		t.Fatalf("store missing created server: %#v", store.servers)
	}
}

func TestCreateServerStoresRawConfigWithTimeout(t *testing.T) {
	store := &memoryServerStore{}
	svc := NewService(WithServerStore(store))

	state, err := svc.CreateServer(context.Background(), "grafana", map[string]any{
		"url":     "https://mcp.example.com/grafana",
		"timeout": 45,
	})
	if err != nil {
		t.Fatalf("CreateServer() error = %v", err)
	}
	servers := state[ServersKey].(map[string]any)
	grafana, ok := servers["grafana"].(map[string]any)
	if !ok {
		t.Fatalf("stored grafana config = %#v, want object", servers["grafana"])
	}
	if timeout, _ := grafana["timeout"].(float64); timeout != 45 {
		t.Fatalf("grafana.timeout = %#v, want 45", grafana["timeout"])
	}
	if _, exists := grafana[ServersKey]; exists {
		t.Fatalf("grafana config unexpectedly contains nested mcpServers: %#v", grafana)
	}
}

func TestCreateServerRejectsWrappedMCPServersConfig(t *testing.T) {
	store := &memoryServerStore{}
	svc := NewService(WithServerStore(store))

	_, err := svc.CreateServer(context.Background(), "grafana", map[string]any{
		ServersKey: map[string]any{
			"grafana": map[string]any{
				"url": "https://mcp.example.com/grafana",
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "single server object") {
		t.Fatalf("CreateServer() error = %v, want wrapped config rejection", err)
	}
	if len(store.servers) != 0 {
		t.Fatalf("CreateServer() wrote wrapped config: %#v", store.servers)
	}
}

func TestServiceCRUDAndErrors(t *testing.T) {
	svc := NewService(WithServerStore(&memoryServerStore{}))
	ctx := context.Background()

	if _, err := svc.CreateServer(ctx, "alpha", map[string]any{"command": "uvx"}); err != nil {
		t.Fatalf("CreateServer(alpha) error = %v", err)
	}
	if _, err := svc.CreateServer(ctx, "alpha", map[string]any{"command": "uvx"}); !errors.Is(err, ErrServerExists) {
		t.Fatalf("CreateServer(alpha duplicate) error = %v, want ErrServerExists", err)
	}
	if _, err := svc.UpdateServer(ctx, "alpha", "beta", map[string]any{"url": "https://mcp.example.com"}); err != nil {
		t.Fatalf("UpdateServer(alpha, beta) error = %v", err)
	}
	listed, err := svc.ListServers(ctx)
	if err != nil {
		t.Fatalf("ListServers() error = %v", err)
	}
	if _, exists := listed["alpha"]; exists {
		t.Fatalf("ListServers() retained renamed server: %#v", listed)
	}
	if _, exists := listed["beta"]; !exists {
		t.Fatalf("ListServers() missing renamed server: %#v", listed)
	}
	if _, err := svc.DeleteServer(ctx, "beta"); err != nil {
		t.Fatalf("DeleteServer(beta) error = %v", err)
	}
	if _, err := svc.DeleteServer(ctx, "beta"); !errors.Is(err, ErrServerNotFound) {
		t.Fatalf("DeleteServer(beta) error = %v, want ErrServerNotFound", err)
	}
}

func TestLocalServerStoreUsesMCPServersRootStateSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := localstore.WriteSection(path, "auth", map[string]any{"provider": "token"}); err != nil {
		t.Fatalf("WriteSection(auth) error = %v", err)
	}
	if err := localstore.WriteSection(path, ServersKey, map[string]any{
		"legacy": map[string]any{"command": "npx"},
	}); err != nil {
		t.Fatalf("WriteSection(mcpServers) error = %v", err)
	}

	store := localServerStore{statePath: func() (string, error) { return path, nil }}
	servers, err := store.ReadServers(context.Background())
	if err != nil {
		t.Fatalf("ReadServers() error = %v", err)
	}
	if _, ok := servers["legacy"]; !ok {
		t.Fatalf("ReadServers() = %#v, want legacy server", servers)
	}
	if err := store.WriteServers(context.Background(), map[string]any{
		"current": map[string]any{"command": "uvx"},
	}); err != nil {
		t.Fatalf("WriteServers() error = %v", err)
	}
	servers, err = store.ReadServers(context.Background())
	if err != nil {
		t.Fatalf("ReadServers() after write error = %v", err)
	}
	if _, ok := servers["current"]; !ok {
		t.Fatalf("ReadServers() after write = %#v, want current server", servers)
	}

	var auth map[string]any
	found, err := localstore.ReadSection(path, "auth", &auth)
	if err != nil {
		t.Fatalf("ReadSection(auth) error = %v", err)
	}
	if !found || auth["provider"] != "token" {
		t.Fatalf("auth section = %#v, want preserved token", auth)
	}
}

type memoryServerStore struct {
	servers map[string]any
}

func (s *memoryServerStore) ReadServers(context.Context) (map[string]any, error) {
	return cloneMap(s.servers), nil
}

func (s *memoryServerStore) WriteServers(_ context.Context, servers map[string]any) error {
	s.servers = cloneMap(servers)
	return nil
}
