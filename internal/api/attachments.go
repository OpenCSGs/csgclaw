package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"csgclaw/internal/im"
)

const multipartAttachmentMemory = 8 * 1024 * 1024

var errAttachmentPayloadTooLarge = errors.New("attachment payload is too large")

func parseCreateMessageHTTP(w http.ResponseWriter, r *http.Request) (createMessageRequest, error) {
	var req createMessageRequest
	uploads, err := decodeMessagePayload(w, r, &req)
	if err != nil {
		return createMessageRequest{}, err
	}
	req.Attachments = uploads
	return req, nil
}

func parseParticipantSendMessageHTTP(w http.ResponseWriter, r *http.Request) (im.ParticipantSendMessageRequest, error) {
	var req im.ParticipantSendMessageRequest
	uploads, err := decodeMessagePayload(w, r, &req)
	if err != nil {
		return im.ParticipantSendMessageRequest{}, err
	}
	req.Attachments = uploads
	return req, nil
}

func decodeMessagePayload(w http.ResponseWriter, r *http.Request, dst any) ([]im.MessageAttachmentUpload, error) {
	if !isMultipartRequest(r) {
		if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
			return nil, fmt.Errorf("decode request: %w", err)
		}
		return nil, nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, im.MaxAttachmentMessageBytes+1024*1024)
	if err := r.ParseMultipartForm(multipartAttachmentMemory); err != nil {
		return nil, fmt.Errorf("decode multipart request: %w", err)
	}
	defer r.MultipartForm.RemoveAll()
	payload := strings.TrimSpace(r.FormValue("payload"))
	if payload == "" {
		return nil, fmt.Errorf("payload is required")
	}
	if err := json.Unmarshal([]byte(payload), dst); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	uploads, err := readMultipartAttachmentUploads(r.MultipartForm)
	if err != nil {
		return nil, err
	}
	return uploads, nil
}

func isMultipartRequest(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && strings.EqualFold(mediaType, "multipart/form-data")
}

func readMultipartAttachmentUploads(form *multipart.Form) ([]im.MessageAttachmentUpload, error) {
	if form == nil || len(form.File) == 0 {
		return nil, nil
	}
	var headers []*multipart.FileHeader
	for _, field := range []string{"files", "file"} {
		headers = append(headers, form.File[field]...)
	}
	if len(headers) == 0 {
		return nil, nil
	}
	if len(headers) > im.MaxAttachmentsPerMessage {
		return nil, fmt.Errorf("too many attachments: max %d", im.MaxAttachmentsPerMessage)
	}
	uploads := make([]im.MessageAttachmentUpload, 0, len(headers))
	total := int64(0)
	for _, header := range headers {
		name, err := multipartOriginalFilename(header)
		if err != nil {
			return nil, err
		}
		data, err := readMultipartAttachment(header)
		if err != nil {
			return nil, err
		}
		total += int64(len(data))
		if total > im.MaxAttachmentMessageBytes {
			return nil, fmt.Errorf("%w: attachments exceed %d bytes per message", errAttachmentPayloadTooLarge, im.MaxAttachmentMessageBytes)
		}
		uploads = append(uploads, im.MessageAttachmentUpload{
			Name:      name,
			MediaType: header.Header.Get("Content-Type"),
			Data:      data,
		})
	}
	return uploads, nil
}

func multipartOriginalFilename(header *multipart.FileHeader) (string, error) {
	if header == nil {
		return "", fmt.Errorf("attachment is missing")
	}
	_, params, err := mime.ParseMediaType(header.Header.Get("Content-Disposition"))
	if err != nil {
		return "", fmt.Errorf("decode attachment filename: %w", err)
	}
	if name := strings.TrimSpace(params["filename"]); name != "" {
		return name, nil
	}
	return header.Filename, nil
}

func readMultipartAttachment(header *multipart.FileHeader) ([]byte, error) {
	if header == nil {
		return nil, fmt.Errorf("attachment is missing")
	}
	file, err := header.Open()
	if err != nil {
		return nil, fmt.Errorf("open attachment %q: %w", header.Filename, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, im.MaxAttachmentFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read attachment %q: %w", header.Filename, err)
	}
	if len(data) > im.MaxAttachmentFileBytes {
		return nil, fmt.Errorf("%w: attachment %q exceeds %d bytes", errAttachmentPayloadTooLarge, header.Filename, im.MaxAttachmentFileBytes)
	}
	return data, nil
}

func writeMessagePayloadError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	var maxBytesError *http.MaxBytesError
	if errors.Is(err, errAttachmentPayloadTooLarge) || errors.Is(err, multipart.ErrMessageTooLarge) || errors.As(err, &maxBytesError) {
		status = http.StatusRequestEntityTooLarge
	}
	http.Error(w, err.Error(), status)
}

func (h *Handler) handleAttachmentByID(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}
	id := pathValue(r, "id")
	if strings.TrimSpace(id) == "" {
		http.NotFound(w, r)
		return
	}
	file, err := h.im.AttachmentFile(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) && !file.AuthorizesDownloadToken(r.URL.Query().Get("token")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", file.MediaType)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	disposition := "attachment"
	if strings.HasPrefix(strings.ToLower(file.MediaType), "image/") {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": file.SafeName}))
	http.ServeFile(w, r, file.Path)
}
