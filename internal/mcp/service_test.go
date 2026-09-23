package mcp

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/knowledgebase"
	"csgclaw/internal/localstore"
	"csgclaw/internal/mcpschema"
)

func TestInstallRemoteServerProbesStableIDAndPreservesSourceMetadata(t *testing.T) {
	probed := make(chan string, 2)
	svc := NewService(WithServerStore(&memoryServerStore{}), WithServerProber(availabilityTestProber{
		probe: func(_ context.Context, id string, _ map[string]any) (ProbeResult, error) {
			probed <- id
			return ProbeResult{Connected: true}, nil
		},
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	id, err := svc.InstallRemoteServer(ctx, RemoteServer{ID: "42", HubURL: "https://hub.example", Name: "必应 搜索", URL: "https://mcp.example"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-probed:
		if got != id {
			t.Fatalf("probe key = %q, want persisted ID %q", got, id)
		}
	case <-ctx.Done():
		t.Fatal("installation did not start an availability probe")
	}
	servers, err := svc.ListAvailableServers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := servers[id].(map[string]any)
	if !ok {
		t.Fatal("healthy installation was omitted from the catalog")
	}
	resourceID, sourceName, managed := RemoteHubServerMetadata(entry)
	if !managed || resourceID != "42" || sourceName != "必应 搜索" {
		t.Fatal("availability source metadata lost")
	}
	if source := marketplaceSource(entry); source["server_id"] != "42" || source["hub_url"] != "https://hub.example" {
		t.Fatal("reinstall source identity lost")
	}
	if _, err = svc.UpdateServer(ctx, id, "改名后的搜索", entry); err != nil {
		t.Fatal(err)
	}
	servers, err = svc.ListAvailableServers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mcpschema.ServerDisplayName(id, servers[id]) != "改名后的搜索" {
		t.Fatal("renamed installation lost identity or availability")
	}
	select {
	case got := <-probed:
		t.Fatalf("display-only rename started another probe for %q", got)
	default:
	}
}

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

func TestRenameAndReinstallPreserveIdentity(t *testing.T) {
	store := &memoryServerStore{}
	svc := NewService(WithServerStore(store))
	ctx := context.Background()
	remote := RemoteServer{ID: "42", HubURL: "https://hub.example", Name: "必应 搜索", URL: "https://mcp.example/one"}
	id, err := svc.InstallRemoteServer(ctx, remote)
	if err != nil {
		t.Fatal(err)
	}
	if !mcpschema.ValidServerID(id) {
		t.Fatal("invalid runtime ID")
	}
	config := cloneMap(store.servers[id].(map[string]any))
	if _, err = svc.UpdateServer(ctx, id, "自定义中文 名称", config); err != nil {
		t.Fatal(err)
	}
	remote.Name = "市场也改名了"
	remote.URL = "https://mcp.example/two"
	again, err := svc.InstallRemoteServer(ctx, remote)
	if err != nil {
		t.Fatal(err)
	}
	if again != id || len(store.servers) != 1 {
		t.Fatal("reinstall created another identity")
	}
	if got := mcpschema.ServerDisplayName(id, store.servers[id]); got != "自定义中文 名称" {
		t.Fatalf("rename lost: %s", got)
	}
	if store.servers[id].(map[string]any)["url"] != remote.URL {
		t.Fatal("reinstall did not refresh config")
	}
	remote.ID = "43"
	other, err := svc.InstallRemoteServer(ctx, remote)
	if err != nil {
		t.Fatal(err)
	}
	if other == id || len(store.servers) != 2 {
		t.Fatal("distinct marketplace source replaced an installation")
	}
}

func TestRenameDoesNotAllowReusingOccupiedID(t *testing.T) {
	store := &memoryServerStore{}
	svc := NewService(WithServerStore(store))
	ctx := context.Background()
	config := map[string]any{"url": "https://mcp.example"}
	if _, err := svc.CreateServer(ctx, "alpha", config); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateServer(ctx, "alpha", "beta", config); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateServer(ctx, "alpha", config); err != nil {
		t.Fatal(err)
	}
	if len(store.servers) != 2 || mcpschema.ServerDisplayName("alpha", store.servers["alpha"]) != "beta" {
		t.Fatal("new display name overwrote occupied ID")
	}
	if _, err := svc.UpdateServer(ctx, "alpha", "alpha", config); !errors.Is(err, ErrServerExists) {
		t.Fatalf("duplicate display name accepted: %v", err)
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

func TestInstallRemoteServerCreatesAndReplacesServer(t *testing.T) {
	store := &memoryServerStore{}
	svc := NewService(WithServerStore(store))
	ctx := context.Background()

	if _, err := svc.InstallRemoteServer(ctx, RemoteServer{Name: "calendar", URL: "https://mcp.example.test/v1"}); err != nil {
		t.Fatalf("InstallRemoteServer(create) error = %v", err)
	}
	name, err := svc.InstallRemoteServer(ctx, RemoteServer{Name: " calendar ", URL: "https://mcp.example.test/v2"})
	if err != nil {
		t.Fatalf("InstallRemoteServer(replace) error = %v", err)
	}
	if got, want := name, "calendar"; got != want {
		t.Fatalf("InstallRemoteServer() name = %q, want %q", got, want)
	}
	servers, err := svc.ListServers(ctx)
	if err != nil {
		t.Fatalf("ListServers() error = %v", err)
	}
	calendar := servers["calendar"].(map[string]any)
	if got, want := calendar["url"], "https://mcp.example.test/v2"; got != want {
		t.Fatalf("calendar.url = %#v, want %q", got, want)
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
	if entry, exists := listed["alpha"].(map[string]any); !exists || entry["display_name"] != "beta" {
		t.Fatalf("rename lost fixed ID: %#v", listed)
	}
	if _, exists := listed["beta"]; exists {
		t.Fatal("rename created a new identity")
	}
	if _, err := svc.DeleteServer(ctx, "alpha"); err != nil {
		t.Fatalf("DeleteServer(beta) error = %v", err)
	}
	if _, err := svc.DeleteServer(ctx, "alpha"); !errors.Is(err, ErrServerNotFound) {
		t.Fatalf("DeleteServer(beta) error = %v, want ErrServerNotFound", err)
	}
}

func TestCreateServerUsesManagedKnowledgeBaseMetadataIdentity(t *testing.T) {
	store := &memoryServerStore{}
	svc := NewService(WithServerStore(store))
	config := map[string]any{
		"type": "remote",
		"url":  "https://gateway.example.test/v1/llmwikis/content-42/mcp",
		knowledgebase.ManagedMetaKey: map[string]any{
			knowledgebase.ManagedMetaNamespace: map[string]any{
				"type":        knowledgebase.ManagedMCPType,
				"resource_id": "42",
				"content_id":  "content-42",
				"auth_type":   knowledgebase.ManagedAuthType,
			},
		},
	}
	if _, err := svc.CreateServer(context.Background(), "knowledge-one", config); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateServer(context.Background(), "content-42", config); !errors.Is(err, ErrServerExists) {
		t.Fatalf("duplicate source: %v", err)
	}
	if _, err := svc.CreateServer(context.Background(), "content-42", config); !errors.Is(err, ErrServerExists) {
		t.Fatalf("CreateServer(duplicate) error = %v, want ErrServerExists", err)
	}
}

func TestManagedKnowledgeBaseAuthenticationIsPersistedAndListed(t *testing.T) {
	managedConfig := map[string]any{
		"type":    "remote",
		"url":     "https://gateway.example.test/v1/llmwikis/content-42/mcp",
		"headers": map[string]any{"Authorization": "Bearer current-csghub-token"},
		knowledgebase.ManagedMetaKey: map[string]any{
			knowledgebase.ManagedMetaNamespace: map[string]any{
				"type":        knowledgebase.ManagedMCPType,
				"resource_id": "42",
				"content_id":  "content-42",
				"auth_type":   knowledgebase.ManagedAuthType,
			},
		},
	}
	store := &memoryServerStore{}
	svc := NewService(WithServerStore(store))
	if _, err := svc.CreateServer(context.Background(), "content-42", managedConfig); err != nil {
		t.Fatalf("CreateServer() error = %v", err)
	}

	listed, err := svc.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers() error = %v", err)
	}
	listedConfig := listed["content-42"].(map[string]any)
	listedHeaders := listedConfig["headers"].(map[string]any)
	if got, want := listedHeaders["Authorization"], "Bearer current-csghub-token"; got != want {
		t.Fatalf("listed Authorization = %#v, want %q", got, want)
	}
	persisted := store.servers["content-42"].(map[string]any)
	persistedHeaders := persisted["headers"].(map[string]any)
	if got, want := persistedHeaders["Authorization"], "Bearer current-csghub-token"; got != want {
		t.Fatalf("persisted Authorization = %#v, want %q", got, want)
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

func TestUpdateServerWithoutNamePreservesDisplayLabel(t *testing.T) {
	store := &memoryServerStore{servers: map[string]any{"fixed": map[string]any{"display_name": "自定义 名称", "url": "https://old.example"}}}
	svc := NewService(WithServerStore(store))
	ctx := context.Background()
	if _, err := svc.UpdateServer(ctx, "fixed", "", map[string]any{"url": "https://new.example"}); err != nil {
		t.Fatal(err)
	}
	if got := mcpschema.ServerDisplayName("fixed", store.servers["fixed"]); got != "自定义 名称" {
		t.Fatalf("lost label: %q", got)
	}
	if _, err := svc.UpdateServer(ctx, "fixed", "", map[string]any{"url": "https://new.example", "display_name": "又改名了"}); err != nil {
		t.Fatal(err)
	}
	if got := mcpschema.ServerDisplayName("fixed", store.servers["fixed"]); got != "又改名了" {
		t.Fatalf("config label ignored: %q", got)
	}
}
