package agent

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	runtimecodex "csgclaw/internal/runtime/codex"
	"csgclaw/internal/runtime/openclawsandbox"
	"csgclaw/internal/runtime/picoclawsandbox"
	"csgclaw/internal/templates"
)

var (
	ErrWorkspaceEmpty         = errors.New("workspace must contain at least one file")
	ErrWorkspacePathUnsafe    = errors.New("workspace path is unsafe")
	ErrWorkspaceSymlinkDenied = errors.New("workspace symlinks are not supported")
)

// managerGatewayMatch reports whether a gateway run should use the PicoClaw manager template,
// by agent name and bot id.
func managerGatewayMatch(name, botID string) bool {
	return strings.EqualFold(strings.TrimSpace(name), ManagerName) || strings.TrimSpace(botID) == ManagerUserID
}

func workspaceTemplateForAgent(name, botID string) (string, error) {
	role := RoleWorker
	if managerGatewayMatch(name, botID) {
		role = RoleManager
	}
	return templates.Resolve(RuntimeKindPicoClawSandbox, role)
}

func resolveRuntimeTemplateRoot(runtimeKind, role string) (string, error) {
	return templates.Resolve(runtimeKind, role)
}

func runtimeTemplateManifestPath(templateRoot string) string {
	return templates.ManifestPath(templateRoot)
}

func runtimeTemplateWorkspacePath(templateRoot string) string {
	return templates.WorkspacePath(templateRoot)
}

func ensureAgentWorkspace(agentName, template string) (string, error) {
	hostRoot, err := agentWorkspaceRoot(agentName, RuntimeKindPicoClawSandbox)
	if err != nil {
		return "", err
	}
	return ensureWorkspaceAtRoot(hostRoot, template)
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

func agentWorkspaceRoot(agentName, runtimeKind string) (string, error) {
	agentHome, err := agentHomeDir(agentName)
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(runtimeKind) {
	case RuntimeKindPicoClawSandbox:
		return picoclawsandbox.WorkspaceRoot(agentHome), nil
	case RuntimeKindOpenClawSandbox:
		return openclawsandbox.WorkspaceRoot(agentHome), nil
	case RuntimeKindCodex:
		return runtimecodex.WorkspaceRoot(agentHome), nil
	default:
		return "", fmt.Errorf("unsupported runtime_kind %q for agent workspace", runtimeKind)
	}
}

func copyEmbeddedTree(templateRoot, dstRoot string) error {
	templateRoot = strings.Trim(strings.TrimSpace(templateRoot), "/")
	if templateRoot == "" {
		return fmt.Errorf("runtime template root is required")
	}
	if _, err := fs.Stat(templates.FS(), runtimeTemplateManifestPath(templateRoot)); err != nil {
		return fmt.Errorf("stat embedded runtime template manifest %q: %w", templateRoot, err)
	}
	return copyWorkspaceFS(templates.FS(), runtimeTemplateWorkspacePath(templateRoot), dstRoot, "embedded workspace", false)
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
