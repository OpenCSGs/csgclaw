package template

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"csgclaw/internal/config"
	"csgclaw/internal/runtime"
)

const (
	testBuiltinOpenClawImage = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/openclaw:20260717.27-csgclaw"
)

func TestBuiltinStoreListGetAndFetchWorkspace(t *testing.T) {
	store := NewBuiltinStore()

	items, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := len(items), 3; got != want {
		t.Fatalf("len(List()) = %d, want %d", got, want)
	}
	if got, want := items[0].ID, "codex-worker"; got != want {
		t.Fatalf("List()[0].ID = %q, want %q", got, want)
	}
	if got, want := items[1].ID, "manager-codex"; got != want {
		t.Fatalf("List()[1].ID = %q, want %q", got, want)
	}
	if got, want := items[2].ID, "openclaw-worker"; got != want {
		t.Fatalf("List()[2].ID = %q, want %q", got, want)
	}

	openclawItem, err := store.Get(context.Background(), "openclaw-worker")
	if err != nil {
		t.Fatalf("Get(openclaw-worker) error = %v", err)
	}
	if got, want := openclawItem.Image, testBuiltinOpenClawImage; got != want {
		t.Fatalf("Get(openclaw-worker).Image = %q, want %q", got, want)
	}

	codexItem, err := store.Get(context.Background(), "codex-worker")
	if err != nil {
		t.Fatalf("Get(codex-worker) error = %v", err)
	}
	if got, want := codexItem.RuntimeKind, runtime.KindCodex; got != want {
		t.Fatalf("Get(codex-worker).RuntimeKind = %q, want %q", got, want)
	}
	if got := codexItem.Image; got != "" {
		t.Fatalf("Get(codex-worker).Image = %q, want empty", got)
	}

	workspace, err := store.FetchWorkspace(context.Background(), "openclaw-worker")
	if err != nil {
		t.Fatalf("FetchWorkspace() error = %v", err)
	}
	if got, want := workspace.Kind, WorkspaceKindDir; got != want {
		t.Fatalf("FetchWorkspace().Kind = %q, want %q", got, want)
	}
	data, err := os.ReadFile(filepath.Join(workspace.Path, "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("FetchWorkspace() copied empty AGENT.md")
	}
}

func TestMkdirHubWorkspaceTempFallsBackToSlashTmpWhenEnvTempRootIsMissing(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("slash-tmp fallback is only meaningful on Unix-like hosts")
	}

	missingRoot := filepath.Join(t.TempDir(), "missing")
	t.Setenv("TMPDIR", missingRoot)
	t.Setenv("TMP", "")
	t.Setenv("TEMP", "")

	dir, err := mkdirHubWorkspaceTemp("csgclaw-hub-test-*")
	if err != nil {
		t.Fatalf("mkdirHubWorkspaceTemp() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	if got, wantPrefix := filepath.Clean(dir), filepath.Clean(fallbackTempRoot)+string(os.PathSeparator); !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("mkdirHubWorkspaceTemp() = %q, want under %q", dir, fallbackTempRoot)
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
	if got, want := len(items), 4; got != want {
		t.Fatalf("len(List()) = %d, want %d", got, want)
	}
	if got, want := items[0].ID, "builtin.codex-worker"; got != want {
		t.Fatalf("List()[0].ID = %q, want %q", got, want)
	}
	if got, want := items[1].ID, "builtin.manager-codex"; got != want {
		t.Fatalf("List()[1].ID = %q, want %q", got, want)
	}
	if got, want := items[2].ID, "builtin.openclaw-worker"; got != want {
		t.Fatalf("List()[2].ID = %q, want %q", got, want)
	}
	if got, want := items[3].ID, "local.team-helper"; got != want {
		t.Fatalf("List()[3].ID = %q, want %q", got, want)
	}
}
