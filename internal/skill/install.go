package skill

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/skill/installstate"
)

func extractSkillZip(archive []byte, dstDir string, maxBytes int64) (string, error) {
	if int64(len(archive)) > maxBytes {
		return "", fmt.Errorf("skill archive exceeds %d bytes", maxBytes)
	}
	dstDir = strings.TrimSpace(dstDir)
	if dstDir == "" {
		return "", fmt.Errorf("skill destination directory is required")
	}
	dstDir = filepath.Clean(dstDir)

	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return "", fmt.Errorf("open skill zip: %w", err)
	}
	if len(reader.File) == 0 {
		return "", ErrSkillArchiveEmpty
	}

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", fmt.Errorf("create skill dir: %w", err)
	}

	hasSKILLMD := false
	fileCount := 0
	for _, file := range reader.File {
		if file == nil {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: %s", ErrWorkspacePathUnsafe, file.Name)
		}

		rel := filepath.Clean(filepath.FromSlash(strings.TrimSpace(file.Name)))
		if rel == "." || rel == string(filepath.Separator) {
			continue
		}
		if err := validateArchiveRelativePath(rel); err != nil {
			return "", err
		}
		target := filepath.Join(dstDir, rel)
		if err := ensurePathInsideRoot(dstDir, target); err != nil {
			return "", err
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", fmt.Errorf("create skill dir %q: %w", target, err)
			}
			continue
		}
		if !file.Mode().IsRegular() {
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", fmt.Errorf("create skill parent %q: %w", filepath.Dir(target), err)
		}
		if err := writeZipFile(file, target); err != nil {
			return "", err
		}
		fileCount++
		if strings.EqualFold(filepath.Base(rel), "SKILL.md") {
			hasSKILLMD = true
		}
	}
	if fileCount == 0 {
		return "", ErrSkillArchiveEmpty
	}
	if !hasSKILLMD {
		return "", ErrSKILLMDMissing
	}
	return installstate.BundleSHA256(dstDir)
}

func writeZipFile(file *zip.File, target string) error {
	rc, err := file.Open()
	if err != nil {
		return fmt.Errorf("open skill zip entry %q: %w", file.Name, err)
	}
	defer rc.Close()

	mode := file.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	mode |= 0o200
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create skill file %q: %w", target, err)
	}
	if _, err := io.Copy(out, rc); err != nil {
		_ = out.Close()
		return fmt.Errorf("write skill file %q: %w", target, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close skill file %q: %w", target, err)
	}
	return nil
}

func validateArchiveRelativePath(rel string) error {
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return ErrWorkspacePathUnsafe
	}
	if filepath.IsAbs(rel) {
		return ErrWorkspacePathUnsafe
	}
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts {
		if part == ".." {
			return ErrWorkspacePathUnsafe
		}
	}
	return nil
}

func ensurePathInsideRoot(root, target string) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrWorkspacePathUnsafe, target)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %s", ErrWorkspacePathUnsafe, target)
	}
	return nil
}
