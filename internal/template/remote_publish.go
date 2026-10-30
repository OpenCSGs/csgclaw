package template

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (s *RemoteStore) remoteTemplateEndpoint(name string) string {
	return s.hubBaseURL + "/api/v1/codes/" + url.PathEscape(s.username) + "/" + url.PathEscape(name)
}

func (s *RemoteStore) lookupRemoteTemplate(ctx context.Context, name string) (string, bool, error) {
	data, status, err := s.request(ctx, http.MethodGet, s.remoteTemplateEndpoint(name)+"?"+url.Values{"current_user": {s.username}}.Encode(), s.maxJSON+1)
	if err != nil {
		return "", false, fmt.Errorf("check cloud template: %w", err)
	}
	if status == http.StatusNotFound {
		return "", false, nil
	}
	if status < 200 || status >= 300 {
		return "", false, fmt.Errorf("check cloud template: %w", decodeRemoteAPIError(status, data))
	}
	if int64(len(data)) > s.maxJSON {
		return "", false, fmt.Errorf("cloud template response exceeds %d bytes", s.maxJSON)
	}
	var response remoteCodeResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return "", false, fmt.Errorf("decode cloud template: %w", err)
	}
	return strings.TrimSpace(response.Data.DefaultBranch), true, nil
}

type remoteCommitFile struct {
	Path    string `json:"path"`
	Action  string `json:"action"`
	Content string `json:"content,omitempty"`
}

type remotePreuploadFile struct {
	Path   string `json:"path"`
	Size   int    `json:"size"`
	Sample string `json:"sample"`
}

type remotePreuploadResult struct {
	Path         string `json:"path"`
	OID          string `json:"oid"`
	UploadMode   string `json:"uploadMode"`
	ShouldIgnore bool   `json:"shouldIgnore"`
	IsDir        bool   `json:"isDir"`
}

// Stage exactly the same filtered template layout used by local publication.
// Bound the uncompressed snapshot before making any remote mutations.
func buildRemoteTemplateFiles(spec PublishSpec, maxBytes int64) (map[string][]byte, error) {
	root, err := os.MkdirTemp("", "csgclaw-hub-publish-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(root)
	if err := (&LocalStore{}).writeManifest(filepath.Join(root, localManifestFileName), spec); err != nil {
		return nil, err
	}
	if spec.WorkspaceRef.Kind == WorkspaceKindDir {
		if err := writeTemplateLayout(spec.WorkspaceRef, root, spec.RuntimeKind, spec.MCPServers, spec.IncludeMemory); err != nil {
			return nil, err
		}
	}
	files := make(map[string][]byte)
	var total int64
	err = filepath.WalkDir(root, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported template file %q", name)
		}
		total += info.Size()
		if total > maxBytes {
			return fmt.Errorf("remote hub template exceeds %d bytes", maxBytes)
		}
		content, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = content
		return nil
	})
	return files, err
}

// Enumerate the old snapshot so files removed locally are removed in the same
// commit. Use the hub endpoint for publication reads as well as writes.
func (s *RemoteStore) remotePublicationFiles(ctx context.Context, name, branch, dir string, files map[string]bool) error {
	cursor := ""
	for {
		endpoint := s.remoteTemplateEndpoint(name) + "/refs/" + url.PathEscape(branch) + "/tree/" + escapeRemotePath(dir)
		query := url.Values{"current_user": {s.username}, "limit": {"500"}, "cursor": {cursor}}
		var response remoteTreeResponse
		if err := s.getJSON(ctx, endpoint+"?"+query.Encode(), &response); err != nil {
			return fmt.Errorf("list cloud template files: %w", err)
		}
		for _, entry := range response.Data.Files {
			if err := validateWorkspaceRelativePath(filepath.FromSlash(entry.Path)); err != nil {
				return err
			}
			switch strings.ToLower(entry.Type) {
			case "dir", "tree":
				if entry.Path == dir || (dir != "" && !strings.HasPrefix(entry.Path, dir+"/")) {
					return ErrWorkspacePathUnsafe
				}
				if err := s.remotePublicationFiles(ctx, name, branch, entry.Path, files); err != nil {
					return err
				}
			case "file", "blob":
				files[entry.Path] = true
			default:
				return fmt.Errorf("unsupported cloud template entry type %q", entry.Type)
			}
		}
		next := strings.TrimSpace(response.Data.Cursor)
		if next == "" {
			return nil
		}
		if next == cursor {
			return fmt.Errorf("cloud template tree cursor did not advance")
		}
		cursor = next
	}
}

func (s *RemoteStore) commitRemoteTemplate(ctx context.Context, name, branch string, files map[string][]byte, existing bool) error {
	old := make(map[string]bool)
	if existing {
		if err := s.remotePublicationFiles(ctx, name, branch, "", old); err != nil {
			return err
		}
	}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	request := struct {
		Files []remotePreuploadFile `json:"files"`
	}{}
	for _, p := range paths {
		content := files[p]
		sample := content[:min(len(content), 512)]
		request.Files = append(request.Files, remotePreuploadFile{Path: p, Size: len(content), Sample: base64.StdEncoding.EncodeToString(sample)})
	}
	endpoint := s.remoteTemplateEndpoint(name)
	query := "?" + url.Values{"current_user": {s.username}}.Encode()
	var preupload struct {
		Data struct {
			Files []remotePreuploadResult `json:"files"`
		} `json:"data"`
	}
	if err := s.postJSON(ctx, endpoint+"/preupload/"+url.PathEscape(branch)+query, request, &preupload); err != nil {
		return fmt.Errorf("preupload cloud template: %w", err)
	}
	results := make(map[string]remotePreuploadResult)
	for _, file := range preupload.Data.Files {
		if _, ok := files[file.Path]; !ok {
			return fmt.Errorf("unexpected preupload file %q", file.Path)
		}
		if _, duplicate := results[file.Path]; duplicate {
			return fmt.Errorf("duplicate preupload file %q", file.Path)
		}
		results[file.Path] = file
	}
	commit := struct {
		Files   []remoteCommitFile `json:"files"`
		Message string             `json:"message"`
	}{Message: "Publish template snapshot"}
	for _, p := range paths {
		result, ok := results[p]
		if !ok {
			return fmt.Errorf("missing preupload result for %q", p)
		}
		if result.ShouldIgnore {
			return fmt.Errorf("cloud template file %q cannot be uploaded", p)
		}
		if result.UploadMode != "regular" {
			return fmt.Errorf("unsupported upload mode %q for template file %q", result.UploadMode, p)
		}
		action := "create"
		if result.OID != "" && !result.IsDir {
			action = "update"
		}
		commit.Files = append(commit.Files, remoteCommitFile{Path: p, Action: action, Content: base64.StdEncoding.EncodeToString(files[p])})
		delete(old, p)
	}
	removed := make([]string, 0, len(old))
	for p := range old {
		// Keep repository-level Git policy files created by the community service.
		if p == ".gitattributes" || p == ".gitignore" {
			continue
		}
		removed = append(removed, p)
	}
	sort.Strings(removed)
	deletions := make([]remoteCommitFile, 0, len(removed))
	for _, p := range removed {
		deletions = append(deletions, remoteCommitFile{Path: p, Action: "delete"})
	}
	// Delete ancestors and descendants before creating files at conflicting paths.
	commit.Files = append(deletions, commit.Files...)
	if err := s.postJSON(ctx, endpoint+"/commit/"+url.PathEscape(branch)+query, commit, nil); err != nil {
		return fmt.Errorf("commit cloud template files: %w", err)
	}
	return nil
}
