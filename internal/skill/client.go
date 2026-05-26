package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"csgclaw/internal/apiclient"
	"csgclaw/internal/config"
)

const (
	defaultHTTPTimeout     = 60 * time.Second
	defaultMaxJSONBytes    = 4 * 1024 * 1024
	defaultMaxArchiveBytes = 50 * 1024 * 1024
	apiV1Prefix            = "/api/v1"
)

type Client struct {
	baseURL           string
	token             string
	nonSuspiciousOnly bool
	httpClient        apiclient.HTTPClient
	maxJSON           int64
	maxArchive        int64
}

func NewClient(cfg config.SkillConfig, httpClient apiclient.HTTPClient) *Client {
	resolved := cfg.Resolved()
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Client{
		baseURL:           resolved.BaseURL,
		token:             resolved.Token,
		nonSuspiciousOnly: resolved.NonSuspiciousOnly,
		httpClient:        httpClient,
		maxJSON:           defaultMaxJSONBytes,
		maxArchive:        defaultMaxArchiveBytes,
	}
}

func (c *Client) apiURL(path string, query url.Values) string {
	path = strings.TrimPrefix(path, "/")
	endpoint := c.baseURL + apiV1Prefix + "/" + path
	if query != nil && len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	return endpoint
}

func (c *Client) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("search query is required")
	}
	values := url.Values{}
	values.Set("q", query)
	if limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", limit))
	}
	c.applySafetyFilter(values)

	var payload SearchResponse
	if err := c.getCatalogJSON(ctx, c.apiURL("search", values), &payload); err != nil {
		return nil, err
	}
	return payload.Results, nil
}

func (c *Client) Get(ctx context.Context, slug string) (SkillGetResponse, error) {
	slug = normalizeSlug(slug)
	if slug == "" {
		return SkillGetResponse{}, fmt.Errorf("skill slug is required")
	}
	var payload SkillGetResponse
	if err := c.getJSON(ctx, c.apiURL("skills/"+url.PathEscape(slug), nil), &payload); err != nil {
		return SkillGetResponse{}, err
	}
	return payload, nil
}

func (c *Client) GetVersion(ctx context.Context, slug, version string) (SkillVersionGetResponse, error) {
	slug = normalizeSlug(slug)
	version = strings.TrimSpace(version)
	if slug == "" {
		return SkillVersionGetResponse{}, fmt.Errorf("skill slug is required")
	}
	if version == "" {
		return SkillVersionGetResponse{}, fmt.Errorf("skill version is required")
	}
	var payload SkillVersionGetResponse
	path := "skills/" + url.PathEscape(slug) + "/versions/" + url.PathEscape(version)
	if err := c.getJSON(ctx, c.apiURL(path, nil), &payload); err != nil {
		return SkillVersionGetResponse{}, err
	}
	return payload, nil
}

func (c *Client) Download(ctx context.Context, slug, version, tag string) ([]byte, error) {
	slug = normalizeSlug(slug)
	if slug == "" {
		return nil, fmt.Errorf("skill slug is required")
	}
	query := downloadQuery(version, tag)

	// Prefer path-style download: GET /api/v1/download/:slug
	body, status, err := c.request(ctx, http.MethodGet, c.apiURL("download/"+url.PathEscape(slug), query), c.maxArchive+1)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound || status == http.StatusMethodNotAllowed {
		// Fallback: GET /api/v1/download?slug=
		values := url.Values{}
		values.Set("slug", slug)
		for k, vs := range query {
			for _, v := range vs {
				values.Set(k, v)
			}
		}
		body, status, err = c.request(ctx, http.MethodGet, c.apiURL("download", values), c.maxArchive+1)
		if err != nil {
			return nil, err
		}
	}
	return c.decodeDownload(body, status)
}

func downloadQuery(version, tag string) url.Values {
	values := url.Values{}
	if version = strings.TrimSpace(version); version != "" {
		values.Set("version", version)
	}
	if tag = strings.TrimSpace(tag); tag != "" {
		values.Set("tag", tag)
	}
	return values
}

func (c *Client) decodeDownload(body []byte, status int) ([]byte, error) {
	if status == http.StatusNotFound {
		return nil, ErrSkillNotFound
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("skill download failed with status %d: %s", status, truncateBody(body))
	}
	if int64(len(body)) > c.maxArchive {
		return nil, fmt.Errorf("skill archive exceeds %d bytes", c.maxArchive)
	}
	if len(body) == 0 {
		return nil, ErrSkillArchiveEmpty
	}
	return body, nil
}

func (c *Client) applySafetyFilter(values url.Values) {
	if c.nonSuspiciousOnly {
		values.Set("nonSuspiciousOnly", "true")
	}
}

func (c *Client) getJSON(ctx context.Context, endpoint string, out any) error {
	return c.getJSONWithNotFound(ctx, endpoint, out, ErrSkillNotFound)
}

func (c *Client) getCatalogJSON(ctx context.Context, endpoint string, out any) error {
	return c.getJSONWithNotFound(ctx, endpoint, out, ErrRegistryUnavailable)
}

func (c *Client) getJSONWithNotFound(ctx context.Context, endpoint string, out any, notFound error) error {
	body, status, err := c.request(ctx, http.MethodGet, endpoint, c.maxJSON+1)
	if err != nil {
		return err
	}
	if int64(len(body)) > c.maxJSON {
		return fmt.Errorf("clawhub response exceeds %d bytes", c.maxJSON)
	}
	if status == http.StatusNotFound {
		if errors.Is(notFound, ErrRegistryUnavailable) {
			return fmt.Errorf("%w: GET %s", notFound, endpoint)
		}
		return notFound
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("clawhub request failed with status %d: %s", status, truncateBody(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode clawhub response: %w", err)
	}
	return nil
}

func (c *Client) request(ctx context.Context, method, endpoint string, maxBody int64) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create clawhub request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "*/*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("clawhub request %s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if maxBody > 0 {
		reader = io.LimitReader(resp.Body, maxBody)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read clawhub response: %w", err)
	}
	return body, resp.StatusCode, nil
}

func normalizeSlug(slug string) string {
	return strings.TrimSpace(slug)
}

func truncateBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 256 {
		return text[:256] + "..."
	}
	return text
}
