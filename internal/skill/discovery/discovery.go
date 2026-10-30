// Package discovery registers external skills by reference, without copying or
// modifying their source. Discovery opens only bounded, regular SKILL.md files.
package discovery

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"csgclaw/internal/config"
	"csgclaw/internal/skill/installstate"
	"csgclaw/internal/utils/filebrowse"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

type Source struct {
	Tool      string    `json:"tool"`
	Path      string    `json:"path"`
	Version   string    `json:"version,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source
	Sources []Source `json:"sources"`
	Key     string   `json:"key"`
	New     bool     `json:"new,omitempty"`
}

type catalog struct {
	Skills  []Skill         `json:"skills"`
	Ignored map[string]bool `json:"ignored,omitempty"`
	Seen    map[string]bool `json:"seen,omitempty"`
}

const catalogFile = ".discovered-skills.json"
const maxDefinitionSize = 1 << 20

// IsGlobal prevents external discovery from being applied to Agent workspaces.
func IsGlobal(root string) bool {
	dir, err := config.DefaultDir()
	return err == nil && filepath.Clean(root) == filepath.Join(dir, "skills")
}

// Roots is deliberately finite. Never recurse through a home directory or read
// another application's config to discover more paths.
func Roots(home string) []Source {
	paths := []struct{ tool, path string }{
		{"Agent", ".agent/skills"}, {"Agents", ".agents/skills"},
		{"Claude Code", ".claude/skills"}, {"Codex", ".codex/skills"},
		{"Pi", ".pi/agent/skills"}, {"OpenCode", ".config/opencode/skills"},
		{"WorkBuddy", ".workbuddy/skills"}, {"Cursor", ".cursor/skills"}, {"CodeBuddy", ".codebuddy/skills"},
	}
	out := make([]Source, 0, len(paths))
	for _, p := range paths {
		out = append(out, Source{Tool: p.tool, Path: filepath.Join(home, filepath.FromSlash(p.path))})
	}
	// Respect documented directory overrides without opening config files.
	for _, p := range []struct{ tool, env, suffix string }{
		{"Claude Code", "CLAUDE_CONFIG_DIR", "skills"}, {"Codex", "CODEX_HOME", "skills"},
		{"Pi", "PI_CODING_AGENT_DIR", "skills"}, {"OpenCode", "XDG_CONFIG_HOME", "opencode/skills"},
	} {
		if dir := os.Getenv(p.env); filepath.IsAbs(dir) {
			out = append(out, Source{Tool: p.tool, Path: filepath.Join(dir, filepath.FromSlash(p.suffix))})
		}
	}
	return out
}

// Scan traverses only skill roots. Once a skill is found, its assets, scripts,
// and references are not inspected. Directory links are allowed at the root's
// first level (common skill installers use them), but never followed recursively.
func Scan(roots []Source) []Skill {
	byName := map[string][]Skill{}
	visited := map[string]bool{}
	budget := 20000
	var walk func(Source, string, int)
	walk = func(source Source, dir string, depth int) {
		if depth > 6 || budget <= 0 {
			return
		}
		budget--
		canonical, err := filepath.EvalSymlinks(dir)
		if err != nil || visited[source.Tool+"\x00"+canonical] {
			return
		}
		visited[source.Tool+"\x00"+canonical] = true
		if item, err := ReadDefinition(canonical); err == nil {
			item.Tool = source.Tool
			item.Path = canonical
			byName[strings.ToLower(item.Name)] = append(byName[strings.ToLower(item.Name)], item)
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			return
		}
		entries, err := os.ReadDir(canonical)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" {
				continue
			}
			if depth == 0 && entry.Type()&os.ModeSymlink != 0 {
				linked := filepath.Join(canonical, entry.Name())
				if _, err := ReadDefinition(linked); err == nil {
					walk(source, linked, depth+1)
				}
			} else if entry.IsDir() {
				walk(source, filepath.Join(canonical, entry.Name()), depth+1)
			}
		}
	}
	for _, root := range roots {
		walk(root, root.Path, 0)
	}
	out := make([]Skill, 0, len(byName))
	for _, candidates := range byName {
		slices.SortFunc(candidates, func(a, b Skill) int { return -Compare(a, b) })
		winner := candidates[0]
		seen := map[string]bool{}
		for _, candidate := range candidates {
			key := candidate.Tool + "\x00" + candidate.Path
			if !seen[key] {
				winner.Sources = append(winner.Sources, candidate.Source)
				seen[key] = true
			}
		}
		digest := sha256.Sum256([]byte(winner.Path + "\x00" + winner.Key))
		winner.Key = hex.EncodeToString(digest[:])
		out = append(out, winner)
	}
	slices.SortFunc(out, func(a, b Skill) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func ReadDefinition(dir string) (Skill, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Skill{}, err
	}
	defer root.Close()
	info, err := root.Lstat("SKILL.md")
	if err != nil {
		return Skill{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxDefinitionSize {
		return Skill{}, fmt.Errorf("invalid skill definition")
	}
	file, err := filebrowse.OpenFile(dir, "SKILL.md")
	if err != nil {
		return Skill{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return Skill{}, fmt.Errorf("skill definition changed")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxDefinitionSize+1))
	if err != nil || len(data) > maxDefinitionSize {
		return Skill{}, fmt.Errorf("cannot read skill definition")
	}
	data = bytes.ReplaceAll(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}), []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return Skill{}, fmt.Errorf("missing skill metadata")
	}
	_, rest, _ := bytes.Cut(data, []byte("---\n"))
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return Skill{}, fmt.Errorf("missing skill metadata")
	}
	var meta struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		Version     string `yaml:"version"`
		Metadata    struct {
			Version string `yaml:"version"`
		} `yaml:"metadata"`
	}
	if err := yaml.Unmarshal(rest[:end], &meta); err != nil {
		return Skill{}, err
	}
	name := strings.TrimSpace(meta.Name)
	if name == "" {
		name = filepath.Base(dir)
	}
	if name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") || strings.HasPrefix(name, ".") {
		return Skill{}, fmt.Errorf("invalid skill name")
	}
	if meta.Version == "" {
		meta.Version = meta.Metadata.Version
	}
	digest := sha256.Sum256(data)
	return Skill{Name: name, Description: strings.TrimSpace(meta.Description), Source: Source{Path: dir, Version: meta.Version, UpdatedAt: info.ModTime()}, Key: hex.EncodeToString(digest[:])}, nil
}

// Compare prefers versioned candidates, then semantic version order. Equal or
// unavailable versions use definition mtime; paths break ties deterministically.
func Compare(a, b Skill) int {
	av, bv := "v"+strings.TrimPrefix(a.Version, "v"), "v"+strings.TrimPrefix(b.Version, "v")
	if semver.IsValid(av) != semver.IsValid(bv) {
		if semver.IsValid(av) {
			return 1
		}
		return -1
	}
	if semver.IsValid(av) && semver.IsValid(bv) {
		if cmp := semver.Compare(av, bv); cmp != 0 {
			return cmp
		}
	}
	if cmp := a.UpdatedAt.Compare(b.UpdatedAt); cmp != 0 {
		return cmp
	}
	return -strings.Compare(a.Path, b.Path)
}

// Registered is an atomic, read-only catalog snapshot. Source resolution may
// run inside an installation transaction, so it must not take that lock again.
func Registered(root string) ([]Skill, error) {
	data, err := os.ReadFile(filepath.Join(root, catalogFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state catalog
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return state.Skills, nil
}

func transact(root string, change func(*catalog) error) error {
	return installstate.ReadSnapshot(root, func(_ map[string]installstate.Record) error {
		path := filepath.Join(root, catalogFile)
		before, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		state := catalog{}
		if len(before) > 0 {
			if err := json.Unmarshal(before, &state); err != nil {
				return err
			}
		}
		if state.Ignored == nil {
			state.Ignored = map[string]bool{}
		}
		if state.Seen == nil {
			state.Seen = map[string]bool{}
		}
		if err := change(&state); err != nil {
			return err
		}
		after, err := json.Marshal(state)
		if err != nil || bytes.Equal(before, after) {
			return err
		}
		file, err := os.CreateTemp(root, ".discovery-")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())
		if _, err = file.Write(after); err != nil {
			file.Close()
			return err
		}
		if err = file.Sync(); err != nil {
			file.Close()
			return err
		}
		if err = file.Close(); err != nil {
			return err
		}
		return os.Rename(file.Name(), path)
	})
}

// Sync atomically registers live references and retains acknowledgements across
// restarts. No external source file is ever written, removed, or copied.
func Sync(root string) ([]Skill, error) {
	return syncCatalog(root, false)
}

// Rescan explicitly rediscovers removed skills. Background scans continue to
// respect removals so a dismissed skill does not immediately reappear.
func Rescan(root string) ([]Skill, error) {
	return syncCatalog(root, true)
}

func syncCatalog(root string, restoreRemoved bool) ([]Skill, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	scanned := Scan(Roots(home))
	var items []Skill
	err = transact(root, func(state *catalog) error {
		ignored := state.Ignored
		if restoreRemoved {
			state.Ignored = map[string]bool{}
		}
		for _, item := range scanned {
			if restoreRemoved && ignored[strings.ToLower(item.Name)] {
				delete(state.Seen, item.Key)
			}
			if !state.Ignored[strings.ToLower(item.Name)] {
				item.New = !state.Seen[item.Key]
				items = append(items, item)
			}
		}
		state.Skills = items
		return nil
	})
	return items, err
}

func Acknowledge(root string, keys []string) error {
	return transact(root, func(state *catalog) error {
		for i := range state.Skills {
			if slices.Contains(keys, state.Skills[i].Key) {
				state.Seen[state.Skills[i].Key] = true
				state.Skills[i].New = false
			}
		}
		return nil
	})
}

func Ignore(root, name string) error {
	return transact(root, func(state *catalog) error {
		state.Ignored[strings.ToLower(name)] = true
		state.Skills = slices.DeleteFunc(state.Skills, func(s Skill) bool { return strings.EqualFold(s.Name, name) })
		return nil
	})
}

// BoundedFS is used only after explicit browsing or assignment to an Agent.
// It cannot follow a skill's internal links outside that skill directory.
func BoundedFS(dir string) fs.FS { return boundedFS(dir) }

type boundedFS string

func (dir boundedFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	return filebrowse.OpenEntry(string(dir), name)
}
