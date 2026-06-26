package system

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const skillFileName = "SKILL.md"

const (
	SkillSourceLocal  = "local"
	SkillSourceSystem = "system"
)

type SkillSummary struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
	Readonly    bool   `json:"readonly,omitempty"`
}

type SkillSource struct {
	Name     string
	Source   string
	FS       fs.FS
	RootPath string
}

func List() ([]SkillSummary, error) {
	return listFS(skillsFS, skillsRoot)
}

func listFS(sourceFS fs.FS, root string) ([]SkillSummary, error) {
	entries, err := fs.ReadDir(sourceFS, root)
	if err != nil {
		return nil, err
	}
	items := make([]SkillSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		skillRoot := pathJoin(root, name)
		if err := statValidSkillFS(sourceFS, skillRoot); err != nil {
			return nil, err
		}
		description, err := skillDescriptionFS(sourceFS, pathJoin(skillRoot, skillFileName))
		if err != nil {
			return nil, err
		}
		items = append(items, SkillSummary{
			Name:        name,
			Description: description,
			Source:      SkillSourceSystem,
			Readonly:    true,
		})
	}
	slices.SortFunc(items, func(left, right SkillSummary) int {
		return strings.Compare(left.Name, right.Name)
	})
	return items, nil
}

func Names() ([]string, error) {
	items, err := List()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	return names, nil
}

func RootSource() SkillSource {
	return SkillSource{
		Source:   SkillSourceSystem,
		FS:       skillsFS,
		RootPath: skillsRoot,
	}
}

func ResolveSource(name string) (SkillSource, error) {
	name, err := NormalizeName(name)
	if err != nil {
		return SkillSource{}, err
	}
	root := pathJoin(skillsRoot, name)
	if err := statValidSkillFS(skillsFS, root); err != nil {
		return SkillSource{}, err
	}
	return SkillSource{
		Name:     name,
		Source:   SkillSourceSystem,
		FS:       skillsFS,
		RootPath: root,
	}, nil
}

func IsName(name string) bool {
	_, err := ResolveSource(name)
	return err == nil
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

func NameFromRelativePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	clean := filepath.Clean(filepath.FromSlash(value))
	if clean == "." {
		return "", nil
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("skill path is invalid")
	}
	parts := strings.Split(clean, string(filepath.Separator))
	if len(parts) == 0 {
		return "", nil
	}
	return NormalizeName(parts[0])
}

func statValidSkillFS(sourceFS fs.FS, root string) error {
	info, err := fs.Stat(sourceFS, root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return os.ErrNotExist
	}
	info, err = fs.Stat(sourceFS, pathJoin(root, skillFileName))
	if err != nil {
		return err
	}
	if info.IsDir() {
		return os.ErrNotExist
	}
	return nil
}

func skillDescriptionFS(sourceFS fs.FS, path string) (string, error) {
	data, err := fs.ReadFile(sourceFS, path)
	if err != nil {
		return "", fmt.Errorf("read skill file %q: %w", path, err)
	}
	return skillDescriptionBytes(data, path)
}

func skillDescriptionBytes(data []byte, label string) (string, error) {
	frontmatter, ok := extractFrontmatter(data)
	if !ok {
		return "", nil
	}
	var meta struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(frontmatter, &meta); err != nil {
		return "", fmt.Errorf("parse skill frontmatter %q: %w", label, err)
	}
	return strings.TrimSpace(meta.Description), nil
}

func pathJoin(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	return strings.Join(cleaned, "/")
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
