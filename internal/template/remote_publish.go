package template

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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

const (
	remotePublishMaxFileBytes  = 50 * 1024 * 1024
	remotePublishMaxTotalBytes = 100 * 1024 * 1024
	remotePublishMaxBatchFiles = 200
	remotePublishMaxBatchBytes = 10 * 1024 * 1024
)

// PublishFileTooLargeError identifies a file exceeding the community publication limit.
type PublishFileTooLargeError struct {
	Path     string
	MaxBytes int64
}

func (e *PublishFileTooLargeError) Error() string {
	return fmt.Sprintf("template file %q exceeds the size limit of %d MiB", e.Path, e.MaxBytes/(1024*1024))
}

// PublishTotalTooLargeError identifies a snapshot exceeding the community publication limit.
type PublishTotalTooLargeError struct {
	MaxBytes int64
}

func (e *PublishTotalTooLargeError) Error() string {
	return fmt.Sprintf("template exceeds the total size limit of %d MiB", e.MaxBytes/(1024*1024))
}

// Stage the filtered template layout and validate its size before contacting the hub.
func buildRemoteTemplateFiles(spec PublishSpec, maxFileBytes int64) (map[string][]byte, error) {
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
	var totalBytes int64
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
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		path := filepath.ToSlash(rel)
		if info.Size() > maxFileBytes {
			return &PublishFileTooLargeError{Path: path, MaxBytes: maxFileBytes}
		}
		totalBytes += info.Size()
		if totalBytes > remotePublishMaxTotalBytes {
			return &PublishTotalTooLargeError{MaxBytes: remotePublishMaxTotalBytes}
		}
		content, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		files[path] = content
		return nil
	})
	return files, err
}

// Enumerate the old snapshot so stale files can be removed during publication.
// Use the hub endpoint for publication reads as well as writes.
func (s *RemoteStore) remotePublicationFiles(ctx context.Context, name, branch, dir string, files map[string]string) error {
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
				files[entry.Path] = entry.SHA
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

// publicationBatches groups ordered paths by raw content bytes and operation count.
// A file above the byte target occupies a batch by itself.
func publicationBatches(paths []string, files map[string][]byte) [][]string {
	var batches [][]string
	for len(paths) > 0 {
		count, size := 1, len(files[paths[0]])
		for count < min(len(paths), remotePublishMaxBatchFiles) {
			next := len(files[paths[count]])
			if size+next > remotePublishMaxBatchBytes {
				break
			}
			count++
			size += next
		}
		batches = append(batches, paths[:count])
		paths = paths[count:]
	}
	return batches
}

func sameTemplateBlob(oid string, content []byte) bool {
	var digest hash.Hash
	switch len(oid) {
	case sha1.Size * 2:
		digest = sha1.New()
	case sha256.Size * 2:
		digest = sha256.New()
	default:
		return false
	}
	fmt.Fprintf(digest, "blob %d\x00", len(content))
	_, _ = digest.Write(content)
	return hex.EncodeToString(digest.Sum(nil)) == oid
}

func (s *RemoteStore) remotePublicationHead(ctx context.Context, name, branch string) (string, error) {
	query := url.Values{"current_user": {s.username}, "ref": {branch}}
	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := s.getJSON(ctx, s.remoteTemplateEndpoint(name)+"/last_commit?"+query.Encode(), &result); err != nil {
		return "", fmt.Errorf("read cloud template head: %w", err)
	}
	if result.Data.ID == "" {
		return "", fmt.Errorf("cloud template branch %q has no commit ID", branch)
	}
	return result.Data.ID, nil
}

func (s *RemoteStore) commitRemoteTemplate(ctx context.Context, name, branch string, files map[string][]byte, existing bool, nickname string) error {
	const maxAttempts = 3
	for attempt := 1; ; attempt++ {
		err := s.commitRemoteTemplateAttempt(ctx, name, branch, files, existing || attempt > 1, existing, nickname)
		if err == nil || attempt == maxAttempts || ctx.Err() != nil || !retryableTemplateCommit(err) {
			return err
		}
		// Let a timed-out request settle before reading the current repository state.
		timer := time.NewTimer(time.Duration(attempt) * 500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func retryableTemplateCommit(err error) bool {
	if errors.Is(err, ErrTemplateSensitiveInfo) {
		return false
	}
	var remote *RemoteAPIError
	if errors.As(err, &remote) {
		switch remote.StatusCode {
		case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
		return false
	}
	var network net.Error
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		(errors.As(err, &network) && network.Timeout())
}

func (s *RemoteStore) commitRemoteTemplateAttempt(ctx context.Context, name, branch string, files map[string][]byte, inspect, prune bool, nickname string) error {
	old := make(map[string]string)
	var base string
	var err error
	if inspect {
		base, err = s.remotePublicationHead(ctx, name, branch)
		if err != nil {
			return err
		}
		if err := s.remotePublicationFiles(ctx, name, base, "", old); err != nil {
			return err
		}
	}
	paths := make([]string, 0, len(files))
	manifestChanged := false
	for p, content := range files {
		if p == localManifestFileName {
			manifestChanged = !sameTemplateBlob(old[p], content)
		} else if !sameTemplateBlob(old[p], content) {
			paths = append(paths, p)
		}
		delete(old, p)
	}
	sort.Strings(paths)
	removed := make([]string, 0, len(old))
	for p := range old {
		// Keep repository-level Git policy files created by the community service.
		if prune && p != ".gitattributes" && p != ".gitignore" {
			removed = append(removed, p)
		}
	}
	sort.Strings(removed)

	endpoint := s.remoteTemplateEndpoint(name)
	query := "?" + url.Values{"current_user": {s.username}}.Encode()
	actions := make(map[string]string, len(paths))
	preuploadRef := branch
	if base != "" {
		preuploadRef = base
	}
	preuploadPaths := paths
	if manifestChanged {
		preuploadPaths = append(preuploadPaths, localManifestFileName)
	}
	for _, batch := range publicationBatches(preuploadPaths, files) {
		if err := s.preuploadTemplateBatch(ctx, endpoint+"/preupload/"+url.PathEscape(preuploadRef)+query, batch, files, actions); err != nil {
			return fmt.Errorf("preupload cloud template: %w", err)
		}
	}

	// Delete ancestors and descendants before creating files at conflicting paths.
	batches := publicationBatches(append(removed, paths...), files)
	if manifestChanged {
		// Publish the version marker only after the rest of the snapshot is ready.
		batches = append(batches, []string{localManifestFileName})
	}
	for index, batch := range batches {
		if prune && len(batch) == 1 && batch[0] == localManifestFileName {
			if err := s.putJSON(ctx, endpoint, remoteUpdateCodeRequest{Nickname: nickname}, nil); err != nil {
				return fmt.Errorf("update cloud template metadata: %w", err)
			}
		}
		commit := struct {
			Files   []remoteCommitFile `json:"files"`
			Message string             `json:"message"`
		}{Files: make([]remoteCommitFile, 0, len(batch)), Message: "Publish template snapshot"}
		for _, p := range batch {
			action, ok := actions[p]
			if !ok {
				action = "delete"
			}
			commit.Files = append(commit.Files, remoteCommitFile{
				Path: p, Action: action, Content: base64.StdEncoding.EncodeToString(files[p]),
			})
		}
		if err := s.postJSON(ctx, endpoint+"/commit/"+url.PathEscape(branch)+query, commit, nil); err != nil {
			return fmt.Errorf("commit cloud template files (batch %d of %d): %w", index+1, len(batches), err)
		}
	}

	return nil
}

func (s *RemoteStore) preuploadTemplateBatch(ctx context.Context, endpoint string, paths []string, files map[string][]byte, actions map[string]string) error {
	request := struct {
		Files []remotePreuploadFile `json:"files"`
	}{Files: make([]remotePreuploadFile, 0, len(paths))}
	expected := make(map[string]bool, len(paths))
	for _, p := range paths {
		content := files[p]
		sample := content[:min(len(content), 512)]
		request.Files = append(request.Files, remotePreuploadFile{Path: p, Size: len(content), Sample: base64.StdEncoding.EncodeToString(sample)})
		expected[p] = true
	}
	var preupload struct {
		Data struct {
			Files []remotePreuploadResult `json:"files"`
		} `json:"data"`
	}
	if err := s.postJSON(ctx, endpoint, request, &preupload); err != nil {
		return err
	}
	for _, file := range preupload.Data.Files {
		if !expected[file.Path] {
			return fmt.Errorf("unexpected preupload file %q", file.Path)
		}
		if _, duplicate := actions[file.Path]; duplicate {
			return fmt.Errorf("duplicate preupload file %q", file.Path)
		}
		if file.ShouldIgnore {
			return fmt.Errorf("cloud template file %q cannot be uploaded", file.Path)
		}
		if file.UploadMode != "regular" {
			return fmt.Errorf("unsupported upload mode %q for template file %q", file.UploadMode, file.Path)
		}
		action := "create"
		if file.OID != "" && !file.IsDir {
			action = "update"
		}
		actions[file.Path] = action
	}
	for _, p := range paths {
		if _, ok := actions[p]; !ok {
			return fmt.Errorf("missing preupload result for %q", p)
		}
	}
	return nil
}
