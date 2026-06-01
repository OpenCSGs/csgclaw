package sandboxgateway

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"csgclaw/internal/templates"
)

var (
	errWorkspaceEmpty         = errors.New("workspace must contain at least one file")
	errWorkspacePathUnsafe    = errors.New("workspace path is unsafe")
	errWorkspaceSymlinkDenied = errors.New("workspace symlinks are not supported")
)

func OverlayWorkspaceTree(srcRoot, dstRoot string) error {
	srcRoot = strings.TrimSpace(srcRoot)
	if srcRoot == "" {
		return fmt.Errorf("workspace source is required")
	}
	info, err := os.Lstat(srcRoot)
	if err != nil {
		return fmt.Errorf("stat workspace source %q: %w", srcRoot, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errWorkspaceSymlinkDenied
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace source %q is not a directory", srcRoot)
	}
	return copyWorkspaceFS(os.DirFS(srcRoot), ".", dstRoot, "workspace", true)
}

func EnsureEmbeddedWorkspace(templateRoot, dstRoot string) error {
	templateRoot = strings.Trim(strings.TrimSpace(templateRoot), "/")
	if templateRoot == "" {
		return fmt.Errorf("runtime template root is required")
	}
	if _, err := fs.Stat(templates.FS(), templates.ManifestPath(templateRoot)); err != nil {
		return fmt.Errorf("stat embedded runtime template manifest %q: %w", templateRoot, err)
	}
	return copyWorkspaceFS(templates.FS(), templates.WorkspacePath(templateRoot), dstRoot, "embedded workspace", false)
}

func EnsureWorkspaceProjectsMountpoint(workspaceRoot string) error {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return fmt.Errorf("workspace root is required")
	}
	// Keep the nested projects bind mount target present after the runtime root mount hides image defaults.
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "projects"), 0o755); err != nil {
		return fmt.Errorf("create workspace projects mountpoint: %w", err)
	}
	return nil
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
				return errWorkspaceSymlinkDenied
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
			return fmt.Errorf("%w: %s", errWorkspaceSymlinkDenied, rel)
		}
		if d.IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return fmt.Errorf("create workspace dir %q: %w", dst, err)
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("read workspace file info %q: %w", path, err)
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
		return errWorkspaceEmpty
	}
	return nil
}

func workspaceFSRelativePath(root, current string) (string, error) {
	root = strings.TrimSpace(root)
	current = strings.TrimSpace(current)
	if root == "" || current == "" {
		return "", errWorkspacePathUnsafe
	}
	if root == "." {
		return current, nil
	}
	rel := strings.TrimPrefix(current, root)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == current {
		return "", fmt.Errorf("%w: %s", errWorkspacePathUnsafe, current)
	}
	return rel, nil
}

func validateWorkspaceRelativePath(rel string) error {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return errWorkspacePathUnsafe
	}
	slashed := filepath.ToSlash(rel)
	if cleaned := pathpkg.Clean(slashed); cleaned == "." || cleaned == ".." {
		return errWorkspacePathUnsafe
	}
	if strings.HasPrefix(slashed, "../") {
		return errWorkspacePathUnsafe
	}
	if strings.Contains("/"+slashed+"/", "/../") {
		return errWorkspacePathUnsafe
	}
	if strings.HasSuffix(slashed, "/..") {
		return errWorkspacePathUnsafe
	}
	rel = filepath.Clean(rel)
	if rel == "." || rel == ".." {
		return errWorkspacePathUnsafe
	}
	if filepath.IsAbs(rel) {
		return errWorkspacePathUnsafe
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errWorkspacePathUnsafe
	}
	return nil
}
