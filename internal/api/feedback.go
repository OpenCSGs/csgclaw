package api

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"csgclaw/internal/auth"
	"csgclaw/internal/feedback"
	"csgclaw/internal/upgrade"
	"csgclaw/internal/version"
)

func (h *Handler) handleFeedback(w http.ResponseWriter, r *http.Request) {
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) && !feedbackBrowserRequestAllowed(r) {
		writeCodedAPIError(w, http.StatusUnauthorized, "feedback_access_denied", "Feedback access requires a local same-origin browser or a valid server credential")
		return
	}
	record, found, err := auth.Default().Store.Load()
	status := record.Status()
	if err != nil || !found || !status.Authenticated {
		writeCodedAPIError(w, http.StatusUnauthorized, "feedback_login_required", "OpenCSG sign-in is required")
		return
	}
	environment := openCSGEnvironmentFromStatus(status)
	client := feedback.Client{BaseURL: environment.CSGHubBaseURL, Token: record.Tokens.AccessToken}
	r.Body = http.MaxBytesReader(w, r.Body, feedback.MaxRequestBytes)
	if parseErr := r.ParseMultipartForm(2 << 20); parseErr != nil {
		writeCodedAPIError(w, http.StatusBadRequest, "feedback_invalid", "Invalid feedback upload")
		return
	}
	defer r.MultipartForm.RemoveAll()
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
			writeCodedAPIError(w, http.StatusBadRequest, "feedback_invalid", "Unable to read image")
			return
		}
		data, readErr := io.ReadAll(io.LimitReader(source, feedback.MaxImageBytes+1))
		source.Close()
		image, parseErr := feedback.ParseImage(data)
		if readErr != nil || parseErr != nil {
			writeCodedAPIError(w, http.StatusBadRequest, "feedback_invalid", "Invalid image format or size")
			return
		}
		images = append(images, image)
	}
	archives := make([]feedback.Archive, 0, len(archiveFiles))
	for _, file := range archiveFiles {
		source, openErr := file.Open()
		if openErr != nil {
			writeCodedAPIError(w, http.StatusBadRequest, "feedback_invalid", "Unable to read archive")
			return
		}
		data, readErr := io.ReadAll(io.LimitReader(source, feedback.MaxArchiveBytes+1))
		source.Close()
		archive, parseErr := feedback.ParseArchive(data)
		if readErr != nil || parseErr != nil {
			writeCodedAPIError(w, http.StatusBadRequest, "feedback_invalid", "Invalid archive format or size")
			return
		}
		archives = append(archives, archive)
	}
	currentVersion := version.Current()
	result, err := client.Submit(r.Context(), feedback.Submission{Description: description, UserID: status.UserUUID, UserName: status.UserID,
		SiteURL: environment.OpenCSGBaseURL, Version: currentVersion, Channel: string(upgrade.InferChannelFromVersion(currentVersion)), Images: images, Archives: archives})
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
			case 400, 413, 422:
				statusCode = http.StatusBadRequest
				code = "feedback_invalid"
			}
		}
		slog.LogAttrs(r.Context(), slog.LevelWarn, "feedback request failed", attrs...)
		writeCodedAPIError(w, statusCode, code, "Unable to complete feedback request")
		return
	}
	writeJSON(w, http.StatusOK, result)
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
