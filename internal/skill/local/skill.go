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
	"csgclaw/internal/skill/discovery"
	skillinstall "csgclaw/internal/skill/installstate"

	"gopkg.in/yaml.v3"
)

const skillFileName = "SKILL.md"

var ErrSkillInvalid = errors.New("skill directory must contain SKILL.md")

type SkillSummary struct {
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Error       string               `json:"error,omitempty"`
	Source      string               `json:"source,omitempty"`
	Discovery   *discovery.Skill     `json:"discovery,omitempty"`
	Directory   string               `json:"directory,omitempty"`
	Install     *skillinstall.Record `json:"install,omitempty"`
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
	var discovered []discovery.Skill
	if discovery.IsGlobal(root) {
		var err error
		discovered, err = discovery.Sync(root)
		if err != nil {
			return nil, err
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	records, err := skillinstall.Read(root)
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
		metadata, err := readSkillMetadata(skillPath)
		if err != nil {
			return nil, err
		}
		var origin *skillinstall.Record
		if record, ok := records[entry.Name()]; ok {
			if record.SourceKind() == skillinstall.SourceLocal && record.Version == "" {
				record.Version = metadata.Version
			}
			origin = &record
		}
		items = append(items, SkillSummary{
			Install:     origin,
			Name:        entry.Name(),
			Description: metadata.Description,
			Source:      records[entry.Name()].SourceKind(),
			Directory:   absoluteSkillDirectory(filepath.Join(root, entry.Name())),
		})
	}
	for _, found := range discovered {
		index := slices.IndexFunc(items, func(item SkillSummary) bool { return strings.EqualFold(item.Name, found.Name) })
		if index >= 0 {
			current, err := discovery.ReadDefinition(filepath.Join(root, items[index].Name))
			if err != nil || discovery.Compare(current, found) >= 0 {
				continue
			}
			items = slices.Delete(items, index, index+1)
		}
		items = append(items, SkillSummary{Name: found.Name, Description: found.Description, Source: "discovered", Discovery: &found, Directory: found.Path})
	}
	slices.SortFunc(items, func(left, right SkillSummary) int {
		return strings.Compare(left.Name, right.Name)
	})
	return items, nil
}

func absoluteSkillDirectory(dir string) string {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	return abs
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
	if discovery.IsGlobal(root) {
		if found, err := Discovered(root, cleanName); err != nil {
			return err
		} else if found != nil {
			return discovery.Ignore(root, cleanName)
		}
	}
	return skillinstall.Update(root, cleanName, nil, func() error {
		// Never use external source resolution for a destructive operation.
		// A source can appear or change between validation and this transaction.
		skillDir, err := resolveInstalledDir(root, cleanName)
		if err != nil {
			return err
		}
		return os.RemoveAll(skillDir)
	})
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
	if found, err := Discovered(root, cleanName); err != nil {
		return "", err
	} else if found != nil {
		return found.Path, nil
	}
	return resolveInstalledDir(root, cleanName)
}

func resolveInstalledDir(root, cleanName string) (string, error) {
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

type skillMetadata struct {
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
	Metadata    struct {
		Version string `yaml:"version"`
	} `yaml:"metadata"`
}

func readSkillMetadata(path string) (skillMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return skillMetadata{}, fmt.Errorf("read skill file %q: %w", path, err)
	}
	frontmatter, ok := extractFrontmatter(data)
	if !ok {
		return skillMetadata{}, nil
	}
	metadata, err := parseSkillMetadata(frontmatter)
	if err != nil {
		return skillMetadata{}, fmt.Errorf("parse skill frontmatter %q: %w", path, err)
	}
	return metadata, nil
}

func parseSkillDescription(frontmatter []byte) (string, error) {
	meta, err := parseSkillMetadata(frontmatter)
	return meta.Description, err
}

func parseSkillMetadata(frontmatter []byte) (skillMetadata, error) {
	var meta skillMetadata
	if err := yaml.Unmarshal(frontmatter, &meta); err != nil {
		return skillMetadata{}, err
	}
	meta.Description = strings.TrimSpace(meta.Description)
	meta.Version = strings.TrimSpace(meta.Version)
	if meta.Version == "" {
		meta.Version = strings.TrimSpace(meta.Metadata.Version)
	}
	return meta, nil
}

func extractFrontmatter(data []byte) ([]byte, bool) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, false
	}
	rest := data[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		return nil, false
	}
	return rest[:end], true
}

// Discovered resolves the same winning source as List, never an arbitrary path.
func Discovered(root, name string) (*discovery.Skill, error) {
	if !discovery.IsGlobal(root) {
		return nil, nil
	}
	items, err := discovery.Registered(root)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if !strings.EqualFold(item.Name, name) {
			continue
		}
		resolved, err := filepath.EvalSymlinks(item.Path)
		if err != nil || resolved != item.Path {
			continue
		}
		latest, err := discovery.ReadDefinition(item.Path)
		if err != nil || !strings.EqualFold(latest.Name, item.Name) {
			continue
		}
		item.Description = latest.Description
		item.Version = latest.Version
		item.UpdatedAt = latest.UpdatedAt
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			current, err := discovery.ReadDefinition(path)
			if err != nil || discovery.Compare(current, item) >= 0 {
				return nil, nil
			}
		}
		return &item, nil
	}
	return nil, nil
}
