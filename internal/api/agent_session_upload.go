package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"

	"csgclaw/internal/agentengine"
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
	attachments, err := h.storeSessionUploads(r.Context(), selected, sessionID, newAgentSessionAPIID("upload"), files)
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

func (h *Handler) sessionInputReferences(w http.ResponseWriter, r *http.Request, agentID, sessionID string, input agentSessionInput) ([]im.AttachmentFile, bool) {
	if !h.authorizeSessionFiles(w, r) {
		return nil, false
	}
	files := make([]im.AttachmentFile, 0, len(input.attachmentIDs))
	for _, id := range input.attachmentIDs {
		file, err := h.im.SessionAttachmentFile(agentID, sessionID, id)
		if errors.Is(err, im.ErrSessionAttachmentNotFound) {
			writeAgentSessionError(w, http.StatusNotFound, "attachment_not_found", "session attachment not found", "attachment_ids")
			return nil, false
		}
		if err != nil {
			slog.Warn("read session attachment failed", "attachment_id", id, "error", err)
			writeAgentSessionError(w, http.StatusServiceUnavailable, "attachment_unavailable", "session attachment could not be read", "attachment_ids")
			return nil, false
		}
		files = append(files, file)
	}
	if err := input.validateFileLimits(files); err != nil {
		writeSessionInputError(w, err)
		return nil, false
	}
	return files, true
}

func (h *Handler) restoreSessionWorkspaceFiles(ctx context.Context, agent agentengine.Agent, files []im.AttachmentFile) ([]im.MessageAttachment, error) {
	if len(files) == 0 {
		return nil, nil
	}
	root, err := h.configuredAgentWorkspace(agent.ID, agent.Spec.Runtime.Options)
	if err != nil {
		return nil, err
	}
	attachments := make([]im.MessageAttachment, 0, len(files))
	for _, file := range files {
		if err := im.RestoreSessionWorkspaceFile(ctx, root, file); err != nil {
			return nil, err
		}
		attachments = append(attachments, file.Attachment)
	}
	return attachments, nil
}
