package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"csgclaw/internal/auth"
	"csgclaw/internal/config"
	"csgclaw/internal/feedback"
	"csgclaw/internal/upgrade"
	"csgclaw/internal/version"
)

func (h *Handler) feedbackAccount(w http.ResponseWriter, r *http.Request) (auth.Record, bool) {
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) && !feedbackBrowserRequestAllowed(r) {
		writeCodedAPIError(w, http.StatusUnauthorized, "feedback_access_denied", "Feedback access requires a local same-origin browser or a valid server credential")
		return auth.Record{}, false
	}
	record, found, err := auth.Default().Store.Load()
	status := record.Status()
	if err != nil || !found || !status.Authenticated {
		writeCodedAPIError(w, http.StatusUnauthorized, "feedback_login_required", "OpenCSG sign-in is required")
		return auth.Record{}, false
	}
	return record, true
}

func feedbackAttempts() (feedback.Attempts, error) {
	dir, err := config.DefaultDir()
	if err != nil {
		return feedback.Attempts{}, err
	}
	return feedback.Attempts{Path: filepath.Join(dir, "feedback-submissions.json")}, nil
}

func feedbackAccountKey(status auth.Status) string {
	if status.UserUUID != "" {
		return "uuid:" + status.UserUUID
	}
	if status.UserID != "" {
		return "user:" + status.UserID
	}
	return ""
}

func (h *Handler) handleFeedbackStatus(w http.ResponseWriter, r *http.Request) {
	record, ok := h.feedbackAccount(w, r)
	if !ok {
		return
	}
	status := record.Status()
	var input struct {
		RequestID string `json:"request_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input) != nil || !feedback.ValidAttemptID(input.RequestID) || feedbackAccountKey(status) == "" {
		writeCodedAPIError(w, http.StatusBadRequest, "feedback_invalid", "Invalid feedback reference")
		return
	}
	store, err := feedbackAttempts()
	if err == nil {
		var attempt feedback.Attempt
		attempt, err = store.Status(openCSGEnvironmentFromStatus(status).OpenCSGBaseURL, feedbackAccountKey(status), input.RequestID)
		if err == nil {
			writeJSON(w, http.StatusOK, attempt)
			return
		}
	}
	slog.WarnContext(r.Context(), "read feedback status failed", "error", err)
	writeCodedAPIError(w, http.StatusServiceUnavailable, "feedback_tracking_unavailable", "Feedback tracking is unavailable")
}

func (h *Handler) handleFeedback(w http.ResponseWriter, r *http.Request) {
	record, ok := h.feedbackAccount(w, r)
	if !ok {
		return
	}
	status := record.Status()
	environment := openCSGEnvironmentFromStatus(status)
	client := feedback.Client{BaseURL: environment.CSGHubBaseURL, Token: record.Tokens.AccessToken}
	r.Body = http.MaxBytesReader(w, r.Body, feedback.MaxRequestBytes)
	if parseErr := r.ParseMultipartForm(2 << 20); parseErr != nil {
		writeCodedAPIError(w, http.StatusBadRequest, "feedback_invalid", "Invalid feedback upload")
		return
	}
	defer r.MultipartForm.RemoveAll()
	requestID := r.FormValue("request_id")
	account := feedbackAccountKey(status)
	if requestID != "" && (!feedback.ValidAttemptID(requestID) || account == "") {
		writeCodedAPIError(w, http.StatusBadRequest, "feedback_invalid", "Invalid feedback reference")
		return
	}
	description := r.FormValue("problem_description")
	files := r.MultipartForm.File["images"]
	archiveFiles := r.MultipartForm.File["archives"]
	if parseErr := feedback.ValidateContent(description, len(files), len(archiveFiles)); parseErr != nil {
		writeCodedAPIError(w, http.StatusBadRequest, "feedback_invalid", parseErr.Error())
		return
	}
	images := make([]feedback.Image, 0, len(files))
	for _, file := range files {
		source, openErr := file.Open()
		if openErr != nil {
			writeFeedbackAttachmentError(w, requestID, file.Filename, "Unable to read image")
			return
		}
		data, readErr := io.ReadAll(io.LimitReader(source, feedback.MaxImageBytes+1))
		source.Close()
		image, parseErr := feedback.ParseImage(data)
		if readErr != nil || parseErr != nil {
			writeFeedbackAttachmentError(w, requestID, file.Filename, "Invalid image format or size")
			return
		}
		image.Filename = file.Filename
		images = append(images, image)
	}
	archives := make([]feedback.Archive, 0, len(archiveFiles))
	for _, file := range archiveFiles {
		source, openErr := file.Open()
		if openErr != nil {
			writeFeedbackAttachmentError(w, requestID, file.Filename, "Unable to read archive")
			return
		}
		data, readErr := io.ReadAll(io.LimitReader(source, feedback.MaxArchiveBytes+1))
		source.Close()
		archive, parseErr := feedback.ParseArchive(data)
		if readErr != nil || parseErr != nil {
			writeFeedbackAttachmentError(w, requestID, file.Filename, "Invalid archive format or size")
			return
		}
		archive.Filename = file.Filename
		archives = append(archives, archive)
	}
	ctx := r.Context()
	var store feedback.Attempts
	if requestID != "" {
		var err error
		store, err = feedbackAttempts()
		var attempt feedback.Attempt
		var reserved bool
		if err == nil {
			attempt, reserved, err = store.Begin(environment.OpenCSGBaseURL, account, requestID)
		}
		if err != nil {
			slog.WarnContext(ctx, "reserve feedback reference failed", "error", err)
			writeCodedAPIError(w, http.StatusServiceUnavailable, "feedback_tracking_unavailable", "Feedback tracking is unavailable")
			return
		}
		if !reserved {
			if attempt.Receipt != nil {
				writeJSON(w, http.StatusOK, attempt.Receipt)
			} else {
				writeCodedAPIError(w, http.StatusConflict, "feedback_unavailable", "Feedback outcome is not yet confirmed")
			}
			return
		}
		// A browser refresh must not interrupt a reserved submission.
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), time.Duration(len(images)+len(archives)+1)*time.Minute)
		defer cancel()
	}
	currentVersion := version.Current()
	result, err := client.Submit(ctx, feedback.Submission{Description: description, UserID: status.UserUUID, UserName: status.UserID,
		SiteURL: environment.OpenCSGBaseURL, Version: currentVersion, Channel: string(upgrade.InferChannelFromVersion(currentVersion)), Images: images, Archives: archives})
	finish := func(receipt *feedback.Result) {
		if requestID != "" {
			if err := store.Finish(environment.OpenCSGBaseURL, account, requestID, receipt); err != nil {
				slog.WarnContext(ctx, "record feedback outcome failed", "error", err)
			}
		}
	}
	if err != nil {
		statusCode := http.StatusBadGateway
		code := "feedback_unavailable"
		stage := "issue_submission"
		var uploadErr *feedback.UploadError
		if errors.As(err, &uploadErr) {
			code = "feedback_upload_failed"
			stage = uploadErr.Kind + "_upload"
		}
		attrs := []slog.Attr{slog.String("stage", stage), slog.Any("error", err)}
		var upstream *feedback.HTTPError
		if errors.As(err, &upstream) {
			attrs = append(attrs,
				slog.Int("upstream_status", upstream.Status),
				slog.Any("upstream_response_headers", upstream.ResponseHeaders),
				slog.String("upstream_response_body", upstream.ResponseBody),
				slog.Bool("upstream_response_body_truncated", upstream.ResponseBodyTruncated),
			)
			switch upstream.Status {
			case 401, 403:
				statusCode = http.StatusUnauthorized
				code = "feedback_login_required"
			case http.StatusRequestEntityTooLarge:
				if uploadErr != nil {
					statusCode = http.StatusRequestEntityTooLarge
					code = "feedback_upload_too_large"
				} else {
					statusCode = http.StatusBadRequest
					code = "feedback_invalid"
				}
			case http.StatusBadRequest, http.StatusUnprocessableEntity:
				statusCode = http.StatusBadRequest
				if uploadErr == nil {
					code = "feedback_invalid"
				}
			}
		}
		if code != "feedback_unavailable" {
			finish(nil)
		}
		slog.LogAttrs(ctx, slog.LevelWarn, "feedback request failed", attrs...)
		writeJSON(w, statusCode, map[string]any{
			"error": map[string]string{
				"code": code, "message": "Unable to complete feedback request", "stage": stage,
			},
		})
		return
	}
	if result.Success {
		finish(&result)
	}
	writeJSON(w, http.StatusOK, result)
}

func writeFeedbackAttachmentError(w http.ResponseWriter, requestID, fileName, message string) {
	if requestID == "" {
		writeCodedAPIError(w, http.StatusBadRequest, "feedback_invalid", message)
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": map[string]string{"code": "feedback_attachment_invalid", "message": message, "file_name": fileName},
	})
}

func feedbackBrowserRequestAllowed(r *http.Request) bool {
	if len(r.Header.Values("Authorization")) != 0 || appRequestAgentID(r) != "" || r.Header.Get("X-CSGClaw-Caller-Agent") != "" {
		return false
	}
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(peer).IsLoopback() {
		return false
	}
	origins := r.Header.Values("Origin")
	if len(origins) != 1 {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	origin, err := url.Parse(origins[0])
	// A browser Origin contains only the scheme and authority, including the port.
	if err != nil || origin.Scheme != scheme || origin.Host != r.Host || origins[0] != scheme+"://"+origin.Host {
		return false
	}
	host := origin.Hostname()
	if host != "localhost" && !net.ParseIP(host).IsLoopback() {
		return false
	}
	authority := host
	if strings.Contains(host, ":") {
		authority = "[" + host + "]"
	}
	if port := origin.Port(); port != "" {
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return false
		}
		authority = net.JoinHostPort(host, port)
	}
	return origin.Host == authority
}
