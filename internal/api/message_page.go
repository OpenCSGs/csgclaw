package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"csgclaw/internal/channel/csgclaw"
	"csgclaw/internal/im"
)

func (h *Handler) writeMessagePage(w http.ResponseWriter, r *http.Request, channel *csgclaw.Service, roomID string) {
	query := r.URL.Query()
	limit, err := strconv.Atoi(query.Get("limit"))
	if err != nil || limit < 1 || limit > im.MaxMessagePageSize {
		writeAPIError(w, fmt.Errorf("limit must be between 1 and %d", im.MaxMessagePageSize), http.StatusBadRequest)
		return
	}
	before, after := strings.TrimSpace(query.Get("before")), strings.TrimSpace(query.Get("after"))
	if before != "" && after != "" {
		writeAPIError(w, fmt.Errorf("before and after are mutually exclusive"), http.StatusBadRequest)
		return
	}
	rootID := query.Get("thread_root_id")
	page, err := channel.ListMessagePage(roomID, im.MessagePageOptions{
		Before:               before,
		After:                after,
		Limit:                limit,
		ThreadRootID:         rootID,
		IncludeThreadReplies: rootID != "" || parseBoolQuery(query.Get("include_thread_replies")),
		PublicAutonomousOnly: appRequestAgentID(r) != "",
	})
	if err != nil {
		writeIMError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}
