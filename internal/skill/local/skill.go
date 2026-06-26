package local

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"csgclaw/internal/config"

	"gopkg.in/yaml.v3"
)

const skillFileName = "SKILL.md"

var ErrSkillInvalid = errors.New("skill directory must contain SKILL.md")

type SkillSummary struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func SkillsRoot() (string, error) {
	dir, err := config.DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "skills"), nil
}

func List(root string) ([]SkillSummary, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("skills root is required")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	items := make([]SkillSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(root, entry.Name(), skillFileName)
		info, err := os.Stat(skillPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat skill file %q: %w", skillPath, err)
		}
		if info.IsDir() {
			continue
		}
		description, err := skillDescription(skillPath)
		if err != nil {
			return nil, err
		}
		items = append(items, SkillSummary{
			Name:        entry.Name(),
			Description: description,
		})
	}
	slices.SortFunc(items, func(left, right SkillSummary) int {
		return strings.Compare(left.Name, right.Name)
	})
	return items, nil
}

func Delete(root, name string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return fmt.Errorf("skills root is required")
	}
	cleanName, err := NormalizeName(name)
	if err != nil {
		return err
	}
	skillDir, err := ResolveDir(root, cleanName)
	if err != nil {
		return err
	}
	return os.RemoveAll(skillDir)
}

func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("skill name is required")
	}
	cleanName := filepath.Clean(name)
	if cleanName == "." || cleanName == ".." || cleanName != filepath.Base(cleanName) {
		return "", fmt.Errorf("invalid skill name %q", name)
	}
	return cleanName, nil
}

func ResolveDir(root, name string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("skills root is required")
	}
	cleanName, err := NormalizeName(name)
	if err != nil {
		return "", err
	}
	skillDir := filepath.Join(root, cleanName)
	info, err := os.Stat(skillDir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", ErrSkillInvalid
	}
	skillFile := filepath.Join(skillDir, skillFileName)
	fileInfo, err := os.Lstat(skillFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrSkillInvalid
		}
		return "", err
	}
	if !fileInfo.Mode().IsRegular() || fileInfo.IsDir() || fileInfo.Mode()&fs.ModeSymlink != 0 {
		return "", ErrSkillInvalid
	}
	return skillDir, nil
}

func skillDescription(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read skill file %q: %w", path, err)
	}
	frontmatter, ok := extractFrontmatter(data)
	if !ok {
		return "", nil
	}
	var meta struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(frontmatter, &meta); err != nil {
		return "", fmt.Errorf("parse skill frontmatter %q: %w", path, err)
	}
	return strings.TrimSpace(meta.Description), nil
}

func extractFrontmatter(data []byte) ([]byte, bool) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, false
	}
	rest := data[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		end = bytes.Index(rest, []byte("\n---\r\n"))
	}
	if end < 0 {
		return nil, false
	}
	return rest[:end], true
}
