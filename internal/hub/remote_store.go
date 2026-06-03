package hub

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"csgclaw/internal/apitypes"
)

const (
	defaultRemoteHTTPTimeout     = 60 * time.Second
	defaultRemoteMaxJSONBytes    = 4 * 1024 * 1024
	defaultRemoteMaxArchiveBytes = 50 * 1024 * 1024
)

type RemoteStore struct {
	baseURL    string
	token      string
	httpClient *http.Client
	maxJSON    int64
	maxArchive int64
}

func NewRemoteStore(baseURL, token string) *RemoteStore {
	baseURL = strings.TrimSpace(strings.TrimRight(baseURL, "/"))
	return &RemoteStore{
		baseURL: baseURL,
		token:   strings.TrimSpace(token),
		httpClient: &http.Client{
			Timeout: defaultRemoteHTTPTimeout,
		},
		maxJSON:    defaultRemoteMaxJSONBytes,
		maxArchive: defaultRemoteMaxArchiveBytes,
	}
}

func (s *RemoteStore) List(ctx context.Context) ([]Template, error) {
	var payload []apitypes.HubTemplate
	if err := s.getJSON(ctx, s.templatesURL(), &payload); err != nil {
		return nil, err
	}
	items := make([]Template, 0, len(payload))
	for _, item := range payload {
		items = append(items, templateFromAPI(item))
	}
	return items, nil
}

func (s *RemoteStore) Get(ctx context.Context, id string) (Template, error) {
	id = strings.TrimSpace(id)
	if err := validateLocalTemplateID(id); err != nil {
		return Template{}, err
	}
	var payload apitypes.HubTemplate
	if err := s.getJSON(ctx, s.templateURL(id), &payload); err != nil {
		return Template{}, err
	}
	if strings.TrimSpace(payload.ID) == "" {
		payload.ID = id
	}
	return templateFromAPI(payload), nil
}

func (s *RemoteStore) FetchWorkspace(ctx context.Context, id string) (WorkspaceRef, error) {
	id = strings.TrimSpace(id)
	if err := validateLocalTemplateID(id); err != nil {
		return WorkspaceRef{}, err
	}
	archive, err := s.download(ctx, s.workspaceArchiveURL(id))
	if err != nil {
		return WorkspaceRef{}, err
	}
	tmpDir, err := os.MkdirTemp("", "csgclaw-hub-remote-*")
	if err != nil {
		return WorkspaceRef{}, fmt.Errorf("create remote hub workspace temp dir: %w", err)
	}
	if err := extractWorkspaceTarGz(archive, tmpDir, s.maxArchive); err != nil {
		_ = os.RemoveAll(tmpDir)
		return WorkspaceRef{}, err
	}
	return WorkspaceRef{Kind: WorkspaceKindDir, Path: tmpDir}, nil
}

func (s *RemoteStore) Publish(context.Context, PublishSpec) (Template, error) {
	return Template{}, ErrRegistryNotWritable
}

func (s *RemoteStore) Delete(context.Context, string) error {
	return ErrRegistryNotDeletable
}

func (s *RemoteStore) templatesURL() string {
	return s.baseURL + "/api/v1/hub/templates"
}

func (s *RemoteStore) templateURL(id string) string {
	return s.baseURL + "/api/v1/hub/templates/" + url.PathEscape(id)
}

func (s *RemoteStore) workspaceArchiveURL(id string) string {
	return s.templateURL(id) + "/workspace.tar.gz"
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

func (s *RemoteStore) download(ctx context.Context, endpoint string) ([]byte, error) {
	body, status, err := s.request(ctx, http.MethodGet, endpoint, s.maxArchive+1)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("%w", ErrTemplateNotFound)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("remote hub workspace download failed with status %d: %s", status, truncateRemoteBody(body))
	}
	if int64(len(body)) > s.maxArchive {
		return nil, fmt.Errorf("remote hub workspace archive exceeds %d bytes", s.maxArchive)
	}
	return body, nil
}

func (s *RemoteStore) request(ctx context.Context, method, endpoint string, maxBody int64) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create remote hub request: %w", err)
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	req.Header.Set("Accept", "*/*")

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

func templateFromAPI(item apitypes.HubTemplate) Template {
	kind := strings.TrimSpace(item.Workspace.Kind)
	return Template{
		ID:           strings.TrimSpace(item.ID),
		Name:         strings.TrimSpace(item.Name),
		Description:  strings.TrimSpace(item.Description),
		Role:         normalizeTemplateRole(item.Role),
		RuntimeKind:  strings.TrimSpace(item.RuntimeKind),
		Image:        strings.TrimSpace(item.Image),
		ImageEnv:     append([]apitypes.ImageEnvContract(nil), item.ImageEnv...),
		WorkspaceRef: WorkspaceRef{Kind: kind},
		UpdatedAt:    item.UpdatedAt,
	}
}

func extractWorkspaceTarGz(archive []byte, dstRoot string, maxBytes int64) error {
	if int64(len(archive)) > maxBytes {
		return fmt.Errorf("hub workspace archive exceeds %d bytes", maxBytes)
	}
	dstRoot = strings.TrimSpace(dstRoot)
	if dstRoot == "" {
		return ErrWorkspaceDirRequired
	}
	dstRoot = filepath.Clean(dstRoot)

	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("open remote hub workspace gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read remote hub workspace tar: %w", err)
		}
		if hdr == nil {
			continue
		}
		switch hdr.Typeflag {
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("%w: %s", ErrWorkspaceSymlinkDenied, hdr.Name)
		}

		rel := filepath.Clean(filepath.FromSlash(strings.TrimSpace(hdr.Name)))
		if rel == "." || rel == string(filepath.Separator) {
			continue
		}
		if err := validateWorkspaceRelativePath(rel); err != nil {
			return err
		}
		target := filepath.Join(dstRoot, rel)
		if err := ensurePathInsideRoot(dstRoot, target); err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create remote hub workspace dir %q: %w", target, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create remote hub workspace parent %q: %w", filepath.Dir(target), err)
			}
			mode := hdr.FileInfo().Mode().Perm()
			if mode == 0 {
				mode = 0o644
			}
			mode |= 0o200
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return fmt.Errorf("create remote hub workspace file %q: %w", target, err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return fmt.Errorf("write remote hub workspace file %q: %w", target, err)
			}
			if err := out.Close(); err != nil {
				return fmt.Errorf("close remote hub workspace file %q: %w", target, err)
			}
		default:
			continue
		}
	}
	return nil
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
	text := strings.TrimSpace(string(body))
	if len(text) > 256 {
		return text[:256] + "..."
	}
	return text
}
