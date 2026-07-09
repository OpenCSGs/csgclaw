package agent

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	agentruntime "csgclaw/internal/runtime"
	templateembed "csgclaw/internal/template/embed"
)

var (
	ErrWorkspaceEmpty         = errors.New("workspace must contain at least one file")
	ErrWorkspacePathUnsafe    = errors.New("workspace path is unsafe")
	ErrWorkspaceSymlinkDenied = errors.New("workspace symlinks are not supported")
)

// managerGatewayMatch reports whether a request targets the built-in manager,
// by agent name and bot id.
func managerGatewayMatch(name, botID string) bool {
	return strings.EqualFold(strings.TrimSpace(name), ManagerName) || strings.TrimSpace(botID) == ManagerUserID
}

func workspaceTemplateForAgent(name, botID string) (string, error) {
	if managerGatewayMatch(name, botID) {
		return templateembed.Resolve(RuntimeKindCodex, RoleManager)
	}
	return templateembed.Resolve(RuntimeKindPicoClawSandbox, RoleWorker)
}

func resolveRuntimeTemplateRoot(runtimeKind, role string) (string, error) {
	return templateembed.Resolve(runtimeKind, role)
}

func runtimeTemplateManifestPath(templateRoot string) string {
	return templateembed.ManifestPath(templateRoot)
}

func runtimeTemplateWorkspacePath(templateRoot string) string {
	return templateembed.WorkspacePath(templateRoot)
}

func ensureWorkspaceAtRoot(hostRoot, template string) (string, error) {
	if strings.TrimSpace(template) == "" {
		return "", fmt.Errorf("workspace template is required")
	}
	if err := os.MkdirAll(hostRoot, 0o755); err != nil {
		return "", fmt.Errorf("create agent workspace dir: %w", err)
	}
	if err := copyEmbeddedTree(template, hostRoot); err != nil {
		return "", err
	}
	return hostRoot, nil
}

func (s *Service) agentWorkspaceRoot(agentID, runtimeKind string) (string, error) {
	layout, err := s.agentLayout(agentID, runtimeKind)
	if err != nil {
		return "", err
	}
	return layout.WorkspaceRoot, nil
}

func (s *Service) agentSkillsRoot(agentID, runtimeKind string) (string, error) {
	layout, err := s.agentLayout(agentID, runtimeKind)
	if err != nil {
		return "", err
	}
	return layout.SkillsRoot, nil
}

func (s *Service) agentLayout(agentID, runtimeKind string) (agentruntime.Layout, error) {
	agentHome, err := s.agentHomeDir(agentID)
	if err != nil {
		return agentruntime.Layout{}, err
	}
	rt, err := s.runtimeForKind(strings.TrimSpace(runtimeKind))
	if err != nil {
		return agentruntime.Layout{}, err
	}
	layout := rt.Layout(agentHome)
	if strings.TrimSpace(layout.WorkspaceRoot) == "" {
		return agentruntime.Layout{}, fmt.Errorf("runtime %q returned empty workspace root", rt.Kind())
	}
	if strings.TrimSpace(layout.SkillsRoot) == "" {
		return agentruntime.Layout{}, fmt.Errorf("runtime %q returned empty skills root", rt.Kind())
	}
	return layout, nil
}

func copyEmbeddedTree(templateRoot, dstRoot string) error {
	templateRoot = strings.Trim(strings.TrimSpace(templateRoot), "/")
	if templateRoot == "" {
		return fmt.Errorf("runtime template root is required")
	}
	if _, err := fs.Stat(templateembed.FS(), runtimeTemplateManifestPath(templateRoot)); err != nil {
		return fmt.Errorf("stat embedded runtime template manifest %q: %w", templateRoot, err)
	}
	return copyWorkspaceFS(templateembed.FS(), runtimeTemplateWorkspacePath(templateRoot), dstRoot, "embedded workspace", false)
}

func overlayWorkspaceTree(srcRoot, dstRoot string) error {
	srcRoot = strings.TrimSpace(srcRoot)
	if srcRoot == "" {
		return fmt.Errorf("workspace source is required")
	}
	info, err := os.Lstat(srcRoot)
	if err != nil {
		return fmt.Errorf("stat workspace source %q: %w", srcRoot, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrWorkspaceSymlinkDenied
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace source %q is not a directory", srcRoot)
	}
	return copyWorkspaceFS(os.DirFS(srcRoot), ".", dstRoot, "workspace", true)
}

func (s *Service) prepareWorkspaceSkillsPreservation(agentID, sourceRuntimeKind, targetRuntimeKind, role string) (func() error, func(), error) {
	sourceRuntimeKind = strings.TrimSpace(sourceRuntimeKind)
	targetRuntimeKind = strings.TrimSpace(targetRuntimeKind)
	if sourceRuntimeKind == "" {
		sourceRuntimeKind = targetRuntimeKind
	}
	if targetRuntimeKind == "" {
		targetRuntimeKind = sourceRuntimeKind
	}
	sourceSkills, err := s.agentSkillsRoot(agentID, sourceRuntimeKind)
	if err != nil {
		return nil, nil, err
	}
	info, err := os.Stat(sourceSkills)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("stat workspace skills: %w", err)
	}
	if !info.IsDir() {
		return nil, nil, nil
	}

	tempDir, err := os.MkdirTemp("", "csgclaw-preserve-skills-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create skills preservation dir: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	preservedSkills := filepath.Join(tempDir, "skills")
	if err := copyWorkspaceFS(os.DirFS(sourceSkills), ".", preservedSkills, "workspace skills", true); err != nil {
		cleanup()
		if errors.Is(err, ErrWorkspaceEmpty) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	templateNames, err := managedWorkspaceSkillNames(targetRuntimeKind, role)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	for name := range templateNames {
		if err := os.RemoveAll(filepath.Join(preservedSkills, name)); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("drop template skill %q from preservation set: %w", name, err)
		}
	}

	restore := func() error {
		if empty, err := directoryEmpty(preservedSkills); err != nil || empty {
			return err
		}
		targetSkills, err := s.agentSkillsRoot(agentID, targetRuntimeKind)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(targetSkills, 0o755); err != nil {
			return fmt.Errorf("create target workspace skills dir: %w", err)
		}
		return overlayWorkspaceTree(preservedSkills, targetSkills)
	}
	return restore, cleanup, nil
}

func (s *Service) refreshGatewayTemplateSkills(agentID, runtimeKind, role string) error {
	runtimeKind = strings.TrimSpace(runtimeKind)
	if !isGatewayRuntimeKind(runtimeKind) {
		return nil
	}
	skillsRoot, err := s.agentSkillsRoot(agentID, runtimeKind)
	if err != nil {
		return err
	}
	templateNames, err := managedWorkspaceSkillNames(runtimeKind, role)
	if err != nil {
		return err
	}
	for name := range templateNames {
		if err := os.RemoveAll(filepath.Join(skillsRoot, name)); err != nil {
			return fmt.Errorf("remove template skill %q: %w", name, err)
		}
	}
	return nil
}

func recreateTemplateRole(a Agent) string {
	if isManagerAgent(a) {
		return RoleManager
	}
	if role := normalizeRole(a.Role); role == RoleManager || role == RoleWorker {
		return role
	}
	return RoleWorker
}

func templateWorkspaceSkillNames(runtimeKind, role string) (map[string]struct{}, error) {
	names := map[string]struct{}{}
	templateRoot, err := resolveRuntimeTemplateRoot(runtimeKind, role)
	if err != nil {
		return names, err
	}
	skillsRoot := pathpkg.Join(runtimeTemplateWorkspacePath(templateRoot), "skills")
	entries, err := fs.ReadDir(templateembed.FS(), skillsRoot)
	if errors.Is(err, fs.ErrNotExist) {
		return names, nil
	}
	if err != nil {
		return names, fmt.Errorf("read template skills: %w", err)
	}
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name != "" {
			names[name] = struct{}{}
		}
	}
	return names, nil
}

func managedWorkspaceSkillNames(runtimeKind, role string) (map[string]struct{}, error) {
	names, err := templateWorkspaceSkillNames(runtimeKind, role)
	if err != nil {
		return nil, err
	}
	systemNames, err := defaultSystemSkillNames()
	if err != nil {
		return nil, err
	}
	for _, name := range systemNames {
		names[name] = struct{}{}
	}
	return names, nil
}

func directoryEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func copyWorkspaceFS(srcFS fs.FS, root, dstRoot, label string, overwrite bool) error {
	dstRoot = strings.TrimSpace(dstRoot)
	if dstRoot == "" {
		return fmt.Errorf("workspace destination is required")
	}
	root = strings.TrimSpace(root)
	fileCount := 0
	return finalizeWorkspaceCopy(fs.WalkDir(srcFS, root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk %s %q: %w", label, root, walkErr)
		}
		if path == root {
			if d.Type()&os.ModeSymlink != 0 {
				return ErrWorkspaceSymlinkDenied
			}
			return nil
		}
		rel, err := workspaceFSRelativePath(root, path)
		if err != nil {
			return err
		}
		rel = filepath.FromSlash(rel)
		if err := validateWorkspaceRelativePath(rel); err != nil {
			return err
		}
		if rel == "" {
			return nil
		}
		dst := filepath.Join(dstRoot, filepath.FromSlash(rel))
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrWorkspaceSymlinkDenied, rel)
		}
		if d.IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return fmt.Errorf("create workspace dir %q: %w", dst, err)
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("read embedded workspace file info %q: %w", path, err)
		}
		data, err := fs.ReadFile(srcFS, path)
		if err != nil {
			return fmt.Errorf("read workspace file %q: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("create workspace parent %q: %w", filepath.Dir(dst), err)
		}
		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		mode |= 0o200
		if !overwrite {
			if _, err := os.Stat(dst); err == nil {
				fileCount++
				return nil
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("stat workspace file %q: %w", dst, err)
			}
		}
		if err := os.WriteFile(dst, data, mode); err != nil {
			return fmt.Errorf("write workspace file %q: %w", dst, err)
		}
		fileCount++
		return nil
	}), fileCount)
}

func finalizeWorkspaceCopy(err error, fileCount int) error {
	if err != nil {
		return err
	}
	if fileCount == 0 {
		return ErrWorkspaceEmpty
	}
	return nil
}

func workspaceFSRelativePath(root, current string) (string, error) {
	root = strings.TrimSpace(root)
	current = strings.TrimSpace(current)
	if root == "" || current == "" {
		return "", ErrWorkspacePathUnsafe
	}
	if root == "." {
		return current, nil
	}
	rel := strings.TrimPrefix(current, root)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == current {
		return "", fmt.Errorf("%w: %s", ErrWorkspacePathUnsafe, current)
	}
	return rel, nil
}

func validateWorkspaceRelativePath(rel string) error {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return ErrWorkspacePathUnsafe
	}
	slashed := filepath.ToSlash(rel)
	if cleaned := pathpkg.Clean(slashed); cleaned == "." || cleaned == ".." {
		return ErrWorkspacePathUnsafe
	}
	if strings.HasPrefix(slashed, "../") {
		return ErrWorkspacePathUnsafe
	}
	if strings.Contains("/"+slashed+"/", "/../") {
		return ErrWorkspacePathUnsafe
	}
	if strings.HasSuffix(slashed, "/..") {
		return ErrWorkspacePathUnsafe
	}
	rel = filepath.Clean(rel)
	if rel == "." || rel == ".." {
		return ErrWorkspacePathUnsafe
	}
	if filepath.IsAbs(rel) {
		return ErrWorkspacePathUnsafe
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ErrWorkspacePathUnsafe
	}
	if strings.Contains(rel, string(filepath.Separator)+".."+string(filepath.Separator)) {
		return ErrWorkspacePathUnsafe
	}
	if strings.HasSuffix(rel, string(filepath.Separator)+"..") {
		return ErrWorkspacePathUnsafe
	}
	if cleaned := pathpkg.Clean(filepath.ToSlash(rel)); cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return ErrWorkspacePathUnsafe
	}
	return nil
}
