package remote

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	pathpkg "path"
	"strconv"
	"strings"
	"time"

	skilllocal "csgclaw/internal/skill/local"
)

const (
	agenticHubDefaultRef        = "main"
	agenticHubDefaultSkillsPage = 16
	agenticHubMaxArchiveFiles   = 10000
	agenticHubSkillTreeLimit    = 500
	agenticHubMaxTreePages      = 1000
	agenticHubMaxJSONBytes      = 4 << 20
	agenticHubMaxArchiveBytes   = 64 << 20
	agenticHubRequestTimeout    = 60 * time.Second
	agenticHubSkillsAPIPathRoot = "skills"
)

var ErrInvalidAgenticHubRequest = errors.New("invalid AgenticHub skill request")

type AgenticHubSkillListOptions struct {
	Page   int
	Per    int
	Search string
}

type AgenticHubSkillSummary struct {
	Description string
	Name        string
	Ref         string
	RemotePath  string
}

type AgenticHubSkillList struct {
	Items       []AgenticHubSkillSummary
	RecordCount int
	Total       *int
}

type agenticHubTreeResponse struct {
	Data struct {
		Files  []agenticHubTreeEntry `json:"Files"`
		Cursor string                `json:"Cursor"`
	} `json:"data"`
}

type agenticHubTreeEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	Type string `json:"type"`
}

type agenticHubBlobResponse struct {
	Data struct {
		Content string `json:"content"`
		Path    string `json:"path"`
		Size    int64  `json:"size"`
		Type    string `json:"type"`
	} `json:"data"`
}

type agenticHubSkillsResponse struct {
	Data  []agenticHubSkillRecord `json:"data"`
	Total json.RawMessage         `json:"total"`
}

type agenticHubSkillRecord struct {
	DefaultBranch      string `json:"default_branch"`
	DefaultBranchCamel string `json:"defaultBranch"`
	Description        string `json:"description"`
	DisplayName        string `json:"displayName"`
	DisplayNameSnake   string `json:"display_name"`
	Name               string `json:"name"`
	Nickname           string `json:"nickname"`
	Path               string `json:"path"`
	Summary            string `json:"summary"`
	Title              string `json:"title"`
}

type agenticHubArchiveBuilder struct {
	baseURL    string
	client     *http.Client
	files      int
	ref        string
	remotePath string
	skillName  string
	totalBytes int64
	treePages  int
	visited    map[string]struct{}
}

func FetchAgenticHubSkillArchive(ctx context.Context, baseURL, remotePath, ref string) ([]byte, error) {
	remotePath, ref, err := NormalizeAgenticHubSkillRequest(remotePath, ref)
	if err != nil {
		return nil, err
	}
	skillName, err := skilllocal.NormalizeName(agenticHubSkillName(remotePath))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAgenticHubRequest, err)
	}
	builder := &agenticHubArchiveBuilder{
		baseURL:    baseURL,
		client:     &http.Client{Timeout: agenticHubRequestTimeout},
		ref:        ref,
		remotePath: remotePath,
		skillName:  skillName,
		visited:    map[string]struct{}{},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := builder.writeTree(ctx, zw, ""); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if builder.files == 0 {
		_ = zw.Close()
		return nil, skilllocal.ErrSkillArchiveEmpty
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close remote skill archive: %w", err)
	}
	return buf.Bytes(), nil
}

func ListAgenticHubSkills(ctx context.Context, baseURL string, options AgenticHubSkillListOptions) (AgenticHubSkillList, error) {
	page := options.Page
	if page <= 0 {
		page = 1
	}
	per := options.Per
	if per <= 0 {
		per = agenticHubDefaultSkillsPage
	}
	endpoint, err := agenticHubSkillsListURL(baseURL, page, per, options.Search)
	if err != nil {
		return AgenticHubSkillList{}, err
	}
	var payload agenticHubSkillsResponse
	if err := getAgenticHubJSON(ctx, &http.Client{Timeout: agenticHubRequestTimeout}, endpoint, &payload); err != nil {
		return AgenticHubSkillList{}, err
	}
	items := make([]AgenticHubSkillSummary, 0, len(payload.Data))
	for _, record := range payload.Data {
		item, ok := agenticHubSkillSummaryFromRecord(record)
		if ok {
			items = append(items, item)
		}
	}
	return AgenticHubSkillList{
		Items:       items,
		RecordCount: len(payload.Data),
		Total:       agenticHubTotal(payload.Total),
	}, nil
}

func NormalizeAgenticHubSkillRequest(remotePath, ref string) (string, string, error) {
	remotePath, err := cleanAgenticHubPath(remotePath, false)
	if err != nil {
		return "", "", err
	}
	ref, err = cleanAgenticHubRef(ref)
	if err != nil {
		return "", "", err
	}
	return remotePath, ref, nil
}

func AgenticHubSkillArchiveName(remotePath string) string {
	return agenticHubSkillName(remotePath) + ".zip"
}

func AgenticHubSkillWebURL(baseURL, remotePath string) (string, error) {
	remotePath, err := cleanAgenticHubPath(remotePath, false)
	if err != nil {
		return "", err
	}
	parts := []string{"skills"}
	parts = append(parts, strings.Split(remotePath, "/")...)
	return buildAgenticHubURL(baseURL, parts, false, nil)
}

func IsInvalidAgenticHubRequest(err error) bool {
	return errors.Is(err, ErrInvalidAgenticHubRequest)
}

func (b *agenticHubArchiveBuilder) writeTree(ctx context.Context, zw *zip.Writer, treePath string) error {
	treePath, err := cleanAgenticHubPath(treePath, true)
	if err != nil {
		return err
	}
	if _, ok := b.visited[treePath]; ok {
		return nil
	}
	b.visited[treePath] = struct{}{}

	return b.walkTree(ctx, treePath, func(entry agenticHubTreeEntry) error {
		entryPath, err := cleanAgenticHubPath(entry.Path, false)
		if err != nil {
			return err
		}
		if isAgenticHubDir(entry.Type) {
			if err := b.writeTree(ctx, zw, entryPath); err != nil {
				return err
			}
			return nil
		}
		if b.files+1 > agenticHubMaxArchiveFiles {
			return fmt.Errorf("remote skill archive exceeds %d files", agenticHubMaxArchiveFiles)
		}
		content, err := b.fetchBlob(ctx, entryPath)
		if err != nil {
			return err
		}
		b.totalBytes += int64(len(content))
		if b.totalBytes > agenticHubMaxArchiveBytes {
			return fmt.Errorf("remote skill archive exceeds %d bytes", agenticHubMaxArchiveBytes)
		}
		header := &zip.FileHeader{
			Name:   pathpkg.Join(b.skillName, entryPath),
			Method: zip.Deflate,
		}
		header.SetMode(0o644)
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("create remote skill archive entry %q: %w", header.Name, err)
		}
		if _, err := writer.Write(content); err != nil {
			return fmt.Errorf("write remote skill archive entry %q: %w", header.Name, err)
		}
		b.files++
		return nil
	})
}

func (b *agenticHubArchiveBuilder) walkTree(
	ctx context.Context,
	treePath string,
	visit func(agenticHubTreeEntry) error,
) error {
	cursor := ""
	seenCursors := map[string]struct{}{}
	for {
		b.treePages++
		if b.treePages > agenticHubMaxTreePages {
			return fmt.Errorf("remote skill tree exceeds %d pages", agenticHubMaxTreePages)
		}
		endpoint, err := agenticHubSkillTreeURL(b.baseURL, b.remotePath, b.ref, treePath, cursor)
		if err != nil {
			return err
		}
		var payload agenticHubTreeResponse
		if err := getAgenticHubJSON(ctx, b.client, endpoint, &payload); err != nil {
			return err
		}
		for _, entry := range payload.Data.Files {
			if err := visit(entry); err != nil {
				return err
			}
		}
		cursor = strings.TrimSpace(payload.Data.Cursor)
		if cursor == "" {
			return nil
		}
		if _, ok := seenCursors[cursor]; ok {
			return fmt.Errorf("AgenticHub tree cursor repeated for %q", treePath)
		}
		seenCursors[cursor] = struct{}{}
	}
}

func (b *agenticHubArchiveBuilder) fetchBlob(ctx context.Context, filePath string) ([]byte, error) {
	endpoint, err := agenticHubSkillBlobURL(b.baseURL, b.remotePath, b.ref, filePath)
	if err != nil {
		return nil, err
	}
	var payload agenticHubBlobResponse
	if err := getAgenticHubJSON(ctx, b.client, endpoint, &payload); err != nil {
		return nil, err
	}
	content, err := decodeAgenticHubBase64(payload.Data.Content)
	if err != nil {
		return nil, fmt.Errorf("decode remote skill blob %q: %w", filePath, err)
	}
	return content, nil
}

func getAgenticHubJSON(ctx context.Context, client *http.Client, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create AgenticHub request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("AgenticHub request %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, agenticHubMaxJSONBytes+1))
	if err != nil {
		return fmt.Errorf("read AgenticHub response: %w", err)
	}
	if int64(len(body)) > agenticHubMaxJSONBytes {
		return fmt.Errorf("AgenticHub response exceeds %d bytes", agenticHubMaxJSONBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("AgenticHub request failed with status %d: %s", resp.StatusCode, truncateAgenticHubBody(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode AgenticHub response: %w", err)
	}
	return nil
}

func agenticHubSkillTreeURL(baseURL, remotePath, ref, treePath, cursor string) (string, error) {
	parts := []string{"api", "v1", agenticHubSkillsAPIPathRoot}
	parts = append(parts, strings.Split(remotePath, "/")...)
	parts = append(parts, "refs", ref, "tree")
	if treePath != "" {
		parts = append(parts, strings.Split(treePath, "/")...)
	}
	query := url.Values{}
	query.Set("cursor", cursor)
	query.Set("limit", fmt.Sprintf("%d", agenticHubSkillTreeLimit))
	return buildAgenticHubURL(baseURL, parts, treePath == "", query)
}

func agenticHubSkillBlobURL(baseURL, remotePath, ref, filePath string) (string, error) {
	parts := []string{"api", "v1", agenticHubSkillsAPIPathRoot}
	parts = append(parts, strings.Split(remotePath, "/")...)
	parts = append(parts, "blob")
	parts = append(parts, strings.Split(filePath, "/")...)
	query := url.Values{}
	query.Set("ref", ref)
	return buildAgenticHubURL(baseURL, parts, false, query)
}

func agenticHubSkillsListURL(baseURL string, page, per int, search string) (string, error) {
	query := url.Values{}
	query.Set("page", fmt.Sprintf("%d", page))
	query.Set("per", fmt.Sprintf("%d", per))
	query.Set("search", strings.TrimSpace(search))
	query.Set("sort", "trending")
	query.Set("source", "")
	return buildAgenticHubURL(baseURL, []string{"api", "v1", agenticHubSkillsAPIPathRoot}, false, query)
}

func buildAgenticHubURL(baseURL string, parts []string, trailingSlash bool, query url.Values) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid official Hub URL")
	}
	pathParts := cleanURLPathParts(u.Path)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pathParts = append(pathParts, part)
	}
	u.Path = "/" + strings.Join(pathParts, "/")
	rawPathParts := make([]string, 0, len(pathParts))
	for _, part := range pathParts {
		rawPathParts = append(rawPathParts, url.PathEscape(part))
	}
	u.RawPath = "/" + strings.Join(rawPathParts, "/")
	if trailingSlash && !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
		u.RawPath += "/"
	}
	u.RawQuery = query.Encode()
	u.Fragment = ""
	return u.String(), nil
}

func cleanURLPathParts(value string) []string {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	return cleaned
}

func cleanAgenticHubPath(value string, allowEmpty bool) (string, error) {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" {
		if allowEmpty {
			return "", nil
		}
		return "", fmt.Errorf("%w: remote skill path is required", ErrInvalidAgenticHubRequest)
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("%w: invalid remote skill path %q", ErrInvalidAgenticHubRequest, value)
		}
	}
	return strings.Join(parts, "/"), nil
}

func cleanAgenticHubRef(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return agenticHubDefaultRef, nil
	}
	if strings.ContainsAny(value, " \t\r\n") || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
		return "", fmt.Errorf("%w: invalid remote skill ref %q", ErrInvalidAgenticHubRequest, value)
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("%w: invalid remote skill ref %q", ErrInvalidAgenticHubRequest, value)
		}
	}
	return value, nil
}

func agenticHubSkillName(remotePath string) string {
	parts := strings.Split(strings.Trim(remotePath, "/"), "/")
	return strings.TrimSpace(parts[len(parts)-1])
}

func agenticHubSkillSummaryFromRecord(record agenticHubSkillRecord) (AgenticHubSkillSummary, bool) {
	remotePath, err := cleanAgenticHubPath(record.Path, false)
	if err != nil {
		return AgenticHubSkillSummary{}, false
	}
	name := usefulAgenticHubSkillTitle(
		record.Name,
		record.Nickname,
		record.DisplayName,
		record.DisplayNameSnake,
		record.Title,
	)
	if name == "" {
		name = agenticHubSkillName(remotePath)
	}
	return AgenticHubSkillSummary{
		Description: firstAgenticHubValue(record.Description, record.Summary),
		Name:        name,
		Ref:         firstAgenticHubValue(record.DefaultBranch, record.DefaultBranchCamel),
		RemotePath:  remotePath,
	}, true
}

func usefulAgenticHubSkillTitle(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !strings.EqualFold(value, "skill") {
			return value
		}
	}
	return ""
}

func firstAgenticHubValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func agenticHubTotal(raw json.RawMessage) *int {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var value int
	if err := json.Unmarshal(raw, &value); err == nil && value >= 0 {
		return &value
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || value < 0 {
		return nil
	}
	return &value
}

func isAgenticHubDir(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "dir" || value == "directory" || value == "tree"
}

func decodeAgenticHubBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return []byte{}, nil
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err == nil {
		return data, nil
	}
	data, rawErr := base64.RawStdEncoding.DecodeString(value)
	if rawErr == nil {
		return data, nil
	}
	return nil, err
}

func truncateAgenticHubBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 256 {
		return text[:256] + "..."
	}
	return text
}
