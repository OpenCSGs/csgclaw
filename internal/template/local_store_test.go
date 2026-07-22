package template

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
	if !workspace.Temporary {
		t.Fatal("FetchWorkspace().Temporary = false, want materialized local workspace to be caller-owned")
	}
	defer os.RemoveAll(workspace.Path)

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

func TestLocalStorePublishUsesRuntimeAwareInstructionAndSkillPaths(t *testing.T) {
	registryRoot := t.TempDir()
	runtimeRoot := t.TempDir()
	workspaceRoot := filepath.Join(runtimeRoot, "workspace")
	instructionsPath := filepath.Join(runtimeRoot, "home", "AGENTS.md")
	skillsRoot := filepath.Join(runtimeRoot, "home", "skills")
	if err := os.MkdirAll(filepath.Join(skillsRoot, "custom"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(instructionsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(home) error = %v", err)
	}
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	if err := os.WriteFile(instructionsPath, []byte("effective instructions\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENTS.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsRoot, "custom", "SKILL.md"), []byte("custom skill\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	store := NewLocalStore(registryRoot)
	if _, err := store.Publish(context.Background(), PublishSpec{
		Name:        "codex-worker",
		Role:        TemplateRoleWorker,
		RuntimeKind: runtime.KindCodex,
		WorkspaceRef: WorkspaceRef{
			Kind:             WorkspaceKindDir,
			Path:             workspaceRoot,
			InstructionsPath: instructionsPath,
			SkillsPath:       skillsRoot,
		},
	}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	templateRoot := filepath.Join(registryRoot, localTemplatesDirName, "codex-worker")
	if data, err := os.ReadFile(filepath.Join(templateRoot, localInstructionsDirName, requiredInstructionsFile)); err != nil {
		t.Fatalf("ReadFile(template AGENTS.md) error = %v", err)
	} else if got, want := string(data), "effective instructions\n"; got != want {
		t.Fatalf("template AGENTS.md = %q, want %q", got, want)
	}
	if data, err := os.ReadFile(filepath.Join(templateRoot, localSkillsDirName, "custom", "SKILL.md")); err != nil {
		t.Fatalf("ReadFile(template SKILL.md) error = %v", err)
	} else if got, want := string(data), "custom skill\n"; got != want {
		t.Fatalf("template SKILL.md = %q, want %q", got, want)
	}
}

func TestLocalStoreFetchWorkspaceSupportsLegacyLayout(t *testing.T) {
	registryRoot := t.TempDir()
	templateRoot := filepath.Join(registryRoot, localTemplatesDirName, "legacy-worker")
	legacyRoot := filepath.Join(templateRoot, "workspace")
	if err := os.MkdirAll(filepath.Join(legacyRoot, "skills", "legacy"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateRoot, localManifestFileName), []byte("name = \"legacy-worker\"\nrole = \"worker\"\nruntime_kind = \"codex\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(agent.toml) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "AGENTS.md"), []byte("legacy instructions\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENTS.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "skills", "legacy", "SKILL.md"), []byte("legacy skill\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	store := NewLocalStore(registryRoot)
	item, err := store.Get(context.Background(), "legacy-worker")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if item.WorkspaceRef.Kind != WorkspaceKindDir {
		t.Fatalf("Get().WorkspaceRef = %#v, want legacy workspace available", item.WorkspaceRef)
	}
	workspace, err := store.FetchWorkspace(context.Background(), "legacy-worker")
	if err != nil {
		t.Fatalf("FetchWorkspace() error = %v", err)
	}
	defer os.RemoveAll(workspace.Path)
	if !workspace.Temporary {
		t.Fatal("FetchWorkspace().Temporary = false, want true")
	}
	data, err := os.ReadFile(filepath.Join(workspace.Path, "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	if got, want := string(data), "legacy instructions\n"; got != want {
		t.Fatalf("AGENTS.md = %q, want %q", got, want)
	}
}

func TestLocalStoreWriteWorkspaceFileUpdatesCanonicalInstructions(t *testing.T) {
	registryRoot := t.TempDir()
	store := NewLocalStore(registryRoot)
	workspaceRoot := writeWorkspaceFile(t, "workspace", "AGENTS.md", "old instructions")
	if _, err := store.Publish(context.Background(), PublishSpec{
		Name:         "editable-worker",
		Role:         TemplateRoleWorker,
		RuntimeKind:  runtime.KindCodex,
		WorkspaceRef: WorkspaceRef{Kind: WorkspaceKindDir, Path: workspaceRoot},
	}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if err := store.WriteWorkspaceFile(context.Background(), "editable-worker", "instructions/AGENTS.md", "new instructions"); err != nil {
		t.Fatalf("WriteWorkspaceFile() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(registryRoot, localTemplatesDirName, "editable-worker", localInstructionsDirName, requiredInstructionsFile))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	if got, want := string(data), "new instructions\n"; got != want {
		t.Fatalf("AGENTS.md = %q, want %q", got, want)
	}
	if err := store.WriteWorkspaceFile(context.Background(), "editable-worker", "skills/demo/SKILL.md", "unsafe"); !errors.Is(err, ErrWorkspacePathUnsafe) {
		t.Fatalf("WriteWorkspaceFile(unsafe) error = %v, want ErrWorkspacePathUnsafe", err)
	}
}

func TestLocalStoreDeleteRemovesTemplate(t *testing.T) {
	store := NewLocalStore(t.TempDir())
	if _, err := store.Publish(context.Background(), PublishSpec{
		ID:          "frontend-alice",
		Name:        "frontend-alice",
		Role:        TemplateRoleWorker,
		RuntimeKind: "picoclaw",
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
	entries, err := os.ReadDir(filepath.Join(store.templatesRoot(), "frontend-alice", "instructions"))
	if err != nil {
		t.Fatalf("ReadDir(workspace) error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "AGENTS.md" {
		t.Fatalf("instructions entries = %#v, want generated AGENTS.md", entries)
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
		RuntimeKind:  runtime.NamePicoClaw,
		WorkspaceRef: WorkspaceRef{Kind: WorkspaceKindDir, Path: workspaceRoot},
	})
	if err == nil || err.Error() != `image.ref is required for runtime_kind "picoclaw"` {
		t.Fatalf("Publish() error = %v, want missing image error", err)
	}
}

func TestLocalStoreGetRejectsGatewayRuntimeWithoutImage(t *testing.T) {
	registryRoot := t.TempDir()
	templateDir := filepath.Join(registryRoot, localTemplatesDirName, "gateway-worker")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	manifest := []byte("name = \"gateway-worker\"\nrole = \"worker\"\nruntime_kind = \"picoclaw\"\n")
	if err := os.WriteFile(filepath.Join(templateDir, localManifestFileName), manifest, 0o644); err != nil {
		t.Fatalf("WriteFile(agent.toml) error = %v", err)
	}

	store := NewLocalStore(registryRoot)
	_, err := store.Get(context.Background(), "gateway-worker")
	if err == nil || err.Error() != `validate local hub manifest "gateway-worker": image.ref is required for runtime_kind "picoclaw"` {
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
