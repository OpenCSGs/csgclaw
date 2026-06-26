package local

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	skillsystem "csgclaw/internal/skill/system"
)

const maxArchiveBytes int64 = 64 << 20

var (
	ErrSkillArchiveEmpty   = errors.New("skill archive is empty")
	ErrSkillArchiveUnsafe  = errors.New("skill archive path is unsafe")
	ErrSkillAlreadyExists  = errors.New("skill already exists")
	ErrSkillArchiveInvalid = errors.New("skill archive must contain exactly one skill")
	ErrSKILLMDMissing      = errors.New("skill archive must contain SKILL.md")
)

func InstallArchive(root, filename string, archive []byte) (SkillSummary, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return SkillSummary{}, fmt.Errorf("skills root is required")
	}
	root = filepath.Clean(root)
	if int64(len(archive)) > maxArchiveBytes {
		return SkillSummary{}, fmt.Errorf("skill archive exceeds %d bytes", maxArchiveBytes)
	}
	if len(archive) == 0 {
		return SkillSummary{}, ErrSkillArchiveEmpty
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return SkillSummary{}, fmt.Errorf("create skills root %q: %w", root, err)
	}

	tempDir, err := os.MkdirTemp("", "csgclaw-skill-upload-*")
	if err != nil {
		return SkillSummary{}, fmt.Errorf("create upload temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	if err := extractArchive(archive, tempDir); err != nil {
		return SkillSummary{}, err
	}

	skillDir, skillName, err := resolveExtractedSkillDir(tempDir, filename)
	if err != nil {
		return SkillSummary{}, err
	}
	summary, err := summarizeSkillDir(skillDir, skillName)
	if err != nil {
		return SkillSummary{}, err
	}
	if skillsystem.IsName(summary.Name) {
		return SkillSummary{}, fmt.Errorf("%w: %s", ErrSkillAlreadyExists, summary.Name)
	}

	destDir := filepath.Join(root, summary.Name)
	if err := ensurePathInsideRoot(root, destDir); err != nil {
		return SkillSummary{}, err
	}
	if _, err := os.Stat(destDir); err == nil {
		return SkillSummary{}, fmt.Errorf("%w: %s", ErrSkillAlreadyExists, summary.Name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return SkillSummary{}, fmt.Errorf("stat skill destination %q: %w", destDir, err)
	}

	if err := copyDir(skillDir, destDir); err != nil {
		_ = os.RemoveAll(destDir)
		return SkillSummary{}, err
	}
	return summary, nil
}

func extractArchive(archive []byte, dstDir string) error {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return fmt.Errorf("open skill zip: %w", err)
	}
	if len(reader.File) == 0 {
		return ErrSkillArchiveEmpty
	}
	fileCount := 0
	for _, file := range reader.File {
		if file == nil {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrSkillArchiveUnsafe, file.Name)
		}
		rel := filepath.Clean(filepath.FromSlash(strings.TrimSpace(file.Name)))
		if rel == "." || rel == string(filepath.Separator) {
			continue
		}
		if err := validateArchiveRelativePath(rel); err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if err := ensurePathInsideRoot(dstDir, target); err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create skill dir %q: %w", target, err)
			}
			continue
		}
		if !file.Mode().IsRegular() {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create skill parent %q: %w", filepath.Dir(target), err)
		}
		if err := writeZipFile(file, target); err != nil {
			return err
		}
		fileCount++
	}
	if fileCount == 0 {
		return ErrSkillArchiveEmpty
	}
	return nil
}

func resolveExtractedSkillDir(tempDir, filename string) (string, string, error) {
	rootSkillFile := filepath.Join(tempDir, skillFileName)
	if info, err := os.Stat(rootSkillFile); err == nil && !info.IsDir() {
		name := strings.TrimSuffix(filepath.Base(strings.TrimSpace(filename)), filepath.Ext(strings.TrimSpace(filename)))
		name = strings.TrimSpace(name)
		if name == "" || name == "." {
			return "", "", ErrSkillArchiveInvalid
		}
		return tempDir, name, nil
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return "", "", fmt.Errorf("read extracted skill archive: %w", err)
	}
	dirs := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" || name == "__MACOSX" || strings.HasPrefix(name, ".") {
			continue
		}
		if !entry.IsDir() {
			return "", "", ErrSkillArchiveInvalid
		}
		dirs = append(dirs, entry)
	}
	if len(dirs) != 1 {
		return "", "", ErrSkillArchiveInvalid
	}
	skillDir := filepath.Join(tempDir, dirs[0].Name())
	skillFile := filepath.Join(skillDir, skillFileName)
	info, err := os.Stat(skillFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", ErrSKILLMDMissing
		}
		return "", "", fmt.Errorf("stat skill file %q: %w", skillFile, err)
	}
	if info.IsDir() {
		return "", "", ErrSKILLMDMissing
	}
	return skillDir, dirs[0].Name(), nil
}

func summarizeSkillDir(skillDir, skillName string) (SkillSummary, error) {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" || skillName == "." {
		return SkillSummary{}, ErrSkillArchiveInvalid
	}
	skillFile := filepath.Join(skillDir, skillFileName)
	info, err := os.Stat(skillFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SkillSummary{}, ErrSKILLMDMissing
		}
		return SkillSummary{}, fmt.Errorf("stat skill file %q: %w", skillFile, err)
	}
	if info.IsDir() {
		return SkillSummary{}, ErrSKILLMDMissing
	}
	description, err := skillDescription(skillFile)
	if err != nil {
		return SkillSummary{}, err
	}
	return SkillSummary{
		Name:        skillName,
		Description: description,
	}, nil
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
		return ErrSkillArchiveUnsafe
	}
	if filepath.IsAbs(rel) {
		return ErrSkillArchiveUnsafe
	}
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts {
		if part == ".." {
			return ErrSkillArchiveUnsafe
		}
	}
	return nil
}

func ensurePathInsideRoot(root, target string) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrSkillArchiveUnsafe, target)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %s", ErrSkillArchiveUnsafe, target)
	}
	return nil
}

func copyDir(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("create skill destination %q: %w", dstDir, err)
	}
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == srcDir {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("rel skill path %q: %w", path, err)
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat skill entry %q: %w", path, err)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open skill file %q: %w", src, err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create skill parent %q: %w", filepath.Dir(dst), err)
	}
	if perm == 0 {
		perm = 0o644
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("create skill file %q: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy skill file %q: %w", dst, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close skill file %q: %w", dst, err)
	}
	return nil
}
