package agentworkspace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"csgclaw/internal/apitypes"
)

const filePreviewMaxBytes = 256 * 1024

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
	data, err := io.ReadAll(io.LimitReader(handle, int64(filePreviewMaxBytes)))
	if err != nil {
		return apitypes.WorkspaceFile{}, fmt.Errorf("read workspace file %q: %w", cleanPath, err)
	}
	file := apitypes.WorkspaceFile{
		Path: filepath.ToSlash(cleanPath),
		Size: info.Size(),
	}
	if info.Size() > int64(filePreviewMaxBytes) {
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
