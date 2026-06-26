package template

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"csgclaw/internal/apitypes"
	toml "github.com/pelletier/go-toml/v2"
)

const (
	defaultRemoteHTTPTimeout  = 60 * time.Second
	defaultRemoteMaxJSONBytes = 4 * 1024 * 1024
	defaultRemoteMaxFileBytes = 50 * 1024 * 1024
	officialTemplateNamespace = "Agentic"
	remoteManifestFileName    = "agent.toml"
	remoteWorkspaceDirName    = "workspace"
	remoteFilePreviewMaxBytes = 256 * 1024
)

type RemoteStore struct {
	hubBaseURL     string
	contentBaseURL string
	token          string
	httpClient     *http.Client
	maxJSON        int64
	maxWorkspace   int64
}

type remoteCodeListResponse struct {
	Data  []remoteCodeRepository `json:"data"`
	Total int                    `json:"total"`
}

type remoteCodeResponse struct {
	Data remoteCodeRepository `json:"data"`
}

type remoteCodeRepository struct {
	Name          string    `json:"name"`
	Nickname      string    `json:"nickname"`
	Description   string    `json:"description"`
	Path          string    `json:"path"`
	DefaultBranch string    `json:"default_branch"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type remoteTreeResponse struct {
	Data remoteTreeData `json:"data"`
}

type remoteTreeData struct {
	Files  []remoteTreeEntry `json:"Files"`
	Cursor string            `json:"Cursor"`
}

type remoteTreeEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type remoteBlobResponse struct {
	Data remoteBlob `json:"data"`
}

type remoteBlob struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func NewRemoteStore(baseURL, token string) *RemoteStore {
	hubBaseURL := strings.TrimSpace(strings.TrimRight(baseURL, "/"))
	return &RemoteStore{
		hubBaseURL:     hubBaseURL,
		contentBaseURL: hubBaseURL,
		token:          strings.TrimSpace(token),
		httpClient: &http.Client{
			Timeout: defaultRemoteHTTPTimeout,
		},
		maxJSON:      defaultRemoteMaxJSONBytes,
		maxWorkspace: defaultRemoteMaxFileBytes,
	}
}

func (s *RemoteStore) List(ctx context.Context) ([]Template, error) {
	var payload remoteCodeListResponse
	if err := s.getJSON(ctx, s.templatesURL(), &payload); err != nil {
		return nil, err
	}

	items := make([]Template, 0, len(payload.Data))
	for _, repository := range payload.Data {
		id, err := normalizeRemoteTemplateID(repository.Path)
		if err != nil {
			slog.Warn("skip invalid remote hub template path", "path", repository.Path, "error", err)
			continue
		}
		item, err := s.getTemplate(ctx, id, repository)
		if err != nil {
			slog.Warn("skip invalid remote hub template", "id", id, "error", err)
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *RemoteStore) Get(ctx context.Context, id string) (Template, error) {
	id, err := normalizeRemoteTemplateID(id)
	if err != nil {
		return Template{}, err
	}

	var payload remoteCodeResponse
	if err := s.getJSON(ctx, s.templateURL(id), &payload); err != nil {
		return Template{}, err
	}
	return s.getTemplate(ctx, id, payload.Data)
}

func (s *RemoteStore) getTemplate(ctx context.Context, id string, repository remoteCodeRepository) (Template, error) {
	branch := strings.TrimSpace(repository.DefaultBranch)
	if branch == "" {
		var payload remoteCodeResponse
		if err := s.getJSON(ctx, s.templateURL(id), &payload); err != nil {
			return Template{}, err
		}
		repository = payload.Data
		branch = strings.TrimSpace(repository.DefaultBranch)
	}
	if branch == "" {
		branch = "main"
	}

	manifest, err := s.fetchManifest(ctx, id, branch)
	if err != nil {
		return Template{}, err
	}
	updatedAt, err := parseManifestUpdatedAt(manifest.UpdatedAt)
	if err != nil {
		return Template{}, fmt.Errorf("validate remote hub manifest %q: %w", id, err)
	}
	if updatedAt.IsZero() {
		updatedAt = repository.UpdatedAt
	}
	description := strings.TrimSpace(manifest.Description)
	if description == "" {
		description = strings.TrimSpace(repository.Description)
	}
	name := strings.TrimSpace(manifest.Name)
	if name == "" {
		name = strings.TrimSpace(repository.Nickname)
	}
	if name == "" {
		name = strings.TrimSpace(repository.Name)
	}
	return Template{
		ID:           remoteTemplateName(id),
		Name:         name,
		Description:  description,
		Role:         normalizeTemplateRole(manifest.Role),
		RuntimeKind:  strings.TrimSpace(manifest.RuntimeKind),
		Version:      strings.TrimSpace(manifest.Version),
		Image:        manifestImageRef(manifest.Image),
		ImageEnv:     manifestImageEnv(manifest.Image),
		WorkspaceRef: WorkspaceRef{Kind: WorkspaceKindDir},
		UpdatedAt:    updatedAt,
	}, nil
}

func (s *RemoteStore) fetchManifest(ctx context.Context, id, branch string) (templateManifest, error) {
	data, err := s.fetchBlob(ctx, id, remoteManifestFileName, branch)
	if err != nil {
		return templateManifest{}, err
	}
	var manifest templateManifest
	if err := toml.Unmarshal(data, &manifest); err != nil {
		return templateManifest{}, fmt.Errorf("decode remote hub manifest %q: %w", id, err)
	}
	if err := validateManifest(manifest); err != nil {
		return templateManifest{}, fmt.Errorf("validate remote hub manifest %q: %w", id, err)
	}
	return manifest, nil
}

func (s *RemoteStore) FetchWorkspace(ctx context.Context, id string) (WorkspaceRef, error) {
	id, err := normalizeRemoteTemplateID(id)
	if err != nil {
		return WorkspaceRef{}, err
	}

	branch, err := s.defaultBranch(ctx, id)
	if err != nil {
		return WorkspaceRef{}, err
	}

	tmpDir, err := os.MkdirTemp("", "csgclaw-hub-remote-*")
	if err != nil {
		return WorkspaceRef{}, fmt.Errorf("create remote hub workspace temp dir: %w", err)
	}
	var totalBytes int64
	if err := s.fetchWorkspaceTree(ctx, id, branch, remoteWorkspaceDirName, tmpDir, &totalBytes); err != nil {
		_ = os.RemoveAll(tmpDir)
		if errors.Is(err, ErrTemplateNotFound) {
			return WorkspaceRef{}, nil
		}
		return WorkspaceRef{}, err
	}
	return WorkspaceRef{Kind: WorkspaceKindDir, Path: tmpDir}, nil
}

func (s *RemoteStore) ListWorkspace(
	ctx context.Context,
	id string,
	workspacePath string,
) (apitypes.WorkspaceListing, error) {
	id, err := normalizeRemoteTemplateID(id)
	if err != nil {
		return apitypes.WorkspaceListing{}, err
	}
	cleanPath, err := normalizeRemoteWorkspacePath(workspacePath)
	if err != nil {
		return apitypes.WorkspaceListing{}, err
	}
	branch, err := s.defaultBranch(ctx, id)
	if err != nil {
		return apitypes.WorkspaceListing{}, err
	}

	treePath := remoteWorkspaceDirName
	if cleanPath != "" {
		treePath += "/" + cleanPath
	}
	entries := make([]apitypes.WorkspaceEntry, 0)
	cursor := ""
	for {
		var payload remoteTreeResponse
		if err := s.getJSON(ctx, s.treeURL(id, branch, treePath, cursor), &payload); err != nil {
			if cleanPath == "" && errors.Is(err, ErrTemplateNotFound) {
				return apitypes.WorkspaceListing{Kind: WorkspaceKindDir}, nil
			}
			return apitypes.WorkspaceListing{}, err
		}
		for _, entry := range payload.Data.Files {
			entryPath := strings.Trim(strings.TrimSpace(entry.Path), "/")
			if !strings.HasPrefix(entryPath, remoteWorkspaceDirName+"/") {
				return apitypes.WorkspaceListing{}, fmt.Errorf("%w: %s", ErrWorkspacePathUnsafe, entryPath)
			}
			relativePath := strings.TrimPrefix(entryPath, remoteWorkspaceDirName+"/")
			if path.Dir(relativePath) != path.Clean(cleanPath) && !(cleanPath == "" && !strings.Contains(relativePath, "/")) {
				continue
			}
			entryType := "file"
			if entry.Type == "dir" || entry.Type == "tree" {
				entryType = "dir"
			}
			entries = append(entries, apitypes.WorkspaceEntry{
				Path:  relativePath,
				Name:  entry.Name,
				Type:  entryType,
				Depth: strings.Count(relativePath, "/"),
				Size:  entry.Size,
			})
		}
		cursor = strings.TrimSpace(payload.Data.Cursor)
		if cursor == "" {
			break
		}
	}
	return apitypes.WorkspaceListing{
		Kind:    WorkspaceKindDir,
		Path:    cleanPath,
		Entries: entries,
	}, nil
}

func (s *RemoteStore) ReadWorkspaceFile(
	ctx context.Context,
	id string,
	workspacePath string,
) (apitypes.WorkspaceFile, error) {
	id, err := normalizeRemoteTemplateID(id)
	if err != nil {
		return apitypes.WorkspaceFile{}, err
	}
	cleanPath, err := normalizeRemoteWorkspacePath(workspacePath)
	if err != nil || cleanPath == "" {
		return apitypes.WorkspaceFile{}, ErrWorkspacePathUnsafe
	}
	branch, err := s.defaultBranch(ctx, id)
	if err != nil {
		return apitypes.WorkspaceFile{}, err
	}
	data, err := s.fetchBlob(ctx, id, remoteWorkspaceDirName+"/"+cleanPath, branch)
	if err != nil {
		return apitypes.WorkspaceFile{}, err
	}
	file := apitypes.WorkspaceFile{Path: cleanPath, Size: int64(len(data))}
	preview := data
	if len(preview) > remoteFilePreviewMaxBytes {
		preview = preview[:remoteFilePreviewMaxBytes]
		file.Truncated = true
		validPreview := false
		for trim := 0; trim < utf8.UTFMax && trim < len(preview); trim++ {
			candidate := preview[:len(preview)-trim]
			if utf8.Valid(candidate) {
				preview = candidate
				validPreview = true
				break
			}
		}
		if !validPreview {
			file.Binary = true
			return file, nil
		}
	}
	if !utf8.Valid(preview) {
		file.Binary = true
		return file, nil
	}
	file.Content = string(preview)
	return file, nil
}

func (s *RemoteStore) defaultBranch(ctx context.Context, id string) (string, error) {
	var payload remoteCodeResponse
	if err := s.getJSON(ctx, s.templateURL(id), &payload); err != nil {
		return "", err
	}
	branch := strings.TrimSpace(payload.Data.DefaultBranch)
	if branch == "" {
		branch = "main"
	}
	return branch, nil
}

func (s *RemoteStore) fetchWorkspaceTree(
	ctx context.Context,
	id string,
	branch string,
	treePath string,
	dstRoot string,
	totalBytes *int64,
) error {
	cursor := ""
	for {
		var payload remoteTreeResponse
		if err := s.getJSON(ctx, s.treeURL(id, branch, treePath, cursor), &payload); err != nil {
			return err
		}
		for _, entry := range payload.Data.Files {
			entryPath := strings.Trim(strings.TrimSpace(entry.Path), "/")
			if entryPath != remoteWorkspaceDirName &&
				!strings.HasPrefix(entryPath, remoteWorkspaceDirName+"/") {
				return fmt.Errorf("%w: %s", ErrWorkspacePathUnsafe, entryPath)
			}
			rel := strings.TrimPrefix(entryPath, remoteWorkspaceDirName+"/")
			if entryPath == remoteWorkspaceDirName {
				rel = ""
			}
			if rel != "" {
				if err := validateWorkspaceRelativePath(filepath.FromSlash(rel)); err != nil {
					return err
				}
			}
			switch strings.ToLower(strings.TrimSpace(entry.Type)) {
			case "dir", "tree":
				if rel != "" {
					if err := os.MkdirAll(filepath.Join(dstRoot, filepath.FromSlash(rel)), 0o755); err != nil {
						return fmt.Errorf("create remote hub workspace dir %q: %w", rel, err)
					}
				}
				if err := s.fetchWorkspaceTree(ctx, id, branch, entryPath, dstRoot, totalBytes); err != nil {
					return err
				}
			case "file", "blob":
				data, err := s.fetchBlob(ctx, id, entryPath, branch)
				if err != nil {
					return err
				}
				*totalBytes += int64(len(data))
				if *totalBytes > s.maxWorkspace {
					return fmt.Errorf("remote hub workspace exceeds %d bytes", s.maxWorkspace)
				}
				target := filepath.Join(dstRoot, filepath.FromSlash(rel))
				if err := ensurePathInsideRoot(dstRoot, target); err != nil {
					return err
				}
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					return fmt.Errorf("create remote hub workspace parent %q: %w", filepath.Dir(target), err)
				}
				if err := os.WriteFile(target, data, 0o644); err != nil {
					return fmt.Errorf("write remote hub workspace file %q: %w", target, err)
				}
			}
		}
		cursor = strings.TrimSpace(payload.Data.Cursor)
		if cursor == "" {
			return nil
		}
	}
}

func (s *RemoteStore) fetchBlob(ctx context.Context, id, filePath, branch string) ([]byte, error) {
	var payload remoteBlobResponse
	if err := s.getJSON(ctx, s.blobURL(id, filePath, branch), &payload); err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(payload.Data.Content)
	if err != nil {
		return nil, fmt.Errorf("decode remote hub blob %q: %w", filePath, err)
	}
	if int64(len(data)) > s.maxWorkspace {
		return nil, fmt.Errorf("remote hub blob %q exceeds %d bytes", filePath, s.maxWorkspace)
	}
	return data, nil
}

func (s *RemoteStore) Publish(context.Context, PublishSpec) (Template, error) {
	return Template{}, ErrRegistryNotWritable
}

func (s *RemoteStore) Delete(context.Context, string) error {
	return ErrRegistryNotDeletable
}

func (s *RemoteStore) templatesURL() string {
	return s.hubBaseURL + "/api/v1/organization/" + url.PathEscape(officialTemplateNamespace) + "/codes"
}

func (s *RemoteStore) templateURL(id string) string {
	return s.contentBaseURL + "/api/v1/codes/" + escapeRemotePath(id)
}

func (s *RemoteStore) treeURL(id, branch, treePath, cursor string) string {
	endpoint := s.templateURL(id) + "/refs/" + url.PathEscape(branch) + "/tree/"
	if treePath != "" {
		endpoint += escapeRemotePath(treePath)
	}
	query := url.Values{}
	query.Set("cursor", cursor)
	query.Set("limit", "500")
	return endpoint + "?" + query.Encode()
}

func (s *RemoteStore) blobURL(id, filePath, branch string) string {
	query := url.Values{}
	query.Set("ref", branch)
	return s.templateURL(id) + "/blob/" + escapeRemotePath(filePath) + "?" + query.Encode()
}

func (s *RemoteStore) getJSON(ctx context.Context, endpoint string, out any) error {
	body, status, err := s.request(ctx, http.MethodGet, endpoint, s.maxJSON+1)
	if err != nil {
		return err
	}
	if int64(len(body)) > s.maxJSON {
		return fmt.Errorf("remote hub response exceeds %d bytes", s.maxJSON)
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("%w", ErrTemplateNotFound)
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("remote hub request failed with status %d: %s", status, truncateRemoteBody(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode remote hub response: %w", err)
	}
	return nil
}

func (s *RemoteStore) request(ctx context.Context, method, endpoint string, maxBody int64) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create remote hub request: %w", err)
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("remote hub request %s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if maxBody > 0 {
		reader = io.LimitReader(resp.Body, maxBody)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read remote hub response: %w", err)
	}
	return body, resp.StatusCode, nil
}

func normalizeRemoteTemplateID(id string) (string, error) {
	id = strings.Trim(strings.TrimSpace(id), "/")
	if id == "" {
		return "", ErrTemplateIDRequired
	}
	if !strings.Contains(id, "/") {
		id = officialTemplateNamespace + "/" + id
	}
	parts := strings.Split(id, "/")
	if len(parts) != 2 || parts[0] != officialTemplateNamespace {
		return "", ErrWorkspacePathUnsafe
	}
	for _, part := range parts {
		if err := validateLocalTemplateID(part); err != nil {
			return "", err
		}
	}
	return strings.Join(parts, "/"), nil
}

func escapeRemotePath(value string) string {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return path.Join(parts...)
}

func remoteTemplateName(id string) string {
	return strings.TrimPrefix(id, officialTemplateNamespace+"/")
}

func normalizeRemoteWorkspacePath(value string) (string, error) {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" {
		return "", nil
	}
	if err := validateWorkspaceRelativePath(filepath.FromSlash(value)); err != nil {
		return "", err
	}
	return path.Clean(value), nil
}

func ensurePathInsideRoot(root, target string) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrWorkspacePathUnsafe, target)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %s", ErrWorkspacePathUnsafe, target)
	}
	return nil
}

func truncateRemoteBody(body []byte) string {
	const limit = 512
	text := strings.TrimSpace(string(body))
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
