package modelprovider

import (
	"bytes"
	"context"
	"csgclaw/internal/modelcap"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

type VideoGenerationConfig struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
}

func CloneVideoGeneration(in *VideoGenerationConfig) *VideoGenerationConfig {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

type VideoGenerationOptions struct {
	Size      string                        `json:"size,omitempty"`
	Seconds   int                           `json:"seconds,omitempty"`
	RequestID string                        `json:"-"`
	ResumeID  string                        `json:"-"`
	Progress  func(VideoGenerationProgress) `json:"-"`
}

type VideoGenerationProgress struct {
	ID        string
	Status    string
	UpdatedAt time.Time
}

// ResolveVideoGenerationOptions applies provider-advertised defaults and
// rejects guessed values before a generation request reaches the provider.
func ResolveVideoGenerationOptions(options VideoGenerationOptions, capabilities modelcap.VideoGeneration) (VideoGenerationOptions, error) {
	capabilities = capabilities.Normalized()
	options.Size = strings.TrimSpace(options.Size)
	if len(capabilities.Sizes) > 0 {
		if options.Size == "" {
			options.Size = capabilities.Sizes[0]
		} else {
			matched := ""
			for _, size := range capabilities.Sizes {
				if strings.EqualFold(options.Size, size) {
					matched = size
					break
				}
			}
			if matched == "" {
				return options, &VideoGenerationError{Code: "invalid_video_size", Message: fmt.Sprintf("The selected video model supports size values %s; %q is not supported.", strings.Join(capabilities.Sizes, ", "), options.Size)}
			}
			options.Size = matched
		}
	}
	if len(capabilities.Seconds) > 0 {
		if options.Seconds == 0 {
			options.Seconds = capabilities.Seconds[0]
		} else if !slices.Contains(capabilities.Seconds, options.Seconds) {
			values := make([]string, 0, len(capabilities.Seconds))
			for _, seconds := range capabilities.Seconds {
				values = append(values, strconv.Itoa(seconds))
			}
			return options, &VideoGenerationError{Code: "invalid_video_seconds", Message: fmt.Sprintf("The selected video model supports duration values %s seconds; %d seconds is not supported.", strings.Join(values, ", "), options.Seconds)}
		}
	}
	return options, nil
}

type GeneratedVideo struct {
	Content   io.ReadCloser
	SizeBytes int64
	MediaType string
}

type VideoGenerationError struct {
	HTTPStatus int    `json:"http_status,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
	Stage      string `json:"stage,omitempty"`
}

func (e *VideoGenerationError) Error() string { return e.Message }

const MaxGeneratedVideoBytes = 256 << 20

var videoPollInterval = 2 * time.Second
var videoRequestTimeout = 30 * time.Second
var videoRetryLimit = 5
var videoRetryBaseDelay = time.Second

// GenerateVideo implements AIGateway's normalized OpenAI-compatible async API.
func GenerateVideo(ctx context.Context, client *http.Client, baseURL, key string, headers map[string]string, model, prompt string, options VideoGenerationOptions) (GeneratedVideo, error) {
	var result GeneratedVideo
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	var created struct{ ID, Status string }
	created.ID = strings.TrimSpace(options.ResumeID)
	reportVideoProgress(options.Progress, created.ID, "submitting")
	if created.ID == "" {
		requestBody := map[string]any{"model": model, "prompt": prompt}
		if options.Size != "" {
			requestBody["size"] = options.Size
		}
		if options.Seconds > 0 {
			requestBody["seconds"] = options.Seconds
		}
		body, err := json.Marshal(requestBody)
		if err != nil {
			return result, err
		}
		if err := videoJSON(ctx, client, http.MethodPost, baseURL+"/videos", key, headers, body, options.RequestID, &created); err != nil {
			return result, err
		}
	} else {
		created.Status = ""
	}
	if strings.TrimSpace(created.ID) == "" {
		return result, &VideoGenerationError{Code: "upstream_response_invalid", Message: "The video service did not return a video ID."}
	}
	reportVideoProgress(options.Progress, created.ID, created.Status)

	status := created.Status
	for status != "completed" {
		switch status {
		case "failed", "cancelled":
			return result, &VideoGenerationError{Code: "generation_" + status, Message: "The video service could not complete the request."}
		case "queued", "in_progress", "":
		default:
			return result, &VideoGenerationError{Code: "upstream_response_invalid", Message: "The video service returned an unknown status."}
		}
		timer := time.NewTimer(videoPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return result, ctx.Err()
		case <-timer.C:
		}
		var state struct {
			Status string                          `json:"status"`
			Error  *struct{ Code, Message string } `json:"error"`
		}
		var pollErr error
		for attempt := 0; attempt < videoRetryLimit; attempt++ {
			pollErr = videoJSON(ctx, client, http.MethodGet, baseURL+"/videos/"+created.ID, key, headers, nil, "", &state)
			if pollErr == nil || !retryableVideoError(pollErr) {
				break
			}
			if err := waitVideoRetry(ctx, attempt); err != nil {
				return result, err
			}
		}
		if pollErr != nil {
			return result, pollErr
		}
		status = state.Status
		reportVideoProgress(options.Progress, created.ID, status)
		if status == "failed" && state.Error != nil {
			return result, &VideoGenerationError{Code: state.Error.Code, Message: state.Error.Message}
		}
	}
	reportVideoProgress(options.Progress, created.ID, "downloading")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/videos/"+created.ID+"/content", nil)
	if err != nil {
		return result, err
	}
	setVideoHeaders(req, key, headers)
	resp, err := client.Do(req)
	if err != nil {
		return result, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return result, videoHTTPError(resp)
	}
	sizeBytes, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if err != nil || sizeBytes <= 0 {
		content, detectedSize, spoolErr := spoolVideoContent(resp.Body)
		if spoolErr != nil {
			return result, spoolErr
		}
		resp.Body = content
		sizeBytes = detectedSize
	}
	if sizeBytes > MaxGeneratedVideoBytes {
		resp.Body.Close()
		return result, &VideoGenerationError{Code: "video_too_large", Message: "The generated video exceeds the supported size limit."}
	}
	result.Content = resp.Body
	result.SizeBytes = sizeBytes
	result.MediaType = strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if result.MediaType == "" || result.MediaType == "application/octet-stream" {
		result.MediaType = "video/mp4"
	}
	if !strings.HasPrefix(result.MediaType, "video/") {
		resp.Body.Close()
		return GeneratedVideo{}, &VideoGenerationError{Code: "upstream_response_invalid", Message: "The video service returned non-video content."}
	}
	return result, nil
}

type temporaryVideoContent struct {
	*os.File
	path string
}

func (content *temporaryVideoContent) Close() error {
	closeErr := content.File.Close()
	removeErr := os.Remove(content.path)
	return errors.Join(closeErr, removeErr)
}

func spoolVideoContent(source io.ReadCloser) (io.ReadCloser, int64, error) {
	defer source.Close()
	temporary, err := os.CreateTemp("", "csgclaw-video-download-")
	if err != nil {
		return nil, 0, err
	}
	content := &temporaryVideoContent{File: temporary, path: temporary.Name()}
	cleanup := func() {
		_ = content.Close()
	}
	size, err := io.Copy(temporary, io.LimitReader(source, MaxGeneratedVideoBytes+1))
	if err != nil {
		cleanup()
		return nil, 0, err
	}
	if size == 0 {
		cleanup()
		return nil, 0, &VideoGenerationError{Code: "upstream_response_invalid", Message: "The video service returned empty content."}
	}
	if size > MaxGeneratedVideoBytes {
		cleanup()
		return nil, 0, &VideoGenerationError{Code: "video_too_large", Message: "The generated video exceeds the supported size limit."}
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, 0, err
	}
	return content, size, nil
}

func reportVideoProgress(callback func(VideoGenerationProgress), id, status string) {
	if callback == nil {
		return
	}
	callback(VideoGenerationProgress{ID: strings.TrimSpace(id), Status: strings.TrimSpace(status), UpdatedAt: time.Now().UTC()})
}

func videoJSON(ctx context.Context, client *http.Client, method, url, key string, headers map[string]string, body []byte, requestID string, out any) error {
	requestCtx, cancel := context.WithTimeout(ctx, videoRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, method, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	setVideoHeaders(req, key, headers)
	if requestID != "" {
		req.Header.Set("Idempotency-Key", requestID)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return videoHTTPError(resp)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return &VideoGenerationError{HTTPStatus: resp.StatusCode, Code: "upstream_response_invalid", Message: "The video service returned an invalid response."}
	}
	return nil
}

func retryableVideoError(err error) bool {
	var providerErr *VideoGenerationError
	if errors.As(err, &providerErr) {
		return providerErr.HTTPStatus == http.StatusRequestTimeout || providerErr.HTTPStatus == http.StatusTooManyRequests || providerErr.HTTPStatus >= 500
	}
	var networkErr interface{ Temporary() bool }
	return errors.As(err, &networkErr) && networkErr.Temporary()
}

func waitVideoRetry(ctx context.Context, attempt int) error {
	delay := time.Duration(1<<min(attempt, 4)) * videoRetryBaseDelay
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func setVideoHeaders(req *http.Request, key string, headers map[string]string) {
	req.Header.Set("Accept-Encoding", "identity")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
}

func videoHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	var envelope struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	if envelope.Error.Message == "" {
		envelope.Error.Message = fmt.Sprintf("Video service returned HTTP %d.", resp.StatusCode)
	}
	return &VideoGenerationError{HTTPStatus: resp.StatusCode, Code: envelope.Error.Code, Message: envelope.Error.Message}
}
