package remote

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"

	"csgclaw/internal/apitypes"
)

// ListAgenticHubSkillFiles lists remote files without installing the skill.
func ListAgenticHubSkillFiles(ctx context.Context, baseURL, token, remotePath, ref string) (apitypes.WorkspaceListing, error) {
	ctx, cancel := context.WithTimeout(ctx, agenticHubRequestTimeout)
	defer cancel()
	b, err := skillBrowser(baseURL, token, remotePath, ref)
	if err != nil {
		return apitypes.WorkspaceListing{}, err
	}
	listing := apitypes.WorkspaceListing{Kind: "dir"}
	var walk func(string) error
	walk = func(dir string) error {
		return b.walkTree(ctx, dir, func(entry agenticHubTreeEntry) error {
			filePath, err := cleanAgenticHubPath(entry.Path, false)
			if err != nil {
				return err
			}
			parent := path.Dir(filePath)
			if parent == "." {
				parent = ""
			}
			if parent != dir {
				return fmt.Errorf("remote skill tree entry outside directory %q", dir)
			}
			if _, seen := b.visited[filePath]; seen {
				return nil
			}
			b.visited[filePath] = struct{}{}
			if len(b.visited) > agenticHubMaxArchiveFiles {
				return fmt.Errorf("remote skill tree exceeds %d entries", agenticHubMaxArchiveFiles)
			}
			item := apitypes.WorkspaceEntry{
				Path: filePath, Name: path.Base(filePath), Type: "file",
				Depth: strings.Count(filePath, "/"), Size: entry.Size,
			}
			if isAgenticHubDir(entry.Type) {
				item.Type = "dir"
			}
			listing.Entries = append(listing.Entries, item)
			if item.Type == "dir" {
				return walk(filePath)
			}
			return nil
		})
	}
	if err := walk(""); err != nil {
		return apitypes.WorkspaceListing{}, err
	}
	return listing, nil
}

// ReadAgenticHubSkillFile uses the same authenticated blob API as installation.
func ReadAgenticHubSkillFile(ctx context.Context, baseURL, token, remotePath, ref, filePath string) (apitypes.WorkspaceFile, error) {
	b, err := skillBrowser(baseURL, token, remotePath, ref)
	if err != nil {
		return apitypes.WorkspaceFile{}, err
	}
	filePath, err = cleanAgenticHubPath(filePath, false)
	if err != nil {
		return apitypes.WorkspaceFile{}, err
	}
	content, err := b.fetchBlob(ctx, filePath)
	if err != nil {
		return apitypes.WorkspaceFile{}, err
	}
	file := apitypes.WorkspaceFile{Path: filePath, Size: int64(len(content)), Binary: !utf8.Valid(content)}
	if !file.Binary {
		file.Content = string(content)
	}
	return file, nil
}

func skillBrowser(baseURL, token, remotePath, ref string) (*agenticHubArchiveBuilder, error) {
	remotePath, ref, err := NormalizeAgenticHubSkillRequest(remotePath, ref)
	if err != nil {
		return nil, err
	}
	return &agenticHubArchiveBuilder{
		baseURL: baseURL, accessToken: token, remotePath: remotePath, ref: ref,
		client: &http.Client{Timeout: agenticHubRequestTimeout}, visited: map[string]struct{}{},
	}, nil
}
