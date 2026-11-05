package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/im"
)

var errSessionFileAuthorization = errors.New("valid server authorization is required")

func writeSessionInputError(w http.ResponseWriter, err error) {
	if errors.Is(err, errSessionFileAuthorization) {
		writeAgentSessionError(w, http.StatusUnauthorized, "unauthorized", err.Error(), nil)
		return
	}
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

func (h *Handler) uploadAgentSessionAttachments(w http.ResponseWriter, r *http.Request) {
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
		writeSessionInputError(w, errSessionFileAuthorization)
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

func readSessionAttachmentIDs(decoder *json.Decoder) ([]string, error) {
	open, err := decoder.Token()
	if err != nil || open == nil {
		return nil, err
	}
	if open != json.Delim('[') {
		return nil, &agentSessionRequestValidationError{param: "attachment_ids", message: "attachment_ids must be an array of IDs"}
	}
	var ids []string
	for decoder.More() {
		if len(ids) == im.MaxAttachmentsPerMessage {
			return nil, &agentSessionRequestValidationError{param: "attachment_ids", message: fmt.Sprintf("too many input files: max %d", im.MaxAttachmentsPerMessage)}
		}
		var id string
		if err := decoder.Decode(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	_, err = decoder.Token()
	return ids, err
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

func (h *Handler) sessionInputReferences(w http.ResponseWriter, agent agentengine.Agent, sessionID string, input agentSessionInput) ([]im.MessageAttachment, bool) {
	root, err := h.sessionWorkspaceRoot(agent)
	if err != nil {
		writeAgentSessionError(w, http.StatusServiceUnavailable, "attachment_unavailable", "agent workspace could not be resolved", "attachment_ids")
		return nil, false
	}
	attachments := make([]im.MessageAttachment, 0, len(input.attachmentIDs))
	for _, id := range input.attachmentIDs {
		file, err := h.im.SessionAttachmentFile(agent.ID, sessionID, id)
		if errors.Is(err, im.ErrSessionAttachmentNotFound) || (err == nil && !file.WorkspaceMatches(root)) {
			writeAgentSessionError(w, http.StatusNotFound, "attachment_not_found", "session attachment not found", "attachment_ids")
			return nil, false
		}
		if err != nil {
			slog.Warn("read session attachment failed", "attachment_id", id, "error", err)
			writeAgentSessionError(w, http.StatusServiceUnavailable, "attachment_unavailable", "session attachment could not be read", "attachment_ids")
			return nil, false
		}
		attachments = append(attachments, file.Attachment)
	}
	if err := input.validateFileLimits(attachments); err != nil {
		writeSessionInputError(w, err)
		return nil, false
	}
	return attachments, true
}
