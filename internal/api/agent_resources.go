package api

import (
	"errors"
	"net/http"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/conversationresource"
	"csgclaw/internal/resourcequery"
)

func (h *Handler) EnableConversationResources() {
	if h.agentEngine == nil {
		return
	}
	h.resourcesOnce.Do(func() {
		var apps conversationresource.AppReader
		if h.apps != nil {
			apps = h.apps
		}
		h.resources = conversationresource.New(h.agentEngine.Agents(), apps)
		if h.participantBridge != nil {
			h.participantBridge.SetResourceInputProvider(h.resources.PrepareInput)
		}
	})
}

func (h *Handler) handleAgentResources(w http.ResponseWriter, r *http.Request) {
	if h.resources == nil {
		writeCodedAPIError(w, http.StatusServiceUnavailable, conversationresource.ServiceUnavailable, "资源服务不可用")
		return
	}
	query, err := parseResourceQuery(r)
	if err != nil || query == nil {
		writeResourceQueryError(w, resourcequery.ErrInvalid)
		return
	}
	items, meta, err := h.resources.List(r.Context(), pathValue(r, "id"), apitypes.ResourceType(r.URL.Query().Get("type")), *query)
	if err != nil {
		if !writeResourceQueryError(w, err) && !writeConversationResourceError(w, err) {
			writeAgentGetError(w, err)
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, resourcePage[apitypes.AgentResourceSummary]{Items: items, Metadata: meta})
}

func writeConversationResourceError(w http.ResponseWriter, err error) bool {
	if contract.ErrorCodeOf(err) == contract.ErrorAgentNotFound {
		writeCodedAPIError(w, http.StatusNotFound, "agent_not_found", "Agent 不存在")
		return true
	}
	var resourceErr *conversationresource.Error
	if !errors.As(err, &resourceErr) {
		return false
	}
	status := http.StatusBadRequest
	switch resourceErr.Code {
	case conversationresource.ServiceUnavailable:
		status = http.StatusServiceUnavailable
	case conversationresource.ResourceNotAttached, conversationresource.ResourceDisabled, conversationresource.ResourceUnavailable:
		status = http.StatusConflict
	}
	writeCodedAPIError(w, status, resourceErr.Code, resourceErr.Error())
	return true
}
