package hub

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/config"
	"csgclaw/internal/runtime"
)

const (
	testBuiltinOpenClawImage = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/openclaw:20260608.2-csgclaw"
)

func TestBuiltinStoreListGetAndFetchWorkspace(t *testing.T) {
	store := NewBuiltinStore()

	items, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := len(items), 4; got != want {
		t.Fatalf("len(List()) = %d, want %d", got, want)
	}
	if got, want := items[0].ID, "openclaw-manager"; got != want {
		t.Fatalf("List()[0].ID = %q, want %q", got, want)
	}
	if got, want := items[1].ID, "openclaw-worker"; got != want {
		t.Fatalf("List()[1].ID = %q, want %q", got, want)
	}
	if got, want := items[2].ID, "picoclaw-manager"; got != want {
		t.Fatalf("List()[2].ID = %q, want %q", got, want)
	}
	if got, want := items[3].ID, "picoclaw-worker"; got != want {
		t.Fatalf("List()[3].ID = %q, want %q", got, want)
	}

	item, err := store.Get(context.Background(), "picoclaw-worker")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got, want := item.RuntimeKind, runtime.KindPicoClawSandbox; got != want {
		t.Fatalf("Get().RuntimeKind = %q, want %q", got, want)
	}
	if got, want := item.Role, TemplateRoleWorker; got != want {
		t.Fatalf("Get().Role = %q, want %q", got, want)
	}
	if item.Image == "" {
		t.Fatal("Get().Image is empty")
	}
	if got, want := item.WorkspaceRef.Kind, WorkspaceKindDir; got != want {
		t.Fatalf("Get().WorkspaceRef.Kind = %q, want %q", got, want)
	}

	openclawItem, err := store.Get(context.Background(), "openclaw-worker")
	if err != nil {
		t.Fatalf("Get(openclaw-worker) error = %v", err)
	}
	if got, want := openclawItem.Image, testBuiltinOpenClawImage; got != want {
		t.Fatalf("Get(openclaw-worker).Image = %q, want %q", got, want)
	}

	workspace, err := store.FetchWorkspace(context.Background(), "picoclaw-worker")
	if err != nil {
		t.Fatalf("FetchWorkspace() error = %v", err)
	}
	if got, want := workspace.Kind, WorkspaceKindDir; got != want {
		t.Fatalf("FetchWorkspace().Kind = %q, want %q", got, want)
	}
	data, err := os.ReadFile(filepath.Join(workspace.Path, "AGENT.md"))
	if err != nil {
		t.Fatalf("ReadFile(AGENT.md) error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("FetchWorkspace() copied empty AGENT.md")
	}
}

func TestBuiltinStoreGetMissingTemplate(t *testing.T) {
	store := NewBuiltinStore()

	_, err := store.Get(context.Background(), "missing")
	if !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("Get() error = %v, want ErrTemplateNotFound", err)
	}
}

func TestServiceListAggregatesBuiltinAndLocalWithDefaultStoreFactory(t *testing.T) {
	registryRoot := t.TempDir()
	workspaceRoot := writeWorkspaceFile(t, "workspace", "AGENTS.md", "local agent")
	localStore := NewLocalStore(registryRoot)
	if _, err := localStore.Publish(context.Background(), PublishSpec{
		Name:         "team-helper",
		Role:         TemplateRoleWorker,
		RuntimeKind:  runtime.KindCodex,
		WorkspaceRef: WorkspaceRef{Kind: WorkspaceKindDir, Path: workspaceRoot},
	}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	svc, err := NewService(config.HubConfig{
		DefaultRegistry:        "builtin",
		DefaultPublishRegistry: "local",
		Registries: []config.HubRegistryConfig{
			{Name: "builtin", Kind: RegistryKindBuiltin, Enabled: true},
			{Name: "local", Kind: RegistryKindLocal, Path: registryRoot, Enabled: true},
			{Name: config.DefaultOfficialHubRegistryName, Kind: RegistryKindRemote, Enabled: false},
		},
	}, DefaultStoreFactory)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	items, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := len(items), 5; got != want {
		t.Fatalf("len(List()) = %d, want %d", got, want)
	}
	if got, want := items[0].ID, "builtin.openclaw-manager"; got != want {
		t.Fatalf("List()[0].ID = %q, want %q", got, want)
	}
	if got, want := items[1].ID, "builtin.openclaw-worker"; got != want {
		t.Fatalf("List()[1].ID = %q, want %q", got, want)
	}
	if got, want := items[2].ID, "builtin.picoclaw-manager"; got != want {
		t.Fatalf("List()[2].ID = %q, want %q", got, want)
	}
	if got, want := items[3].ID, "builtin.picoclaw-worker"; got != want {
		t.Fatalf("List()[3].ID = %q, want %q", got, want)
	}
	if got, want := items[4].ID, "local.team-helper"; got != want {
		t.Fatalf("List()[4].ID = %q, want %q", got, want)
	}
}
