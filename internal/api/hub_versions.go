package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	hub "csgclaw/internal/template"
	"github.com/go-chi/chi/v5"
)

func templateVersionAuthor(r *http.Request) (*hub.VersionAuthor, error) {
	status, err := appAuthStatus(r)
	if err != nil {
		return nil, err
	}
	if !status.Authenticated {
		return nil, nil
	}
	return &hub.VersionAuthor{ID: status.UserID, Name: status.Name}, nil
}

func (h *Handler) handleHubVersions(w http.ResponseWriter, r *http.Request) {
	svc, err := h.hubServiceForRequest(r)
	if err != nil || svc == nil {
		writeHubTemplateError(w, 503, "template_service_unavailable", "hub service is not configured", "")
		return
	}
	id := hubTemplateIDPathValue(r)
	var result any
	if raw := chi.URLParam(r, "revision"); raw == "" {
		result, err = svc.Versions(r.Context(), id)
	} else {
		revision, parseErr := strconv.Atoi(raw)
		if parseErr != nil || revision < 1 {
			writeHubTemplateError(w, 400, "template_version_invalid", "revision must be a positive integer", "")
			return
		}
		switch {
		case r.Method == http.MethodPost:
			var request struct {
				Message string `json:"commit_message"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				writeHubTemplateError(w, 400, "invalid_request", err.Error(), "")
				return
			}
			author, authErr := templateVersionAuthor(r)
			if authErr != nil {
				writeHubTemplateError(w, 500, "template_author_failed", authErr.Error(), "")
				return
			}
			item, restoreErr := svc.RestoreVersion(r.Context(), id, revision, request.Message, author)
			err = restoreErr
			result = presentHubTemplate(item)
		case r.URL.Query().Get("file") == "true":
			result, err = svc.ReadVersionFile(r.Context(), id, revision, r.URL.Query().Get("path"))
		default:
			result, err = svc.ListVersionWorkspace(r.Context(), id, revision, r.URL.Query().Get("path"))
		}
	}
	if err != nil {
		status := http.StatusBadRequest
		code := "template_version_failed"
		if errors.Is(err, hub.ErrTemplateNotFound) {
			status = 404
			code = "template_version_not_found"
		}
		writeHubTemplateError(w, status, code, err.Error(), "")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
