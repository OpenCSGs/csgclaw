package filebrowse

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"csgclaw/internal/apitypes"
)

const FilePreviewMaxBytes = 256 * 1024

func List(root, relativePath string) (apitypes.WorkspaceListing, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return apitypes.WorkspaceListing{}, fmt.Errorf("workspace root is required")
	}
	cleanPath, err := cleanRelativePath(relativePath)
	if err != nil {
		return apitypes.WorkspaceListing{}, err
	}
	base := root
	if cleanPath != "" {
		base = filepath.Join(root, filepath.FromSlash(cleanPath))
	}
	info, err := os.Lstat(base)
	if err != nil {
		return apitypes.WorkspaceListing{}, fmt.Errorf("stat workspace %q: %w", cleanPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return apitypes.WorkspaceListing{}, fmt.Errorf("workspace path %q is a symlink", cleanPath)
	}
	if !info.IsDir() {
		return apitypes.WorkspaceListing{}, fmt.Errorf("workspace path %q is not a directory", cleanPath)
	}

	entries := make([]apitypes.WorkspaceEntry, 0)
	err = filepath.WalkDir(base, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == base {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		entry := apitypes.WorkspaceEntry{
			Path:  rel,
			Name:  d.Name(),
			Type:  "file",
			Depth: strings.Count(rel, "/"),
			Size:  info.Size(),
		}
		if d.IsDir() {
			entry.Type = "dir"
			entry.Size = 0
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return apitypes.WorkspaceListing{}, fmt.Errorf("walk workspace %q: %w", cleanPath, err)
	}
	return apitypes.WorkspaceListing{
		Kind:    "dir",
		Path:    cleanPath,
		Entries: entries,
	}, nil
}

func ListDirectory(root, relativePath string) (apitypes.WorkspaceListing, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return apitypes.WorkspaceListing{}, fmt.Errorf("workspace root is required")
	}
	cleanPath, err := cleanRelativePath(relativePath)
	if err != nil {
		return apitypes.WorkspaceListing{}, err
	}
	base := root
	if cleanPath != "" {
		base = filepath.Join(root, filepath.FromSlash(cleanPath))
	}
	info, err := os.Lstat(base)
	if err != nil {
		return apitypes.WorkspaceListing{}, fmt.Errorf("stat workspace %q: %w", cleanPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return apitypes.WorkspaceListing{}, fmt.Errorf("workspace path %q is a symlink", cleanPath)
	}
	if !info.IsDir() {
		return apitypes.WorkspaceListing{}, fmt.Errorf("workspace path %q is not a directory", cleanPath)
	}

	children, err := os.ReadDir(base)
	if err != nil {
		return apitypes.WorkspaceListing{}, fmt.Errorf("read workspace directory %q: %w", cleanPath, err)
	}
	entries := make([]apitypes.WorkspaceEntry, 0, len(children))
	for _, child := range children {
		if child.Type()&os.ModeSymlink != 0 {
			continue
		}
		childInfo, err := child.Info()
		if err != nil {
			return apitypes.WorkspaceListing{}, fmt.Errorf("read workspace entry %q: %w", child.Name(), err)
		}
		entryPath := child.Name()
		if cleanPath != "" {
			entryPath = cleanPath + "/" + child.Name()
		}
		entry := apitypes.WorkspaceEntry{
			Path:  entryPath,
			Name:  child.Name(),
			Type:  "file",
			Depth: strings.Count(entryPath, "/"),
			Size:  childInfo.Size(),
		}
		if child.IsDir() {
			entry.Type = "dir"
			entry.Size = 0
		}
		entries = append(entries, entry)
	}
	return apitypes.WorkspaceListing{
		Kind:    "dir",
		Path:    cleanPath,
		Entries: entries,
	}, nil
}

func ReadFile(root, relativePath string) (apitypes.WorkspaceFile, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return apitypes.WorkspaceFile{}, fmt.Errorf("workspace root is required")
	}
	cleanPath, err := cleanFilePath(relativePath)
	if err != nil {
		return apitypes.WorkspaceFile{}, err
	}
	absPath := filepath.Join(root, filepath.FromSlash(cleanPath))
	info, err := os.Lstat(absPath)
	if err != nil {
		return apitypes.WorkspaceFile{}, fmt.Errorf("stat workspace file %q: %w", cleanPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return apitypes.WorkspaceFile{}, fmt.Errorf("workspace file %q is a symlink", cleanPath)
	}
	if info.IsDir() {
		return apitypes.WorkspaceFile{}, fmt.Errorf("workspace path %q is a directory", cleanPath)
	}
	handle, err := os.Open(absPath)
	if err != nil {
		return apitypes.WorkspaceFile{}, fmt.Errorf("read workspace file %q: %w", cleanPath, err)
	}
	defer handle.Close()
	data, err := io.ReadAll(io.LimitReader(handle, int64(FilePreviewMaxBytes)))
	if err != nil {
		return apitypes.WorkspaceFile{}, fmt.Errorf("read workspace file %q: %w", cleanPath, err)
	}
	file := apitypes.WorkspaceFile{
		Path: filepath.ToSlash(cleanPath),
		Size: info.Size(),
	}
	if info.Size() > int64(FilePreviewMaxBytes) {
		file.Truncated = true
		var ok bool
		data, ok = trimUTF8Preview(data)
		if !ok {
			file.Binary = true
			return file, nil
		}
	} else if !utf8.Valid(data) {
		file.Binary = true
		return file, nil
	}
	file.Content = string(data)
	return file, nil
}

func ListFS(sourceFS fs.FS, root, relativePath string) (apitypes.WorkspaceListing, error) {
	root = strings.Trim(strings.TrimSpace(root), "/")
	if root == "" {
		return apitypes.WorkspaceListing{}, fmt.Errorf("workspace root is required")
	}
	cleanPath, err := cleanFSRelativePath(relativePath)
	if err != nil {
		return apitypes.WorkspaceListing{}, err
	}
	base := root
	if cleanPath != "" {
		base = pathpkg.Join(root, cleanPath)
	}
	info, err := fs.Stat(sourceFS, base)
	if err != nil {
		return apitypes.WorkspaceListing{}, fmt.Errorf("stat workspace %q: %w", cleanPath, err)
	}
	if !info.IsDir() {
		return apitypes.WorkspaceListing{}, fmt.Errorf("workspace path %q is not a directory", cleanPath)
	}

	entries := make([]apitypes.WorkspaceEntry, 0)
	err = fs.WalkDir(sourceFS, base, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == base {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		rel := strings.TrimPrefix(path, strings.TrimRight(root, "/")+"/")
		if rel == path {
			return fmt.Errorf("workspace path %q is outside root", path)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entry := apitypes.WorkspaceEntry{
			Path:  rel,
			Name:  d.Name(),
			Type:  "file",
			Depth: strings.Count(rel, "/"),
			Size:  info.Size(),
		}
		if d.IsDir() {
			entry.Type = "dir"
			entry.Size = 0
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return apitypes.WorkspaceListing{}, fmt.Errorf("walk workspace %q: %w", cleanPath, err)
	}
	return apitypes.WorkspaceListing{
		Kind:    "dir",
		Path:    cleanPath,
		Entries: entries,
	}, nil
}

func ReadFileFS(sourceFS fs.FS, root, relativePath string) (apitypes.WorkspaceFile, error) {
	root = strings.Trim(strings.TrimSpace(root), "/")
	if root == "" {
		return apitypes.WorkspaceFile{}, fmt.Errorf("workspace root is required")
	}
	cleanPath, err := cleanFSFilePath(relativePath)
	if err != nil {
		return apitypes.WorkspaceFile{}, err
	}
	absPath := pathpkg.Join(root, cleanPath)
	info, err := fs.Stat(sourceFS, absPath)
	if err != nil {
		return apitypes.WorkspaceFile{}, fmt.Errorf("stat workspace file %q: %w", cleanPath, err)
	}
	if info.IsDir() {
		return apitypes.WorkspaceFile{}, fmt.Errorf("workspace path %q is a directory", cleanPath)
	}
	handle, err := sourceFS.Open(absPath)
	if err != nil {
		return apitypes.WorkspaceFile{}, fmt.Errorf("read workspace file %q: %w", cleanPath, err)
	}
	defer handle.Close()
	data, err := io.ReadAll(io.LimitReader(handle, int64(FilePreviewMaxBytes)))
	if err != nil {
		return apitypes.WorkspaceFile{}, fmt.Errorf("read workspace file %q: %w", cleanPath, err)
	}
	file := apitypes.WorkspaceFile{
		Path: cleanPath,
		Size: info.Size(),
	}
	if info.Size() > int64(FilePreviewMaxBytes) {
		file.Truncated = true
		var ok bool
		data, ok = trimUTF8Preview(data)
		if !ok {
			file.Binary = true
			return file, nil
		}
	} else if !utf8.Valid(data) {
		file.Binary = true
		return file, nil
	}
	file.Content = string(data)
	return file, nil
}

func trimUTF8Preview(data []byte) ([]byte, bool) {
	if utf8.Valid(data) {
		return data, true
	}
	for trim := 1; trim < utf8.UTFMax && trim < len(data); trim++ {
		preview := data[:len(data)-trim]
		if utf8.Valid(preview) {
			return preview, true
		}
	}
	return nil, false
}

func cleanRelativePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	clean := filepath.Clean(filepath.FromSlash(value))
	if clean == "." {
		return "", nil
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workspace path is invalid")
	}
	return filepath.ToSlash(clean), nil
}

func cleanFilePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("workspace path is required")
	}
	clean, err := cleanRelativePath(value)
	if err != nil {
		return "", err
	}
	if clean == "" {
		return "", fmt.Errorf("workspace path is required")
	}
	return clean, nil
}

func cleanFSRelativePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	clean := pathpkg.Clean(strings.ReplaceAll(value, "\\", "/"))
	if clean == "." {
		return "", nil
	}
	if strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("workspace path is invalid")
	}
	return clean, nil
}

func cleanFSFilePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("workspace path is required")
	}
	clean, err := cleanFSRelativePath(value)
	if err != nil {
		return "", err
	}
	if clean == "" {
		return "", fmt.Errorf("workspace path is required")
	}
	return clean, nil
}
