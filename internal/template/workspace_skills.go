package template

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"

	"csgclaw/internal/apitypes"
	"golang.org/x/sync/errgroup"
)

const workspaceSkillScanConcurrency = 8

// WorkspaceSkillNames returns the skill directories verified before ctx expires.
// Remote listings in a scan share one default branch lookup.
func (s *Service) WorkspaceSkillNames(ctx context.Context, id string) []string {
	store, templateID, err := s.resolveRead(id)
	if err != nil {
		return nil
	}
	list := func(p string) (apitypes.WorkspaceListing, error) {
		return s.ListWorkspace(ctx, id, p)
	}
	if remote, ok := store.store.(*RemoteStore); ok {
		remoteID, err := normalizeRemoteTemplateID(templateID)
		if err != nil {
			return nil
		}
		branch, err := remote.defaultBranch(ctx, remoteID)
		if err != nil {
			return nil
		}
		list = func(p string) (apitypes.WorkspaceListing, error) {
			return remote.listWorkspaceAtBranch(ctx, remoteID, branch, p)
		}
	}

	root := "skills"
	listing, err := list(root)
	if errors.Is(err, ErrTemplateNotFound) || errors.Is(err, fs.ErrNotExist) {
		root = "workspace/skills"
		listing, err = list(root)
	}
	if err != nil {
		return nil
	}
	names := make([]string, len(listing.Entries))
	var scan errgroup.Group
	scan.SetLimit(workspaceSkillScanConcurrency)
	for i, entry := range listing.Entries {
		if ctx.Err() != nil {
			break
		}
		name := entry.Name
		if entry.Type != "dir" || name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
			continue
		}
		scan.Go(func() error {
			if ctx.Err() != nil {
				return nil
			}
			files, err := list(path.Join(root, name))
			if err != nil {
				return nil
			}
			for _, file := range files.Entries {
				if file.Type == "file" && file.Name == "SKILL.md" {
					names[i] = name
					break
				}
			}
			return nil
		})
	}
	_ = scan.Wait()
	names = slices.DeleteFunc(names, func(name string) bool { return name == "" })
	slices.Sort(names)
	return names
}
