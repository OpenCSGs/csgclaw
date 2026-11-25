package feedback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"csgclaw/internal/diagnostics"
)

const Endpoint = "/api/v1/csgbot/user-feedback"

// Keep original image uploads compatible and request extension-preserving URLs for archive downloads.
const (
	UploadEndpoint        = "/internal_api/upload?max_size=30m"
	ArchiveUploadEndpoint = "/internal_api/upload_with_ext?max_size=30m"
)

type Submission struct {
	Description string
	SiteURL     string
	Version     string
	Channel     string
	UserID      string
	UserName    string
	Images      []Image
	Archives    []Archive
}

type Result struct {
	Success  bool   `json:"success"`
	IssueID  int    `json:"issue_id,omitempty"`
	IssueURL string `json:"issue_url,omitempty"`
	Message  string `json:"message,omitempty"`
}

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

const maxErrorResponseBytes = 4096

var responseURLWithQuery = regexp.MustCompile(`(?i)(https?:(?:/|\\/){2}[^?#\s"'<>]+)[?#][^\s"'<>]*`)

type HTTPError struct {
	Status                int
	ResponseBody          string
	ResponseBodyTruncated bool
	ResponseHeaders       http.Header
}

func (e *HTTPError) Error() string { return fmt.Sprintf("feedback service returned HTTP %d", e.Status) }

// UploadError means attachment processing stopped before the issue request.
type UploadError struct {
	Kind string
	Err  error
}

func (e *UploadError) Error() string { return fmt.Sprintf("%s upload failed: %v", e.Kind, e.Err) }
func (e *UploadError) Unwrap() error { return e.Err }

func (c *Client) Submit(ctx context.Context, submission Submission) (Result, error) {
	urls := make([]string, 0, len(submission.Images))
	for index, image := range submission.Images {
		imageURL, err := c.upload(ctx, submission.SiteURL, UploadEndpoint, index, image.Data, image.Extension)
		if err != nil {
			return Result{}, &UploadError{Kind: "image", Err: err}
		}
		urls = append(urls, imageURL)
	}
	// Place links before user Markdown so an unfinished code fence cannot hide them.
	var description strings.Builder
	if len(submission.Archives) > 0 {
		for index, archive := range submission.Archives {
			archiveURL, err := c.upload(ctx, submission.SiteURL, ArchiveUploadEndpoint, len(submission.Images)+index, archive.Data, archive.Extension)
			if err != nil {
				return Result{}, &UploadError{Kind: "archive", Err: err}
			}
			fmt.Fprintf(&description, "- [feedback-%d.%s](<%s>)\n", len(submission.Images)+index, archive.Extension, archiveURL)
		}
		description.WriteString("\n")
	}
	description.WriteString(submission.Description)
	body, err := json.Marshal(map[string]any{
		"problem_module": "feature", "problem_description": description.String(),
		"screenshot_urls": urls, "user_id": submission.UserID, "user_name": submission.UserName,
		"csgclaw": map[string]string{"site_url": submission.SiteURL, "version": submission.Version, "channel": submission.Channel},
	})
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+Endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	var result Result
	err = c.request(req, &result)
	return result, err
}

func (c *Client) upload(ctx context.Context, site, endpoint string, index int, data []byte, extension string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", fmt.Sprintf("feedback-%d.%s", index, extension))
	if err != nil {
		return "", err
	}
	if _, err = part.Write(data); err != nil {
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(site, "/")+endpoint, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	// The portal forwards this credential when its storage proxy is enabled.
	req.AddCookie(&http.Cookie{Name: "user_token", Value: c.Token})
	var result struct {
		URL string `json:"url"`
	}
	if err = c.request(req, &result); err != nil {
		return "", err
	}
	parsed, err := url.Parse(result.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", fmt.Errorf("feedback upload returned an invalid URL")
	}
	return strings.NewReplacer("<", "%3C", ">", "%3E").Replace(parsed.String()), nil
}

func (c *Client) request(req *http.Request, result any) error {
	req.Header.Set("Accept", "application/json")
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: time.Minute, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Read beyond the output limit far enough to redact an echoed token crossing it.
		body, _ := io.ReadAll(io.LimitReader(response.Body, int64(maxErrorResponseBytes+max(1, len(c.Token)-1))))
		truncated := len(body) > maxErrorResponseBytes
		if c.Token != "" {
			body = bytes.ReplaceAll(body, []byte(c.Token), []byte("[redacted]"))
		}
		if len(body) > maxErrorResponseBytes {
			body = body[:maxErrorResponseBytes]
		}
		message := string(body)
		message = responseURLWithQuery.ReplaceAllString(message, "${1}?[redacted]")
		return &HTTPError{
			Status:                response.StatusCode,
			ResponseBody:          diagnostics.ProtectText(strings.TrimSpace(message)),
			ResponseBodyTruncated: truncated,
			ResponseHeaders:       c.responseHeadersForLog(response.Header),
		}
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result)
}

func (c *Client) responseHeadersForLog(headers http.Header) http.Header {
	out := make(http.Header, len(headers))
	for name, values := range headers {
		switch strings.ToLower(name) {
		case "content-type", "content-length", "date", "server", "via", "retry-after",
			"x-request-id", "request-id", "x-correlation-id", "correlation-id", "x-apisix-request-id",
			"x-trace-id", "traceparent", "x-amzn-requestid", "x-amz-request-id", "cf-ray":
			for _, value := range values {
				if c.Token != "" {
					value = strings.ReplaceAll(value, c.Token, "[redacted]")
				}
				out.Add(name, diagnostics.ProtectText(value))
			}
		default:
			out.Set(name, "[redacted]")
		}
	}
	return out
}
