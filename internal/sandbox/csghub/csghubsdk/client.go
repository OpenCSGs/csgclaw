package csghubsdk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// DefaultTimeout applies to lifecycle JSON calls when none is set on the client.
const DefaultTimeout = 60 * time.Second

// DefaultExecuteTimeout applies to stream-execute calls.
const DefaultExecuteTimeout = 30 * time.Minute

// DefaultBatchTimeout applies to batched upload/download calls.
const DefaultBatchTimeout = 2 * time.Minute

// Client talks to CSGHub Sandbox HTTP APIs. Zero value is not usable; build
// one with New.
type Client struct {
	cfg    Config
	http   *http.Client
	logger Logger
}

// Logger is an optional structured logger; nil loggers are treated as no-ops.
type Logger interface {
	Infof(format string, args ...any)
	Errorf(format string, args ...any)
}

// Option configures a Client constructed via New.
type Option func(*Client)

// WithHTTPClient injects a custom *http.Client (for example, to reuse a
// connection pool or wire in a test transport). nil falls back to the default.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// WithLogger installs a structured logger for lifecycle span logs.
func WithLogger(l Logger) Option {
	return func(c *Client) {
		c.logger = l
	}
}

// New constructs a Client. BaseURL must be non-empty; Token, AIGatewayURL and
// other fields are optional.
func New(cfg Config, opts ...Option) *Client {
	c := &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Config returns a copy of the client configuration.
func (c *Client) Config() Config { return c.cfg }

// --- Lifecycle APIs ---------------------------------------------------------

// Create provisions a new sandbox. Expects HTTP 201 with a Response body.
func (c *Client) Create(ctx context.Context, spec CreateRequest) (*Response, error) {
	body, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("sandbox: marshal create body: %w", err)
	}
	raw, err := c.rawJSON(ctx, rawRequest{
		span:    "csg-sandbox-create",
		method:  http.MethodPost,
		url:     c.cfg.apiSandboxesRoot(),
		body:    body,
		success: []int{http.StatusCreated, http.StatusOK},
		trace:   spec.SandboxName,
	})
	if err != nil {
		return nil, err
	}
	return parseResponse(raw)
}

// Get fetches the current spec and state for a sandbox by name.
func (c *Client) Get(ctx context.Context, sandboxID string) (*Response, error) {
	raw, err := c.rawJSON(ctx, rawRequest{
		span:    "csg-sandbox-get",
		method:  http.MethodGet,
		url:     c.cfg.apiSandboxesRoot() + "/" + sandboxID,
		success: []int{http.StatusOK},
		trace:   sandboxID,
	})
	if err != nil {
		return nil, err
	}
	return parseResponse(raw)
}

// UpdateConfig patches mutable sandbox fields (image / env / volumes / port / timeout).
func (c *Client) UpdateConfig(ctx context.Context, sandboxID string, patch UpdateConfigRequest) (*Response, error) {
	body, err := json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("sandbox: marshal update body: %w", err)
	}
	raw, err := c.rawJSON(ctx, rawRequest{
		span:    "csg-sandbox-update-config",
		method:  http.MethodPatch,
		url:     c.cfg.apiSandboxesRoot() + "/" + sandboxID,
		body:    body,
		success: []int{http.StatusOK},
		trace:   sandboxID,
	})
	if err != nil {
		return nil, err
	}
	return parseResponse(raw)
}

// Apply mirrors pycsghub.apply_sandbox: treats a CreateRequest as a desired
// state and patches the existing sandbox by SandboxName.
func (c *Client) Apply(ctx context.Context, spec CreateRequest) (*Response, error) {
	patch := UpdateConfigRequest{
		ResourceID:   spec.ResourceID,
		Image:        spec.Image,
		Environments: spec.Environments,
		Volumes:      spec.Volumes,
		Port:         spec.Port,
		Timeout:      spec.Timeout,
	}
	return c.UpdateConfig(ctx, spec.SandboxName, patch)
}

// Start transitions a sandbox to the running state.
func (c *Client) Start(ctx context.Context, sandboxID string) (*Response, error) {
	raw, err := c.rawJSON(ctx, rawRequest{
		span:    "csg-sandbox-start",
		method:  http.MethodPut,
		url:     c.cfg.apiSandboxesRoot() + "/" + sandboxID + "/status/start",
		success: []int{http.StatusOK},
		trace:   sandboxID,
	})
	if err != nil {
		return nil, err
	}
	return parseResponse(raw)
}

// Stop tears down a sandbox (primary teardown hook). Starhub has no DELETE.
func (c *Client) Stop(ctx context.Context, sandboxID string) error {
	_, err := c.rawJSON(ctx, rawRequest{
		span:    "csg-sandbox-stop",
		method:  http.MethodPut,
		url:     c.cfg.apiSandboxesRoot() + "/" + sandboxID + "/status/stop",
		success: []int{http.StatusOK},
		trace:   sandboxID,
	})
	return err
}

// Delete aliases Stop: parity with pycsghub.delete_sandbox semantics.
func (c *Client) Delete(ctx context.Context, sandboxID string) error { return c.Stop(ctx, sandboxID) }

// --- Runtime (gateway) APIs -------------------------------------------------

// StreamExecute streams stdout/stderr lines from a running sandbox.
//
// Mirrors pycsghub.stream_execute_command: each line is delivered via the
// emit callback. On HTTP or transport failure, the function emits a single
// line prefixed "ERROR: ..." and returns nil (matching Python's behavior of
// not raising). If emit returns an error, streaming stops and that error is
// returned to the caller.
func (c *Client) StreamExecute(ctx context.Context, sandboxName, command string, emit func(line string) error) error {
	if emit == nil {
		return fmt.Errorf("sandbox: emit callback is required")
	}
	url := c.cfg.aigatewayBase() + "/v1/sandboxes/" + sandboxName + "/execute?port=8888"
	payload := map[string]any{"command": command}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("sandbox: marshal execute payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sandbox: build execute request: %w", err)
	}
	c.setJSONHeaders(req)

	resp, err := c.doWithTimeout(req, DefaultExecuteTimeout)
	if err != nil {
		return emit(fmt.Sprintf("ERROR: Request failed: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := readErrorBody(resp)
		return emit(fmt.Sprintf("ERROR: HTTP %d: %s", resp.StatusCode, detail))
	}

	scanner := bufio.NewScanner(resp.Body)
	// Accept long lines (1 MiB).
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if err := emit(line); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// RuntimeHealth probes the sandbox-runtime root through the gateway.
func (c *Client) RuntimeHealth(ctx context.Context, sandboxName string) error {
	url := c.cfg.aigatewayBase() + "/v1/sandboxes/" + sandboxName + "/?port=8888"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("sandbox: build health request: %w", err)
	}
	c.setAuthHeader(req)
	resp, err := c.doWithTimeout(req, DefaultTimeout)
	if err != nil {
		return &TransportError{URL: url, Cause: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &HTTPError{StatusCode: resp.StatusCode, URL: url, Detail: readErrorBody(resp)}
	}
	return nil
}

// UploadFile pushes a single file via multipart/form-data field "file".
func (c *Client) UploadFile(ctx context.Context, sandboxName, fileName string, content []byte) (*UploadFileResponse, error) {
	url := c.cfg.aigatewayBase() + "/v1/sandboxes/" + sandboxName + "/upload?port=8888"

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", fileName)
	if err != nil {
		return nil, fmt.Errorf("sandbox: create form file: %w", err)
	}
	if _, err := fw.Write(content); err != nil {
		return nil, fmt.Errorf("sandbox: write form file: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("sandbox: close multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, fmt.Errorf("sandbox: build upload request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.doWithTimeout(req, DefaultTimeout)
	if err != nil {
		return nil, &TransportError{URL: url, Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{StatusCode: resp.StatusCode, URL: url, Detail: readErrorBody(resp)}
	}
	out := new(UploadFileResponse)
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, &ParseError{Detail: "upload response is not valid JSON", Cause: err}
	}
	return out, nil
}

// FilePayload is a single file entry for UploadFilesBatch.
type FilePayload struct {
	Path    string
	Content []byte
}

// UploadFilesBatch sends many files in one JSON POST with base64 content.
// Returns the server's raw response as a decoded map.
func (c *Client) UploadFilesBatch(ctx context.Context, sandboxName string, files []FilePayload) (map[string]any, error) {
	url := c.cfg.aigatewayBase() + "/v1/sandboxes/" + sandboxName + "/upload-files?port=8888"
	entries := make([]map[string]string, len(files))
	for i, f := range files {
		entries[i] = map[string]string{
			"path":    f.Path,
			"content": base64.StdEncoding.EncodeToString(f.Content),
		}
	}
	payload := map[string]any{"sandbox_name": sandboxName, "files": entries}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("sandbox: marshal upload-files body: %w", err)
	}
	raw, err := c.rawJSONWithTimeout(ctx, rawRequest{
		span:    "csg-sandbox-upload-files-batch",
		method:  http.MethodPost,
		url:     url,
		body:    body,
		success: []int{http.StatusOK},
		trace:   sandboxName,
	}, DefaultBatchTimeout)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, &ParseError{Cause: err}
	}
	return out, nil
}

// DownloadFilesBatch requests many files in one JSON POST. The response shape
// is server-defined; callers inspect the returned map.
func (c *Client) DownloadFilesBatch(ctx context.Context, sandboxName string, paths []string) (map[string]any, error) {
	url := c.cfg.aigatewayBase() + "/v1/sandboxes/" + sandboxName + "/download-files?port=8888"
	entries := make([]map[string]string, len(paths))
	for i, p := range paths {
		entries[i] = map[string]string{"path": p}
	}
	payload := map[string]any{"sandbox_name": sandboxName, "files": entries}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("sandbox: marshal download-files body: %w", err)
	}
	raw, err := c.rawJSONWithTimeout(ctx, rawRequest{
		span:    "csg-sandbox-download-files-batch",
		method:  http.MethodPost,
		url:     url,
		body:    body,
		success: []int{http.StatusOK},
		trace:   sandboxName,
	}, DefaultBatchTimeout)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, &ParseError{Cause: err}
	}
	return out, nil
}

// --- internals --------------------------------------------------------------

type rawRequest struct {
	span    string
	method  string
	url     string
	body    []byte
	success []int
	trace   string
}

func (c *Client) rawJSON(ctx context.Context, r rawRequest) ([]byte, error) {
	return c.rawJSONWithTimeout(ctx, r, DefaultTimeout)
}

func (c *Client) rawJSONWithTimeout(ctx context.Context, r rawRequest, timeout time.Duration) ([]byte, error) {
	if c.logger != nil {
		c.logger.Infof("[sandbox] %s trace=%s url=%s", r.span, r.trace, r.url)
	}

	var body io.Reader
	if len(r.body) > 0 {
		body = bytes.NewReader(r.body)
	}
	req, err := http.NewRequestWithContext(ctx, r.method, r.url, body)
	if err != nil {
		return nil, fmt.Errorf("sandbox: build %s request: %w", r.span, err)
	}
	c.setJSONHeaders(req)

	resp, err := c.doWithTimeout(req, timeout)
	if err != nil {
		return nil, &TransportError{URL: r.url, Cause: err}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &TransportError{URL: r.url, Cause: err}
	}

	if !containsInt(r.success, resp.StatusCode) {
		detail := decodeErrorBody(raw)
		if c.logger != nil {
			c.logger.Errorf("[sandbox] %s HTTP %d trace=%s url=%s body=%s",
				r.span, resp.StatusCode, r.trace, r.url, truncate(detail, 2000))
		}
		return nil, &HTTPError{StatusCode: resp.StatusCode, URL: r.url, Detail: detail}
	}
	return raw, nil
}

func (c *Client) doWithTimeout(req *http.Request, timeout time.Duration) (*http.Response, error) {
	hc := c.http
	if hc == nil {
		hc = &http.Client{Timeout: timeout}
	} else if hc.Timeout == 0 && timeout > 0 {
		hc = &http.Client{
			Transport:     hc.Transport,
			CheckRedirect: hc.CheckRedirect,
			Jar:           hc.Jar,
			Timeout:       timeout,
		}
	}
	return hc.Do(req)
}

func (c *Client) setJSONHeaders(req *http.Request) {
	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}

func (c *Client) setAuthHeader(req *http.Request) {
	if token := strings.TrimSpace(c.cfg.Token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func parseResponse(raw []byte) (*Response, error) {
	if len(raw) == 0 {
		return nil, &ParseError{Detail: "empty response body"}
	}
	var bodyMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &bodyMap); err != nil {
		return nil, &ParseError{Cause: err}
	}
	// Accept bare {spec, state} or httpbase envelope {data: {...}}.
	if _, hasSpec := bodyMap["spec"]; hasSpec {
		var out Response
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, &ParseError{Cause: err}
		}
		return &out, nil
	}
	if inner, ok := bodyMap["data"]; ok {
		if len(inner) == 0 || string(inner) == "null" {
			return nil, &ParseError{Detail: "sandbox API response has data: null"}
		}
		var out Response
		if err := json.Unmarshal(inner, &out); err != nil {
			return nil, &ParseError{Cause: err}
		}
		return &out, nil
	}
	// Fallback attempt.
	var out Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, &ParseError{Cause: err}
	}
	return &out, nil
}

func decodeErrorBody(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var body errorBody
	if err := json.Unmarshal(raw, &body); err == nil {
		line := body.line()
		if line != "" && line != "unknown error" {
			return line
		}
	}
	return truncate(string(raw), 500)
}

func readErrorBody(resp *http.Response) string {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return decodeErrorBody(raw)
}

func containsInt(set []int, v int) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
