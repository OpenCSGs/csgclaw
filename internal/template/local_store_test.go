package template

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	memoryPath := filepath.Join(t.TempDir(), "memory_summary.md")
	if err := os.WriteFile(memoryPath, []byte("# Learned context\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(memory) error = %v", err)
	}

	store := NewLocalStore(registryRoot)
	publishedAt := time.Date(2026, 5, 12, 8, 30, 0, 0, time.UTC)
	published, err := store.Publish(context.Background(), PublishSpec{
		Name:           "frontend-alice",
		IncludeMemory:  true,
		Description:    "Frontend worker with UI and styling skills",
		RuntimeKind:    runtime.KindCodex,
		Image:          "worker:latest",
		RuntimeOptions: map[string]any{"execution_mode": "read_only", "memory_mode": "enabled"},
		WorkspaceRef:   WorkspaceRef{Kind: WorkspaceKindDir, Path: workspaceRoot, MemoryPath: memoryPath},
		UpdatedAt:      publishedAt,
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
	if got, want := published.RuntimeOptions["execution_mode"], "read_only"; got != want {
		t.Fatalf("Publish().RuntimeOptions[execution_mode] = %v, want %q", got, want)
	}
	if got, want := published.RuntimeOptions["memory_mode"], "enabled"; got != want {
		t.Fatalf("Publish().RuntimeOptions[memory_mode] = %v, want %q", got, want)
	}
	if got, want := published.UpdatedAt, publishedAt; !got.Equal(want) {
		t.Fatalf("Publish().UpdatedAt = %v, want %v", got, want)
	}
	storedMemory, err := os.ReadFile(filepath.Join(registryRoot, localTemplatesDirName, "frontend-alice", localMemoriesDirName, "memory_summary.md"))
	if err != nil || string(storedMemory) != "# Learned context\n" {
		t.Fatalf("stored memory_summary.md = %q, error = %v", storedMemory, err)
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
	if gotMode, want := got.RuntimeOptions["execution_mode"], "read_only"; gotMode != want {
		t.Fatalf("Get().RuntimeOptions[execution_mode] = %v, want %q", gotMode, want)
	}
	if gotMode, want := got.RuntimeOptions["memory_mode"], "enabled"; gotMode != want {
		t.Fatalf("Get().RuntimeOptions[memory_mode] = %v, want %q", gotMode, want)
	}
	manifestData, err := os.ReadFile(filepath.Join(registryRoot, localTemplatesDirName, "frontend-alice", localManifestFileName))
	if err != nil {
		t.Fatalf("ReadFile(agent.toml) error = %v", err)
	}
	if !strings.Contains(string(manifestData), "[runtime_options]") ||
		!strings.Contains(string(manifestData), "execution_mode = 'read_only'") ||
		!strings.Contains(string(manifestData), "memory_mode = 'enabled'") {
		t.Fatalf("agent.toml missing runtime options:\n%s", manifestData)
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
	if data, err := os.ReadFile(workspace.MemoryPath); err != nil || string(data) != "# Learned context\n" {
		t.Fatalf("materialized memory = %q, error = %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(workspace.Path, "MEMORY.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Codex memory leaked into workspace, stat error = %v", err)
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

func TestLocalStorePublishOmitsCodexMemoryWhenDisabled(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "AGENTS.md"), []byte("# Instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	memoryPath := filepath.Join(t.TempDir(), "memory_summary.md")
	if err := os.WriteFile(memoryPath, []byte("# Must not publish\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	registryRoot := t.TempDir()
	store := NewLocalStore(registryRoot)
	created, err := store.Publish(context.Background(), PublishSpec{
		Name:           "disabled-memory",
		RuntimeKind:    runtime.KindCodex,
		RuntimeOptions: map[string]any{"memory_mode": "disabled"},
		WorkspaceRef:   WorkspaceRef{Kind: WorkspaceKindDir, Path: workspaceRoot, MemoryPath: memoryPath},
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	storedPath := filepath.Join(registryRoot, localTemplatesDirName, created.ID, localMemoriesDirName, "memory_summary.md")
	if _, err := os.Stat(storedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disabled memory snapshot stat error = %v, want not exist", err)
	}
	workspace, err := store.FetchWorkspace(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("FetchWorkspace() error = %v", err)
	}
	defer os.RemoveAll(workspace.Path)
	if workspace.MemoryPath != "" {
		t.Fatalf("FetchWorkspace().MemoryPath = %q, want empty", workspace.MemoryPath)
	}
}

func TestLocalStorePublishRejectsDuplicateName(t *testing.T) {
	store := NewLocalStore(t.TempDir())
	first, err := store.Publish(context.Background(), PublishSpec{
		Name:        "review-bot",
		Description: "original",
		RuntimeKind: runtime.KindCodex,
	})
	if err != nil {
		t.Fatalf("first Publish() error = %v", err)
	}
	_, err = store.Publish(context.Background(), PublishSpec{
		Name:        "review-bot",
		Description: "replacement",
		RuntimeKind: runtime.KindCodex,
	})
	if !errors.Is(err, ErrTemplateAlreadyExists) {
		t.Fatalf("second Publish() error = %v, want ErrTemplateAlreadyExists", err)
	}
	got, err := store.Get(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Description != "original" {
		t.Fatalf("Description = %q, want original template preserved", got.Description)
	}
}

func TestLocalStoreListSkipsInvalidTemplates(t *testing.T) {
	registryRoot := t.TempDir()
	store := NewLocalStore(registryRoot)
	if _, err := store.Publish(context.Background(), PublishSpec{
		Name:        "valid_codex_worker",
		RuntimeKind: runtime.KindCodex,
	}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	invalidRoot := filepath.Join(registryRoot, localTemplatesDirName, "legacy-openclaw-worker")
	if err := os.MkdirAll(invalidRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	manifest := "name = \"legacy-openclaw-worker\"\nrole = \"worker\"\nruntime_kind = \"openclaw\"\n"
	if err := os.WriteFile(filepath.Join(invalidRoot, localManifestFileName), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile(agent.toml) error = %v", err)
	}

	listed, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := len(listed), 1; got != want {
		t.Fatalf("len(List()) = %d, want %d", got, want)
	}
	if got, want := listed[0].ID, "valid_codex_worker"; got != want {
		t.Fatalf("List()[0].ID = %q, want %q", got, want)
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
	if err := os.MkdirAll(filepath.Join(skillsRoot, ".system", "openai-docs", "references"), 0o755); err != nil {
		t.Fatalf("MkdirAll(system skill) error = %v", err)
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
	if err := os.WriteFile(filepath.Join(skillsRoot, ".system", "openai-docs", "references", "internal.md"), []byte("system skill\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(system skill) error = %v", err)
	}

	store := NewLocalStore(registryRoot)
	if _, err := store.Publish(context.Background(), PublishSpec{
		Name:        "codex-worker",
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
	if _, err := os.Stat(filepath.Join(templateRoot, localSkillsDirName, ".system")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("template system skills stat error = %v, want not exist", err)
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
	linkPath := filepath.Join(workspaceRoot, "USER.md")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Skipf("Symlink() unsupported: %v", err)
	}

	_, err := store.Publish(context.Background(), PublishSpec{
		Name:         "frontend-alice",
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

func TestLocalStorePublishTreatsTemplateAsWorker(t *testing.T) {
	store := NewLocalStore(t.TempDir())

	got, err := store.Publish(context.Background(), PublishSpec{
		Name:        "frontend-alice",
		RuntimeKind: runtime.KindCodex,
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if got.Role != TemplateRoleWorker {
		t.Fatalf("Publish().Role = %q, want %q", got.Role, TemplateRoleWorker)
	}
	manifest, err := os.ReadFile(filepath.Join(store.templatesRoot(), "frontend-alice", localManifestFileName))
	if err != nil {
		t.Fatalf("ReadFile(agent.toml) error = %v", err)
	}
	if strings.Contains(string(manifest), "role =") {
		t.Fatalf("agent.toml contains removed role field:\n%s", manifest)
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
