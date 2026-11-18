package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"csgclaw/internal/relay"
	"csgclaw/internal/resourcequery"
)

func (h *Handler) writeCloudReadError(w http.ResponseWriter, err error) {
	if writeConversationResourceError(w, err) {
		return
	}
	code, status := "cloud_resources_unavailable", http.StatusBadGateway
	var resourceRemote *relay.HTTPError
	if errors.As(err, &resourceRemote) {
		switch resourceRemote.Detail.Code {
		case "resource_list_changed", "invalid_resource_query", "invalid_resource_reference", "resource_not_attached", "resource_disabled", "resource_unavailable":
			writeCodedAPIError(w, resourceRemote.Status, resourceRemote.Detail.Code, "Cloud resource request failed")
			return
		}
	}
	if errors.Is(err, relay.ErrResourcesUnsupported) || (resourceRemote != nil && resourceRemote.Detail.Code == "executor_resources_unsupported") {
		code, status = "executor_resources_unsupported", http.StatusConflict
	}
	if errors.Is(err, relay.ErrChatReference) {
		code, status = "relay_chat_reference_unavailable", http.StatusConflict
	}
	if errors.Is(err, relay.ErrCloudEndpointUnavailable) {
		code, status = "cloud_endpoint_unavailable", http.StatusConflict
	}
	var remote *relay.HTTPError
	if errors.As(err, &remote) && (remote.Status == 401 || remote.Status == 403) {
		code, status = "cloud_resource_access_denied", http.StatusForbidden
	}
	message := "Cloud agent resources are temporarily unavailable. Check the cloud service and try again."
	switch code {
	case "executor_resources_unsupported":
		message = "This cloud agent version does not support the selected resources. Update the cloud agent and try again."
	case "relay_chat_reference_unavailable":
		message = "The cloud agent connection is no longer available. Add the agent again from the plaza."
	case "cloud_endpoint_unavailable":
		message = "The cloud agent read endpoint is unavailable. Check the instance status."
	case "cloud_resource_access_denied":
		message = "You do not have access to this cloud agent's resources. Check your account and instance permissions."
	}
	writeCodedAPIError(w, status, code, message)
}

func (h *Handler) handleCloudProfile(w http.ResponseWriter, r *http.Request) {
	if h.relayManager == nil {
		h.writeCloudReadError(w, relay.ErrChatReference)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	profile, err := h.relayManager.CloudProfile(ctx, pathValue(r, "id"), r.URL.Query().Get("connection_id"))
	if err != nil {
		h.writeCloudReadError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, profile)
}

func (h *Handler) handleCloudResources(w http.ResponseWriter, r *http.Request) {
	if h.relayManager == nil {
		h.writeCloudReadError(w, relay.ErrChatReference)
		return
	}
	query, err := parseResourceQuery(r)
	if err != nil || query == nil {
		writeResourceQueryError(w, resourcequery.ErrInvalid)
		return
	}
	values := r.URL.Query()
	values.Del("connection_id")
	switch values.Get("type") {
	case "skill", "connector", "knowledge_base":
	default:
		writeResourceQueryError(w, resourcequery.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	page, err := h.relayManager.CloudResources(ctx, pathValue(r, "id"), r.URL.Query().Get("connection_id"), values)
	if err != nil {
		h.writeCloudReadError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, page)
}
