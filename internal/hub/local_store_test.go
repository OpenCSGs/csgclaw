package hub

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"csgclaw/internal/runtime"
)

func TestLocalStorePublishRoundTrip(t *testing.T) {
	registryRoot := t.TempDir()
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "skills"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "AGENTS.md"), []byte("agent"), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENTS.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "skills", "frontend.txt"), []byte("skill"), 0o755); err != nil {
		t.Fatalf("WriteFile(skill) error = %v", err)
	}

	store := NewLocalStore(registryRoot)
	publishedAt := time.Date(2026, 5, 12, 8, 30, 0, 0, time.UTC)
	published, err := store.Publish(context.Background(), PublishSpec{
		Name:         "frontend-alice",
		Description:  "Frontend worker with UI and styling skills",
		Role:         TemplateRoleWorker,
		RuntimeKind:  runtime.KindCodex,
		Image:        "worker:latest",
		WorkspaceRef: WorkspaceRef{Kind: WorkspaceKindDir, Path: workspaceRoot},
		UpdatedAt:    publishedAt,
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if got, want := published.ID, "frontend-alice"; got != want {
		t.Fatalf("Publish().ID = %q, want %q", got, want)
	}
	if got, want := published.WorkspaceRef.Kind, WorkspaceKindDir; got != want {
		t.Fatalf("Publish().WorkspaceRef.Kind = %q, want %q", got, want)
	}
	if got, want := published.Role, TemplateRoleWorker; got != want {
		t.Fatalf("Publish().Role = %q, want %q", got, want)
	}
	if got, want := published.UpdatedAt, publishedAt; !got.Equal(want) {
		t.Fatalf("Publish().UpdatedAt = %v, want %v", got, want)
	}

	listed, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := len(listed), 1; got != want {
		t.Fatalf("len(List()) = %d, want %d", got, want)
	}
	if got, want := listed[0].Name, "frontend-alice"; got != want {
		t.Fatalf("List()[0].Name = %q, want %q", got, want)
	}

	got, err := store.Get(context.Background(), "frontend-alice")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.RuntimeKind != runtime.KindCodex {
		t.Fatalf("Get().RuntimeKind = %q, want %q", got.RuntimeKind, runtime.KindCodex)
	}
	if got.Role != TemplateRoleWorker {
		t.Fatalf("Get().Role = %q, want %q", got.Role, TemplateRoleWorker)
	}
	if got.Image != "worker:latest" {
		t.Fatalf("Get().Image = %q, want %q", got.Image, "worker:latest")
	}

	workspace, err := store.FetchWorkspace(context.Background(), "frontend-alice")
	if err != nil {
		t.Fatalf("FetchWorkspace() error = %v", err)
	}
	if workspace.Kind != WorkspaceKindDir {
		t.Fatalf("FetchWorkspace().Kind = %q, want %q", workspace.Kind, WorkspaceKindDir)
	}

	agentsData, err := os.ReadFile(filepath.Join(workspace.Path, "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	if string(agentsData) != "agent" {
		t.Fatalf("AGENTS.md contents = %q, want %q", string(agentsData), "agent")
	}
	skillInfo, err := os.Stat(filepath.Join(workspace.Path, "skills", "frontend.txt"))
	if err != nil {
		t.Fatalf("Stat(skill) error = %v", err)
	}
	if skillInfo.Mode().Perm()&0o111 == 0 {
		t.Fatalf("skill mode = %o, want executable bit preserved", skillInfo.Mode().Perm())
	}
}

func TestLocalStoreDeleteRemovesTemplate(t *testing.T) {
	store := NewLocalStore(t.TempDir())
	if _, err := store.Publish(context.Background(), PublishSpec{
		ID:          "frontend-alice",
		Name:        "frontend-alice",
		Role:        TemplateRoleWorker,
		RuntimeKind: "picoclaw_sandbox",
		Image:       "worker:latest",
	}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if err := store.Delete(context.Background(), "frontend-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get(context.Background(), "frontend-alice"); !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("Get() after Delete() error = %v, want %v", err, ErrTemplateNotFound)
	}
}

func TestLocalStorePublishRejectsUnsafeTemplateID(t *testing.T) {
	store := NewLocalStore(t.TempDir())
	workspaceRoot := writeWorkspaceFile(t, "workspace", "AGENTS.md", "agent")

	_, err := store.Publish(context.Background(), PublishSpec{
		ID:           "../escape",
		Name:         "frontend-alice",
		Role:         TemplateRoleWorker,
		RuntimeKind:  runtime.KindCodex,
		WorkspaceRef: WorkspaceRef{Kind: WorkspaceKindDir, Path: workspaceRoot},
	})
	if !errors.Is(err, ErrWorkspacePathUnsafe) {
		t.Fatalf("Publish() error = %v, want ErrWorkspacePathUnsafe", err)
	}
}

func TestLocalStorePublishAllowsEmptyWorkspace(t *testing.T) {
	store := NewLocalStore(t.TempDir())
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	published, err := store.Publish(context.Background(), PublishSpec{
		Name:         "frontend-alice",
		Role:         TemplateRoleWorker,
		RuntimeKind:  runtime.KindCodex,
		WorkspaceRef: WorkspaceRef{Kind: WorkspaceKindDir, Path: workspaceRoot},
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if got, want := published.WorkspaceRef.Kind, WorkspaceKindDir; got != want {
		t.Fatalf("Publish().WorkspaceRef.Kind = %q, want %q", got, want)
	}
	entries, err := os.ReadDir(filepath.Join(store.templatesRoot(), "frontend-alice", "workspace"))
	if err != nil {
		t.Fatalf("ReadDir(workspace) error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("workspace entries = %d, want 0", len(entries))
	}
}

func TestLocalStorePublishAllowsMissingWorkspace(t *testing.T) {
	store := NewLocalStore(t.TempDir())

	published, err := store.Publish(context.Background(), PublishSpec{
		Name:        "frontend-alice",
		Role:        TemplateRoleWorker,
		RuntimeKind: runtime.KindCodex,
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if published.WorkspaceRef != (WorkspaceRef{}) {
		t.Fatalf("Publish().WorkspaceRef = %#v, want empty", published.WorkspaceRef)
	}

	got, err := store.Get(context.Background(), "frontend-alice")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.WorkspaceRef != (WorkspaceRef{}) {
		t.Fatalf("Get().WorkspaceRef = %#v, want empty", got.WorkspaceRef)
	}

	workspace, err := store.FetchWorkspace(context.Background(), "frontend-alice")
	if err != nil {
		t.Fatalf("FetchWorkspace() error = %v", err)
	}
	if workspace != (WorkspaceRef{}) {
		t.Fatalf("FetchWorkspace() = %#v, want empty", workspace)
	}
}

func TestLocalStorePublishRequiresImageForGatewayRuntime(t *testing.T) {
	store := NewLocalStore(t.TempDir())
	workspaceRoot := writeWorkspaceFile(t, "workspace", "AGENTS.md", "agent")

	_, err := store.Publish(context.Background(), PublishSpec{
		Name:         "gateway-worker",
		Role:         TemplateRoleWorker,
		RuntimeKind:  runtime.KindPicoClawSandbox,
		WorkspaceRef: WorkspaceRef{Kind: WorkspaceKindDir, Path: workspaceRoot},
	})
	if err == nil || err.Error() != `image.ref is required for runtime_kind "picoclaw_sandbox"` {
		t.Fatalf("Publish() error = %v, want missing image error", err)
	}
}

func TestLocalStoreGetRejectsGatewayRuntimeWithoutImage(t *testing.T) {
	registryRoot := t.TempDir()
	templateDir := filepath.Join(registryRoot, localTemplatesDirName, "gateway-worker")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	manifest := []byte("name = \"gateway-worker\"\nrole = \"worker\"\nruntime_kind = \"picoclaw_sandbox\"\n")
	if err := os.WriteFile(filepath.Join(templateDir, localManifestFileName), manifest, 0o644); err != nil {
		t.Fatalf("WriteFile(agent.toml) error = %v", err)
	}

	store := NewLocalStore(registryRoot)
	_, err := store.Get(context.Background(), "gateway-worker")
	if err == nil || err.Error() != `validate local hub manifest "gateway-worker": image.ref is required for runtime_kind "picoclaw_sandbox"` {
		t.Fatalf("Get() error = %v, want missing image validation error", err)
	}
}

func TestLocalStorePublishRejectsSymlinks(t *testing.T) {
	store := NewLocalStore(t.TempDir())
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	target := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(target, []byte("outside"), 0o644); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	linkPath := filepath.Join(workspaceRoot, "outside.txt")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Skipf("Symlink() unsupported: %v", err)
	}

	_, err := store.Publish(context.Background(), PublishSpec{
		Name:         "frontend-alice",
		Role:         TemplateRoleWorker,
		RuntimeKind:  runtime.KindCodex,
		WorkspaceRef: WorkspaceRef{Kind: WorkspaceKindDir, Path: workspaceRoot},
	})
	if !errors.Is(err, ErrWorkspaceSymlinkDenied) {
		t.Fatalf("Publish() error = %v, want ErrWorkspaceSymlinkDenied", err)
	}
}

func TestLocalStoreGetMissingTemplate(t *testing.T) {
	store := NewLocalStore(t.TempDir())

	_, err := store.Get(context.Background(), "missing")
	if !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("Get() error = %v, want ErrTemplateNotFound", err)
	}
}

func TestLocalStorePublishRejectsMissingRole(t *testing.T) {
	store := NewLocalStore(t.TempDir())

	_, err := store.Publish(context.Background(), PublishSpec{
		Name:        "frontend-alice",
		RuntimeKind: runtime.KindCodex,
	})
	if err == nil || err.Error() != `role must be one of "manager" or "worker"` {
		t.Fatalf("Publish() error = %v, want missing role validation error", err)
	}
}

func writeWorkspaceFile(t *testing.T, dirName, relPath, contents string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), dirName)
	fullPath := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(fullPath), err)
	}
	if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", fullPath, err)
	}
	return root
}
