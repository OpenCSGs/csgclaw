package hub

import (
	"fmt"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	hubtemplates "csgclaw/internal/hub/templates"
	toml "github.com/pelletier/go-toml/v2"
)

func copyWorkspaceTree(srcRoot, dstRoot string) error {
	srcRoot = strings.TrimSpace(srcRoot)
	if srcRoot == "" {
		return ErrWorkspaceDirRequired
	}
	info, err := os.Lstat(srcRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrWorkspaceDirRequired, srcRoot)
		}
		return fmt.Errorf("stat hub workspace: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrWorkspaceSymlinkDenied
	}
	if !info.IsDir() {
		return ErrWorkspaceDirRequired
	}
	return copyWorkspaceTreeFS(os.DirFS(srcRoot), ".", dstRoot, "hub workspace")
}

func copyWorkspaceTreeFS(srcFS fs.FS, root, dstRoot, label string) error {
	dstRoot = strings.TrimSpace(dstRoot)
	if dstRoot == "" {
		return ErrWorkspaceDirRequired
	}
	root = strings.TrimSpace(root)
	err := fs.WalkDir(srcFS, root, func(path string, d fs.DirEntry, walkErr error) error {
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
		dstPath := filepath.Join(dstRoot, rel)
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrWorkspaceSymlinkDenied, rel)
		}
		if d.IsDir() {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				return fmt.Errorf("create workspace dir %q: %w", dstPath, err)
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("read workspace entry info %q: %w", path, err)
		}
		data, err := fs.ReadFile(srcFS, path)
		if err != nil {
			return fmt.Errorf("read workspace file %q: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return fmt.Errorf("create workspace parent %q: %w", filepath.Dir(dstPath), err)
		}
		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		mode |= 0o200
		if err := os.WriteFile(dstPath, data, mode); err != nil {
			return fmt.Errorf("write workspace file %q: %w", dstPath, err)
		}
		return nil
	})
	return err
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

func loadManifestFS(srcFS fs.FS, manifestPath, label string) (string, templateManifest, error) {
	id := templateIDFromManifestPath(manifestPath)
	if err := validateLocalTemplateID(id); err != nil {
		return "", templateManifest{}, err
	}

	var manifest templateManifest
	data, err := fs.ReadFile(srcFS, manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", templateManifest{}, fmt.Errorf("%w: %s", ErrTemplateNotFound, id)
		}
		return "", templateManifest{}, fmt.Errorf("read %s manifest %q: %w", label, id, err)
	}
	if err := toml.Unmarshal(data, &manifest); err != nil {
		return "", templateManifest{}, fmt.Errorf("decode %s manifest %q: %w", label, id, err)
	}
	if err := validateManifest(manifest); err != nil {
		return "", templateManifest{}, fmt.Errorf("validate %s manifest %q: %w", label, id, err)
	}
	return id, manifest, nil
}

func templateIDFromManifestPath(path string) string {
	return hubtemplates.TemplateIDFromManifestPath(filepath.ToSlash(strings.TrimSpace(path)))
}
