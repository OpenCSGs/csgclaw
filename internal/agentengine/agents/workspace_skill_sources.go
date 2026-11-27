package agents

import (
	"bytes"
	"cmp"
	"context"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"csgclaw/internal/agentengine/contract"
	skillinstall "csgclaw/internal/skill/installstate"
	skilllocal "csgclaw/internal/skill/local"
	skill "csgclaw/internal/skill/state"
	skillsystem "csgclaw/internal/skill/system"
	templateembed "csgclaw/internal/template/embed"
)

func workspaceSkillSummaries(ctx context.Context, root string, agent Agent, items []skilllocal.SkillSummary) ([]contract.SkillSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	templateRoot, _ := resolveRuntimeTemplateRoot(agent.RuntimeKind, recreateTemplateRole(agent))
	out := make([]contract.SkillSummary, 0, len(items))
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id := cmp.Or(item.ID, item.Name)
		if item.Source == skillinstall.SourceUnknown && id == item.Name {
			candidates := make([]skillsystem.SkillSource, 0, 2)
			if source, err := skillsystem.ResolveSource(item.Name); err == nil {
				candidates = append(candidates, source)
			}
			if templateRoot != "" {
				candidates = append(candidates, skillsystem.SkillSource{
					FS:       templateembed.FS(),
					RootPath: path.Join(runtimeTemplateWorkspacePath(templateRoot), templateembed.SkillsDirName, item.Name),
				})
			}
			for _, source := range candidates {
				matches, err := matchesBuiltinSkill(ctx, filepath.Join(root, item.Name), source)
				if err != nil {
					return nil, err
				}
				if matches {
					item.Source = skillinstall.SourceBuiltin
					break
				}
			}
		}
		if item.Source == skillinstall.SourceBuiltin {
			item.Version = ""
		}
		out = append(out, contract.SkillSummary{ID: id, Enabled: skill.Enabled(agent.SkillStates, id), Name: item.Name, Version: item.Version, Description: item.Description, Source: item.Source, Error: item.Error})
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Older template copies have no installation record. Recognize only a complete,
// unchanged embedded bundle without writing provenance during a read request.
func matchesBuiltinSkill(ctx context.Context, directory string, source skillsystem.SkillSource) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	embedded, err := fs.Sub(source.FS, source.RootPath)
	if err != nil {
		return false, nil
	}
	expected := make(map[string]fs.FileInfo)
	err = fs.WalkDir(embedded, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		expected[name] = info
		return nil
	})
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if err != nil || expected["SKILL.md"] == nil {
		return false, nil
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() {
		return false, nil
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return false, nil
	}
	defer root.Close()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		return false, nil
	}
	matched := true
	count := 0
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		want, exists := expected[name]
		if !exists || entry.Type()&os.ModeSymlink != 0 || entry.IsDir() != want.IsDir() {
			matched = false
			return fs.SkipAll
		}
		count++
		if entry.IsDir() {
			return nil
		}
		actual, err := entry.Info()
		if err != nil {
			return err
		}
		if !actual.Mode().IsRegular() || !want.Mode().IsRegular() || actual.Size() != want.Size() {
			matched = false
			return fs.SkipAll
		}
		file, err := root.Open(name)
		if err != nil {
			return err
		}
		opened, err := file.Stat()
		if err != nil || !opened.Mode().IsRegular() || !os.SameFile(actual, opened) {
			_ = file.Close()
			matched = false
			return fs.SkipAll
		}
		data, err := io.ReadAll(io.LimitReader(file, want.Size()+1))
		_ = file.Close()
		if err != nil {
			return err
		}
		wantData, err := fs.ReadFile(embedded, name)
		if err != nil {
			return err
		}
		if !bytes.Equal(data, wantData) {
			matched = false
			return fs.SkipAll
		}
		return nil
	})
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	return err == nil && matched && count == len(expected), nil
}
