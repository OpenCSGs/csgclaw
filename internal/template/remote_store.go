package template

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"csgclaw/internal/apitypes"
	toml "github.com/pelletier/go-toml/v2"
)

const (
	defaultRemoteHTTPTimeout    = 60 * time.Second
	defaultRemoteMaxJSONBytes   = 4 * 1024 * 1024
	defaultRemoteMaxFileBytes   = 50 * 1024 * 1024
	officialTemplateNamespace   = "Agentic"
	remoteManifestFileName      = "agent.toml"
	remoteFilePreviewMaxBytes   = 256 * 1024
	remoteAgentTemplatesPerPage = 20
)

type RemoteStore struct {
	hubBaseURL     string
	contentBaseURL string
	token          string
	username       string
	httpClient     *http.Client
	maxJSON        int64
	maxWorkspace   int64
}

// RemoteAPIError preserves the stable error contract returned by the Hub API.
type RemoteAPIError struct {
	StatusCode int
	Code       string
	Message    string
	Cause      error
}

func (e *RemoteAPIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code != "" && e.Message != "" {
		return fmt.Sprintf("remote hub request failed with status %d: %s: %s", e.StatusCode, e.Code, e.Message)
	}
	if e.Code != "" {
		return fmt.Sprintf("remote hub request failed with status %d: %s", e.StatusCode, e.Code)
	}
	return fmt.Sprintf("remote hub request failed with status %d: %s", e.StatusCode, e.Message)
}

func (e *RemoteAPIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func RemoteAPIErrorCode(err error) string {
	var remoteErr *RemoteAPIError
	if errors.As(err, &remoteErr) {
		return strings.TrimSpace(remoteErr.Code)
	}
	return ""
}

type remoteCodeListResponse struct {
	Data  []remoteCodeRepository `json:"data"`
	Total int                    `json:"total"`
}

type remoteAgentTemplateListResponse struct {
	Data  []remoteAgentTemplate `json:"data"`
	Total int                   `json:"total"`
}

type remoteAgentTemplate struct {
	ID          int64                       `json:"id"`
	Type        string                      `json:"type"`
	Name        string                      `json:"name"`
	Description string                      `json:"description"`
	Public      bool                        `json:"public"`
	Metadata    remoteAgentTemplateMetadata `json:"metadata"`
	UpdatedAt   time.Time                   `json:"updated_at"`
}

type remoteAgentTemplateMetadata struct {
	RepoPath       string                        `json:"repo_path"`
	SensitiveCheck *remoteTemplateSensitiveCheck `json:"sensitive_check"`
}

type remoteTemplateSensitiveCheck struct {
	Status         string                                `json:"status"`
	FailureDetails []remoteTemplateSensitiveCheckFailure `json:"failure_details"`
}

type remoteTemplateSensitiveCheckFailure struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type remoteCodeResponse struct {
	Data remoteCodeRepository `json:"data"`
}

type remoteUploadURLResponse struct {
	Data remoteUploadURL `json:"data"`
}

type remoteUploadURL struct {
	URL      string            `json:"url"`
	FormData map[string]string `json:"formData"`
	UUID     string            `json:"uuid"`
}

type remoteCreateCodeRequest struct {
	Namespace     string `json:"namespace"`
	Name          string `json:"name"`
	Nickname      string `json:"nickname"`
	Description   string `json:"description"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	Type          string `json:"type"`
	CodeFile      string `json:"code_file"`
}

type remoteCodeRepository struct {
	Name          string            `json:"name"`
	Nickname      string            `json:"nickname"`
	Description   string            `json:"description"`
	Path          string            `json:"path"`
	DefaultBranch string            `json:"default_branch"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Metadata      *TemplateMetadata `json:"-"`
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

func NewAuthenticatedRemoteStore(baseURL, token, username string) *RemoteStore {
	store := NewRemoteStore(baseURL, token)
	store.username = strings.TrimSpace(username)
	return store
}

func (s *RemoteStore) List(ctx context.Context) ([]Template, error) {
	repositories := make([]remoteCodeRepository, 0)
	seen := make(map[string]int)
	var listErrs []error
	var organizationTemplates remoteCodeListResponse
	if err := s.getJSON(ctx, s.templatesURL(), &organizationTemplates); err != nil {
		listErrs = append(listErrs, err)
	} else {
		repositories = appendRemoteTemplateRepositories(repositories, seen, organizationTemplates.Data)
	}

	agentTemplates, err := s.listAgentTemplates(ctx)
	if err != nil {
		listErrs = append(listErrs, err)
	}
	for _, item := range agentTemplates {
		if !strings.EqualFold(strings.TrimSpace(item.Type), "csgclaw") {
			continue
		}
		repositories = appendRemoteTemplateRepositories(repositories, seen, []remoteCodeRepository{{
			Name:        strings.TrimSpace(item.Name),
			Nickname:    strings.TrimSpace(item.Name),
			Description: strings.TrimSpace(item.Description),
			Path:        strings.TrimSpace(item.Metadata.RepoPath),
			UpdatedAt:   item.UpdatedAt,
			Metadata:    remoteTemplateMetadata(item.Metadata),
		}})
	}
	if len(repositories) == 0 && len(listErrs) > 0 {
		return nil, errors.Join(listErrs...)
	}

	items := make([]Template, 0, len(repositories))
	for _, repository := range repositories {
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

func (s *RemoteStore) listAgentTemplates(ctx context.Context) ([]remoteAgentTemplate, error) {
	items := make([]remoteAgentTemplate, 0)
	for page := 1; ; page++ {
		var payload remoteAgentTemplateListResponse
		if err := s.getJSON(ctx, s.agentTemplatesURL(page), &payload); err != nil {
			return items, err
		}
		items = append(items, payload.Data...)
		if len(items) >= payload.Total || len(payload.Data) == 0 {
			return items, nil
		}
	}
}

func appendRemoteTemplateRepositories(
	repositories []remoteCodeRepository,
	seen map[string]int,
	items []remoteCodeRepository,
) []remoteCodeRepository {
	for _, repository := range items {
		id, err := normalizeRemoteTemplateID(repository.Path)
		if err != nil {
			slog.Warn("skip invalid remote hub template path", "path", repository.Path, "error", err)
			continue
		}
		if index, ok := seen[id]; ok {
			if repository.Metadata != nil {
				repositories[index].Metadata = repository.Metadata
			}
			continue
		}
		seen[id] = len(repositories)
		repositories = append(repositories, repository)
	}
	return repositories
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
		metadata := repository.Metadata
		var payload remoteCodeResponse
		if err := s.getJSON(ctx, s.templateURL(id), &payload); err != nil {
			return Template{}, err
		}
		repository = payload.Data
		if repository.Metadata == nil {
			repository.Metadata = metadata
		}
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
	namespace := strings.SplitN(id, "/", 2)[0]
	runtimeOptions, err := normalizeTemplateRuntimeOptions(manifest.RuntimeKind, manifest.RuntimeOptions)
	if err != nil {
		return Template{}, fmt.Errorf("validate remote hub manifest %q runtime options: %w", id, err)
	}
	return Template{
		ID:             remoteTemplateName(id),
		Namespace:      namespace,
		Name:           name,
		Description:    description,
		Role:           TemplateRoleWorker,
		RuntimeKind:    normalizeTemplateRuntimeKind(manifest.RuntimeKind),
		Version:        strings.TrimSpace(manifest.Version),
		Image:          manifestImageRef(manifest.Image),
		ImageEnv:       manifestImageEnv(manifest.Image),
		RuntimeOptions: runtimeOptions,
		Metadata:       repository.Metadata,
		WorkspaceRef:   WorkspaceRef{Kind: WorkspaceKindDir},
		UpdatedAt:      updatedAt,
	}, nil
}

func remoteTemplateMetadata(metadata remoteAgentTemplateMetadata) *TemplateMetadata {
	if metadata.SensitiveCheck == nil {
		return nil
	}
	details := make([]TemplateSensitiveCheckFailure, 0, len(metadata.SensitiveCheck.FailureDetails))
	for _, detail := range metadata.SensitiveCheck.FailureDetails {
		details = append(details, TemplateSensitiveCheckFailure{
			Path: strings.TrimSpace(detail.Path), Status: strings.TrimSpace(detail.Status), Message: strings.TrimSpace(detail.Message),
		})
	}
	return &TemplateMetadata{SensitiveCheck: &TemplateSensitiveCheck{
		Status: strings.TrimSpace(metadata.SensitiveCheck.Status), FailureDetails: details,
	}}
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
	archive, err := s.downloadArchive(ctx, id, branch)
	if err != nil {
		return WorkspaceRef{}, err
	}
	templateDir, err := mkdirHubWorkspaceTemp("csgclaw-hub-remote-*")
	if err != nil {
		return WorkspaceRef{}, fmt.Errorf("create remote hub workspace temp dir: %w", err)
	}
	if err := extractRemoteTemplateArchive(archive, templateDir, s.maxWorkspace); err != nil {
		_ = os.RemoveAll(templateDir)
		return WorkspaceRef{}, err
	}
	workspace, err := materializeTemplateDir(templateDir, "")
	_ = os.RemoveAll(templateDir)
	return workspace, err
}

func (s *RemoteStore) downloadArchive(ctx context.Context, id, branch string) ([]byte, error) {
	body, status, err := s.requestAccept(ctx, http.MethodGet, s.archiveURL(id, branch), "application/zip", s.maxWorkspace+1)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > s.maxWorkspace {
		return nil, fmt.Errorf("remote hub template archive exceeds %d bytes", s.maxWorkspace)
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("%w", ErrTemplateNotFound)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("remote hub archive request failed with status %d: %s", status, truncateRemoteBody(body))
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("remote hub template archive is empty")
	}
	return body, nil
}

func extractRemoteTemplateArchive(archive []byte, dstRoot string, maxBytes int64) error {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return fmt.Errorf("open remote hub template archive: %w", err)
	}
	if len(reader.File) == 0 {
		return fmt.Errorf("remote hub template archive is empty")
	}
	prefix := remoteArchiveRootPrefix(reader.File)
	var totalBytes int64
	for _, file := range reader.File {
		if file == nil {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrWorkspacePathUnsafe, file.Name)
		}
		archiveName := strings.TrimSpace(file.Name)
		if prefix != "" && !strings.HasPrefix(archiveName, prefix) {
			continue
		}
		name := strings.TrimPrefix(archiveName, prefix)
		name = strings.TrimPrefix(name, "/")
		if name == "" {
			continue
		}
		rel := filepath.Clean(filepath.FromSlash(name))
		if err := validateWorkspaceRelativePath(rel); err != nil {
			return err
		}
		target := filepath.Join(dstRoot, rel)
		if err := ensurePathInsideRoot(dstRoot, target); err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create remote hub archive directory %q: %w", rel, err)
			}
			continue
		}
		if !file.Mode().IsRegular() {
			continue
		}
		if file.UncompressedSize64 > uint64(maxBytes) || totalBytes > maxBytes-int64(file.UncompressedSize64) {
			return fmt.Errorf("remote hub template archive exceeds %d uncompressed bytes", maxBytes)
		}
		totalBytes += int64(file.UncompressedSize64)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create remote hub archive parent %q: %w", rel, err)
		}
		if err := extractRemoteArchiveFile(file, target, maxBytes); err != nil {
			return err
		}
	}
	return nil
}

func remoteArchiveRootPrefix(files []*zip.File) string {
	for _, file := range files {
		if file == nil {
			continue
		}
		name := strings.Trim(strings.TrimSpace(file.Name), "/")
		if name == remoteManifestFileName {
			return ""
		}
		if strings.HasSuffix(name, "/"+remoteManifestFileName) && strings.Count(name, "/") == 1 {
			return strings.SplitN(name, "/", 2)[0] + "/"
		}
	}
	return ""
}

func extractRemoteArchiveFile(file *zip.File, target string, maxBytes int64) error {
	source, err := file.Open()
	if err != nil {
		return fmt.Errorf("open remote hub archive entry %q: %w", file.Name, err)
	}
	defer source.Close()
	mode := file.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	mode |= 0o200
	destination, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create remote hub archive file %q: %w", file.Name, err)
	}
	written, copyErr := io.Copy(destination, io.LimitReader(source, maxBytes+1))
	closeErr := destination.Close()
	if copyErr != nil {
		return fmt.Errorf("extract remote hub archive file %q: %w", file.Name, copyErr)
	}
	if written > maxBytes {
		return fmt.Errorf("remote hub template archive entry %q exceeds %d bytes", file.Name, maxBytes)
	}
	if closeErr != nil {
		return fmt.Errorf("close remote hub archive file %q: %w", file.Name, closeErr)
	}
	return nil
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

	treePath := cleanPath
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
			relativePath := entryPath
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
	data, err := s.fetchBlob(ctx, id, cleanPath, branch)
	if errors.Is(err, ErrTemplateNotFound) {
		if legacyPath := legacyRemoteWorkspacePath(cleanPath); legacyPath != "" {
			data, err = s.fetchBlob(ctx, id, legacyPath, branch)
		}
	}
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

func legacyRemoteWorkspacePath(workspacePath string) string {
	switch workspacePath {
	case localInstructionsDirName + "/" + requiredInstructionsFile:
		return "workspace/" + requiredInstructionsFile
	}
	if strings.HasPrefix(workspacePath, localSkillsDirName+"/") {
		return "workspace/" + workspacePath
	}
	return ""
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
			rootName := strings.Split(strings.Trim(treePath, "/"), "/")[0]
			if entryPath != rootName && !strings.HasPrefix(entryPath, rootName+"/") {
				return fmt.Errorf("%w: %s", ErrWorkspacePathUnsafe, entryPath)
			}
			rel := entryPath
			if entryPath == rootName {
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

func (s *RemoteStore) Publish(ctx context.Context, spec PublishSpec) (Template, error) {
	if strings.TrimSpace(s.token) == "" || strings.TrimSpace(s.username) == "" {
		return Template{}, fmt.Errorf("OpenCSG sign-in is required to publish templates")
	}
	normalized, err := normalizePublishSpec(spec)
	if err != nil {
		return Template{}, err
	}
	archive, err := buildRemoteTemplateArchive(normalized)
	if err != nil {
		return Template{}, err
	}
	if int64(len(archive)) > s.maxWorkspace {
		return Template{}, fmt.Errorf("remote hub template archive exceeds %d bytes", s.maxWorkspace)
	}

	var upload remoteUploadURLResponse
	if err := s.postJSON(ctx, s.uploadURL(), nil, &upload); err != nil {
		return Template{}, fmt.Errorf("request remote hub upload URL: %w", err)
	}
	if err := s.uploadArchive(ctx, upload.Data, archive); err != nil {
		return Template{}, err
	}

	request := remoteCreateCodeRequest{
		Namespace:     s.username,
		Name:          normalized.Name,
		Nickname:      normalized.Name,
		Description:   normalized.Description,
		Private:       true,
		DefaultBranch: "main",
		Type:          "template",
		CodeFile:      strings.TrimSpace(upload.Data.UUID),
	}
	var created remoteCodeResponse
	if err := s.postJSON(ctx, s.codesURL(), request, &created); err != nil {
		return Template{}, fmt.Errorf("create remote hub template: %w", err)
	}
	id := strings.TrimSpace(created.Data.Path)
	if id == "" {
		id = path.Join(s.username, normalized.Name)
	}
	return Template{
		ID:             id,
		Namespace:      s.username,
		Name:           normalized.Name,
		Description:    normalized.Description,
		Role:           TemplateRoleWorker,
		RuntimeKind:    normalized.RuntimeKind,
		Version:        normalized.Version,
		Image:          normalized.Image,
		RuntimeOptions: cloneTemplateRuntimeOptions(normalized.RuntimeOptions),
		WorkspaceRef:   WorkspaceRef{Kind: WorkspaceKindDir},
		UpdatedAt:      normalized.UpdatedAt,
	}, nil
}

func (s *RemoteStore) Delete(context.Context, string) error {
	return ErrRegistryNotDeletable
}

func (s *RemoteStore) templatesURL() string {
	return s.hubBaseURL + "/api/v1/organization/" + url.PathEscape(officialTemplateNamespace) + "/codes"
}

func (s *RemoteStore) agentTemplatesURL(page int) string {
	query := url.Values{}
	query.Set("type", "csgclaw")
	query.Set("page", strconv.Itoa(page))
	query.Set("per", strconv.Itoa(remoteAgentTemplatesPerPage))
	return s.hubBaseURL + "/api/v1/agent/templates?" + query.Encode()
}

func (s *RemoteStore) uploadURL() string {
	query := url.Values{}
	query.Set("current_user", s.username)
	return s.hubBaseURL + "/api/v1/codes/upload_url?" + query.Encode()
}

func (s *RemoteStore) codesURL() string {
	query := url.Values{}
	query.Set("current_user", s.username)
	return s.hubBaseURL + "/api/v1/codes?" + query.Encode()
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

func (s *RemoteStore) archiveURL(id, branch string) string {
	return s.templateURL(id) + "/download_archive/refs/" + url.PathEscape(branch)
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
	if len(strings.TrimSpace(string(body))) == 0 {
		return fmt.Errorf("%w", ErrTemplateNotFound)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode remote hub response: %w", err)
	}
	return nil
}

func (s *RemoteStore) postJSON(ctx context.Context, endpoint string, input, out any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode remote hub request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return fmt.Errorf("create remote hub request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Accept", "application/json")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("remote hub request POST %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, s.maxJSON+1))
	if err != nil {
		return fmt.Errorf("read remote hub response: %w", err)
	}
	if int64(len(data)) > s.maxJSON {
		return fmt.Errorf("remote hub response exceeds %d bytes", s.maxJSON)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		remoteErr := decodeRemoteAPIError(resp.StatusCode, data)
		if strings.EqualFold(remoteErr.Code, "SENSITIVE-ERR-0") {
			remoteErr.Cause = ErrTemplateSensitiveInfo
		}
		return remoteErr
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode remote hub response: %w", err)
	}
	return nil
}

func decodeRemoteAPIError(statusCode int, data []byte) *RemoteAPIError {
	var payload struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(data, &payload); err == nil {
		remoteErr := &RemoteAPIError{
			StatusCode: statusCode,
			Code:       strings.TrimSpace(payload.Code),
			Message:    strings.TrimSpace(payload.Msg),
		}
		if remoteErr.Code != "" || remoteErr.Message != "" {
			return remoteErr
		}
	}
	return &RemoteAPIError{StatusCode: statusCode, Message: truncateRemoteBody(data)}
}

func (s *RemoteStore) uploadArchive(ctx context.Context, upload remoteUploadURL, archive []byte) error {
	if strings.TrimSpace(upload.URL) == "" || strings.TrimSpace(upload.UUID) == "" {
		return fmt.Errorf("remote hub upload response is incomplete")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range upload.FormData {
		if err := writer.WriteField(key, value); err != nil {
			return fmt.Errorf("encode remote hub upload field %q: %w", key, err)
		}
	}
	part, err := writer.CreateFormFile("file", upload.UUID+".zip")
	if err != nil {
		return fmt.Errorf("encode remote hub upload file: %w", err)
	}
	if _, err := part.Write(archive); err != nil {
		return fmt.Errorf("encode remote hub template archive: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish remote hub upload body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upload.URL, &body)
	if err != nil {
		return fmt.Errorf("create remote hub archive upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload remote hub template archive: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 513))
	if err != nil {
		return fmt.Errorf("read remote hub archive upload response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("remote hub archive upload failed with status %d: %s", resp.StatusCode, truncateRemoteBody(responseBody))
	}
	return nil
}

func buildRemoteTemplateArchive(spec PublishSpec) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "csgclaw-hub-publish-*")
	if err != nil {
		return nil, fmt.Errorf("create remote hub template temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	if err := (&LocalStore{}).writeManifest(filepath.Join(tmpDir, localManifestFileName), spec); err != nil {
		return nil, err
	}
	if spec.WorkspaceRef.Kind == WorkspaceKindDir {
		if err := writeTemplateLayout(spec.WorkspaceRef, tmpDir, spec.RuntimeKind, spec.MCPServers, spec.IncludeMemory); err != nil {
			return nil, err
		}
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	err = filepath.WalkDir(tmpDir, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(tmpDir, filePath)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		header.Method = zip.Deflate
		target, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		source, err := os.Open(filePath)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(target, source)
		closeErr := source.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		_ = archive.Close()
		return nil, fmt.Errorf("build remote hub template archive: %w", err)
	}
	if err := archive.Close(); err != nil {
		return nil, fmt.Errorf("finish remote hub template archive: %w", err)
	}
	return output.Bytes(), nil
}

func (s *RemoteStore) request(ctx context.Context, method, endpoint string, maxBody int64) ([]byte, int, error) {
	return s.requestAccept(ctx, method, endpoint, "application/json", maxBody)
}

func (s *RemoteStore) requestAccept(ctx context.Context, method, endpoint, accept string, maxBody int64) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create remote hub request: %w", err)
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	req.Header.Set("Accept", accept)

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
	if len(parts) != 2 {
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
	return strings.Trim(strings.TrimSpace(id), "/")
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
