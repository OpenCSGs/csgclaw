package api

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"

	"csgclaw/internal/activity"
	"csgclaw/internal/diagnostics"
	"csgclaw/internal/im"
)

// Diagnostics are a desktop UI surface. Runtime callers must not inspect other
// agents' execution details even when they share a room.
func (h *Handler) diagnosticRoom(w http.ResponseWriter, r *http.Request) (string, bool) {
	if h.im == nil {
		http.Error(w, "IM unavailable", http.StatusServiceUnavailable)
		return "", false
	}
	_, agentCredential := h.agentForAuthorization(r.Header.Get("Authorization"))
	if agentCredential || appRequestAgentID(r) != "" || strings.TrimSpace(r.Header.Get("X-CSGClaw-Caller-Agent")) != "" {
		http.Error(w, "diagnostics require desktop access", http.StatusForbidden)
		return "", false
	}
	roomID := pathValue(r, "id")
	if _, ok := h.im.Room(roomID); !ok {
		http.Error(w, "room not found", http.StatusNotFound)
		return "", false
	}
	return roomID, true
}
func (h *Handler) listDiagnostics(w http.ResponseWriter, r *http.Request) {
	room, ok := h.diagnosticRoom(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	items := h.im.Diagnostics().List(room, q.Get("source_id"), q.Get("agent_id"), "", q.Get("thread_id"))
	videos := h.diagnosticVideos(room)
	filtered := items[:0]
	for _, item := range items {
		enrichVideoDiagnostic(&item, videos[item.TurnID], false)
		if q.Get("status") == "" || item.Status == q.Get("status") {
			filtered = append(filtered, item)
		}
	}
	items = filtered
	limit := 50
	if n, e := strconv.Atoi(q.Get("limit")); e == nil && n > 0 {
		limit = min(n, 100)
	}
	start := 0
	if cursor := q.Get("cursor"); cursor != "" {
		start = len(items)
		for i, v := range items {
			if v.ID == cursor {
				start = i + 1
				break
			}
		}
	}
	end := min(start+limit, len(items))
	next := ""
	if end < len(items) {
		next = items[end-1].ID
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items[start:end], "next_cursor": next})
}
func (h *Handler) getDiagnostic(w http.ResponseWriter, r *http.Request) {
	room, ok := h.diagnosticRoom(w, r)
	if !ok {
		return
	}
	record, found := h.im.Diagnostics().Get(room, pathValue(r, "diagnostic_id"))
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]string{"code": "diagnostic_unavailable", "message": "Diagnostics were not collected or have expired."}})
		return
	}
	h.enrichDiagnosticTools(&record)
	enrichVideoDiagnostic(&record, h.diagnosticVideos(room)[record.TurnID], true)
	writeJSON(w, http.StatusOK, record)
}
func (h *Handler) reportDiagnosticTiming(w http.ResponseWriter, r *http.Request) {
	room, ok := h.diagnosticRoom(w, r)
	if !ok {
		return
	}
	var input struct {
		SourceID string `json:"source_id"`
		TurnID   string `json:"turn_id"`
		diagnostics.Timing
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.SourceID == "" || input.TurnID == "" {
		http.Error(w, "invalid timing", http.StatusBadRequest)
		return
	}
	for _, v := range []*float64{input.FirstMS, input.CompleteMS, input.FirstTextMS} {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 || *v > float64(diagnostics.Retention.Milliseconds())) {
			http.Error(w, "invalid duration", http.StatusBadRequest)
			return
		}
	}
	if (input.FirstTextMS == nil) != (input.FirstTextAt == nil) {
		http.Error(w, "incomplete first text timing", http.StatusBadRequest)
		return
	}
	if input.FirstMS != nil && input.CompleteMS != nil && *input.FirstMS > *input.CompleteMS {
		http.Error(w, "invalid timing order", http.StatusBadRequest)
		return
	}
	if !h.im.Diagnostics().BrowserTiming(room, input.SourceID, input.TurnID, input.Timing) {
		http.Error(w, "source not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Existing transcripts already contain normalized tool inputs. Join by the
// exact turn/tool IDs when an older trace has timing but no display details.
func (h *Handler) enrichDiagnosticTools(record *diagnostics.Snapshot) {
	needed := false
	for _, span := range record.Spans {
		if span.Owner == "tool" && span.Details == nil {
			needed = true
			break
		}
	}
	if !needed {
		return
	}
	messages, err := h.im.ListMessagesWithOptions(record.RoomID, im.ListMessagesOptions{IncludeThreadReplies: true})
	if err != nil {
		return
	}
	for _, message := range messages {
		if message.ID != record.TurnID+"-final" {
			continue
		}
		meta, _ := message.Metadata["csgclaw"].(map[string]any)
		raw, err := json.Marshal(meta["turn_progress"])
		if err != nil {
			return
		}
		var progress activity.TurnProgress
		if json.Unmarshal(raw, &progress) != nil || progress.ID != record.TurnID {
			return
		}
		tools := map[string]diagnostics.SpanDetails{}
		for _, item := range progress.Items {
			if item.Tool != nil {
				fields := map[string]any{"command": item.Tool.Command, "cwd": item.Tool.Cwd}
				if item.Tool.DurationMS != nil {
					fields["durationMs"] = *item.Tool.DurationMS
				}
				if item.Tool.ExitCode != nil {
					fields["exitCode"] = *item.Tool.ExitCode
				}
				tools["tool:"+item.ID] = diagnostics.ToolDetails(item.Tool.Name, item.Tool.Input, fields)
			}
		}
		for i := range record.Spans {
			span := &record.Spans[i]
			if span.Details == nil {
				if details, ok := tools[span.ID]; ok {
					span.Details = &details
				}
			}
		}
		return
	}
}
