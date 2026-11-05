package api

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"csgclaw/internal/im"
)

func writeSessionInputError(w http.ResponseWriter, err error) {
	param := "input"
	var validationErr *agentSessionRequestValidationError
	if errors.As(err, &validationErr) {
		param = validationErr.param
	}
	status := http.StatusBadRequest
	var maxBytesError *http.MaxBytesError
	if errors.Is(err, errAttachmentPayloadTooLarge) || errors.Is(err, multipart.ErrMessageTooLarge) || errors.As(err, &maxBytesError) {
		status = http.StatusRequestEntityTooLarge
	}
	writeAgentSessionError(w, status, "invalid_request", err.Error(), param)
}

func (h *Handler) authorizeSessionFiles(w http.ResponseWriter, r *http.Request) bool {
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
		writeAgentSessionError(w, http.StatusUnauthorized, "unauthorized", "valid server authorization is required", nil)
		return false
	}
	return true
}

func (h *Handler) uploadAgentSessionAttachments(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeSessionFiles(w, r) {
		return
	}
	sessionID := pathValue(r, "session_id")
	if !agentSessionIDPattern.MatchString(sessionID) {
		writeAgentSessionError(w, http.StatusBadRequest, "invalid_session_id", "session_id must be 1-128 path-safe ASCII characters", "session_id")
		return
	}
	selected, ok := h.sessionRequestAgent(w, r)
	if !ok {
		return
	}
	if !isMultipartRequest(r) {
		writeAgentSessionError(w, http.StatusUnsupportedMediaType, "invalid_request", "upload files as multipart/form-data", "files")
		return
	}
	defer cleanupMessagePayload(r)
	r.Body = http.MaxBytesReader(w, r.Body, im.MaxAttachmentMessageBytes+agentSessionResponseBodyLimit)
	if err := r.ParseMultipartForm(multipartAttachmentMemory); err != nil {
		writeSessionInputError(w, err)
		return
	}
	if len(r.MultipartForm.Value) != 0 {
		writeAgentSessionError(w, http.StatusBadRequest, "invalid_request", "expected only files parts", "files")
		return
	}
	files, err := sessionMultipartFiles(r.MultipartForm, "files")
	if err != nil {
		writeSessionInputError(w, err)
		return
	}
	var sources []im.AttachmentSource
	for _, file := range files {
		reader, err := file.open()
		if err != nil {
			writeAgentSessionError(w, http.StatusServiceUnavailable, "attachment_unavailable", "uploaded file could not be read", "files")
			return
		}
		defer reader.Close()
		sources = append(sources, im.AttachmentSource{Name: file.Name, MediaType: file.MediaType, SizeBytes: file.SizeBytes, Reader: reader})
	}
	attachments, err := h.saveSessionAttachmentSources(selected, sessionID, newAgentSessionAPIID("upload"), sources)
	if err != nil {
		writeAgentSessionError(w, http.StatusServiceUnavailable, "attachment_unavailable", "uploaded files could not be saved", "files")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"attachments": attachments})
}

func validateSessionAttachmentIDs(ids []string, newFiles int) error {
	if len(ids)+newFiles > im.MaxAttachmentsPerMessage {
		return fmt.Errorf("too many input files: max %d", im.MaxAttachmentsPerMessage)
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || id != strings.TrimSpace(id) || seen[id] {
			return fmt.Errorf("attachment_ids must contain distinct nonempty IDs")
		}
		seen[id] = true
	}
	return nil
}

func (h *Handler) sessionInputReferences(agentID, sessionID string, ids []string) ([]agentSessionInputFile, []im.MessageAttachment, error) {
	var files []agentSessionInputFile
	var attachments []im.MessageAttachment
	for _, id := range ids {
		file, err := h.im.SessionAttachmentFile(agentID, sessionID, id)
		if err != nil {
			return nil, nil, err
		}
		att := file.Attachment
		files = append(files, agentSessionInputFile{
			Name: att.Name, MediaType: att.MediaType, SizeBytes: att.SizeBytes, sha256: att.SHA256,
			open: func() (io.ReadCloser, error) { return os.Open(file.Path) },
		})
		attachments = append(attachments, att)
	}
	return files, attachments, nil
}
