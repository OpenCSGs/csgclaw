package system

import (
	"embed"
	"fmt"
	"io/fs"
	pathpkg "path"
	"slices"
	"strings"

	templateembed "csgclaw/internal/template/embed"
)

const (
	skillsRoot                        = "embed"
	interactiveOutputDemoName         = "csgclaw-interactive-output-demo"
	interactiveOutputDemoTemplateRoot = templateembed.CodexManagerRoot + "/" + templateembed.SkillsDirName + "/" + interactiveOutputDemoName
)

//go:embed embed
var defaultSkillsFS embed.FS

type registeredSkillSource struct {
	name string
	fs   fs.FS
	root string
}

var additionalSkillSources = []registeredSkillSource{
	{
		name: interactiveOutputDemoName,
		fs:   templateembed.FS(),
		root: interactiveOutputDemoTemplateRoot,
	},
}

var skillsFS fs.FS = combinedSkillsFS{}

type combinedSkillsFS struct{}

func (combinedSkillsFS) Open(name string) (fs.File, error) {
	source, sourcePath := systemSkillFSPath(name)
	return source.Open(sourcePath)
}

func (combinedSkillsFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == skillsRoot {
		entries, err := fs.ReadDir(defaultSkillsFS, name)
		if err != nil {
			return nil, err
		}
		seen := make(map[string]struct{}, len(entries)+len(additionalSkillSources))
		for _, entry := range entries {
			seen[entry.Name()] = struct{}{}
		}
		for _, source := range additionalSkillSources {
			if _, exists := seen[source.name]; exists {
				return nil, fmt.Errorf("duplicate system skill %q", source.name)
			}
			info, err := fs.Stat(source.fs, source.root)
			if err != nil {
				return nil, err
			}
			entries = append(entries, fs.FileInfoToDirEntry(info))
			seen[source.name] = struct{}{}
		}
		slices.SortFunc(entries, func(left, right fs.DirEntry) int {
			return strings.Compare(left.Name(), right.Name())
		})
		return entries, nil
	}
	source, sourcePath := systemSkillFSPath(name)
	return fs.ReadDir(source, sourcePath)
}

func (combinedSkillsFS) ReadFile(name string) ([]byte, error) {
	source, sourcePath := systemSkillFSPath(name)
	return fs.ReadFile(source, sourcePath)
}

func (combinedSkillsFS) Stat(name string) (fs.FileInfo, error) {
	source, sourcePath := systemSkillFSPath(name)
	return fs.Stat(source, sourcePath)
}

func systemSkillFSPath(name string) (fs.FS, string) {
	for _, source := range additionalSkillSources {
		prefix := pathpkg.Join(skillsRoot, source.name)
		if name == prefix {
			return source.fs, source.root
		}
		if suffix, ok := strings.CutPrefix(name, prefix+"/"); ok {
			return source.fs, pathpkg.Join(source.root, suffix)
		}
	}
	return defaultSkillsFS, name
}
