package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"csgclaw/internal/diagnostics"
	"csgclaw/internal/im"
	"csgclaw/internal/taskcore"
	"csgclaw/internal/taskmeta"
)

var diagnosticFilenameUnsafe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

type bundleIssue struct {
	TurnID  string `json:"turn_id,omitempty"`
	Section string `json:"section"`
	Reason  string `json:"reason"`
}

// The export is assembled per request. Only irreplaceable execution evidence
// is persisted; journal entries and log lines are decoded and written singly.
func (h *Handler) getDiagnosticBundle(w http.ResponseWriter, r *http.Request) {
	room, ok := h.diagnosticRoom(w, r)
	if !ok {
		return
	}
	store := h.im.Diagnostics()
	selected, found := store.Get(room, pathValue(r, "diagnostic_id"))
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]string{"code": "diagnostic_unavailable", "message": "Diagnostics were not collected or have expired."}})
		return
	}
	messages, err := h.im.ListMessagesWithOptions(room, im.ListMessagesOptions{IncludeThreadReplies: true})
	messageErr := err
	issues := []bundleIssue{}
	coverage := []diagnosticCoverage{}
	if err != nil {
		issues = append(issues, bundleIssue{Section: "messages", Reason: "messages_unavailable"})
	}
	turns, tasks := h.bundleRelated(selected, messages)
	capturedAt := time.Now().UTC()
	windows := make([]diagnostics.LogWindow, 0, len(turns))
	logs := diagnostics.CurrentLogs()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="csgclaw-turn-%s-diagnostics.json"`, diagnosticFilenameUnsafe.ReplaceAllString(strings.TrimPrefix(selected.TurnID, "turn-csgclaw-"), "_")))
	}
	stream := &bundleStream{out: w}
	stream.raw("{\n")
	stream.field("schema_version", 1, true)
	stream.field("turn_id", selected.TurnID, true)
	stream.field("collected_at", capturedAt, true)
	current := diagnostics.Environment()
	current["distribution"] = h.runtimeDistribution
	if current["distribution"] == "" {
		current["distribution"] = "cli"
	}
	current["goroutines"] = runtime.NumGoroutine()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	current["memory"] = map[string]any{"heap_alloc_bytes": memory.HeapAlloc, "heap_sys_bytes": memory.HeapSys, "gc_cycles": memory.NumGC}
	current["browser"] = map[string]string{"user_agent": r.UserAgent(), "timezone": boundedDiagnosticQuery(r.URL.Query().Get("timezone"), 128)}
	stream.field("current_environment", current, true)
	videos := h.diagnosticVideos(room)
	stream.raw("  \"turns\": [")
	for index, record := range turns {
		if index > 0 {
			stream.raw(",")
		}
		stream.raw("\n    {\n")
		h.enrichDiagnosticTools(&record)
		enrichVideoDiagnostic(&record, videos[record.TurnID], true)
		stream.namedValue("diagnostic", record, "      ", true)
		if record.Status == "running" || record.Status == "queued" {
			issues = append(issues, bundleIssue{record.TurnID, "execution", "execution_in_progress"})
		}
		if record.Incomplete {
			issues = append(issues, bundleIssue{record.TurnID, "timeline", "timeline_incomplete"})
		}
		for _, reason := range record.CollectionIssues {
			issues = append(issues, bundleIssue{record.TurnID, "context", reason})
		}
		end := record.StartedAt.Add(time.Duration(record.TotalMS * float64(time.Millisecond)))
		if record.Status == "running" || record.Status == "queued" {
			end = capturedAt
		}
		windows = append(windows, diagnostics.LogWindow{Start: record.StartedAt.Add(-time.Minute), End: minTime(end.Add(time.Minute), capturedAt)})
		stream.raw("      \"context\": [")
		entries := 0
		runtimePaths := map[string]bool{}
		sections := map[string]int{}
		evidence := newTurnEvidence()
		err := store.ReadContext(r.Context(), record.ID, func(entry diagnostics.ContextEntry) error {
			if stream.err != nil {
				return stream.err
			}
			if entry.At.After(capturedAt) {
				return nil
			}
			sections[entry.Kind]++
			evidence.Observe(entry)
			if entry.Kind == "runtime.configuration" {
				var metadata struct {
					Stderr string `json:"stderr_path"`
				}
				if json.Unmarshal(entry.Data, &metadata) == nil && metadata.Stderr != "" {
					runtimePaths[metadata.Stderr] = true
				}
			}
			if entry.Kind == "model.body_end" {
				var status struct {
					Complete bool `json:"complete"`
				}
				if json.Unmarshal(entry.Data, &status) == nil && !status.Complete {
					issues = append(issues, bundleIssue{record.TurnID, "model", "model_body_incomplete"})
				}
			}

			if entries > 0 {
				stream.raw(",")
			}
			stream.raw("\n")
			stream.value(entry, "        ")
			entries++
			if entry.Kind == "collection.issue" {
				var detail struct {
					Reason string `json:"reason"`
				}
				_ = json.Unmarshal(entry.Data, &detail)
				if detail.Reason == "" {
					detail.Reason = "collection_issue"
				}
				issues = append(issues, bundleIssue{record.TurnID, "context", detail.Reason})
			}
			return stream.err
		})
		if err != nil || entries == 0 {
			issues = append(issues, bundleIssue{record.TurnID, "context", "not_collected_expired_or_unavailable"})
		}
		coverage = append(coverage, evidence.Coverage(record)...)
		if record.RuntimeStartMS != nil && sections["model.runtime_request"] == 0 && evidence.ingressFailures == 0 {
			issues = append(issues, bundleIssue{record.TurnID, "model", "model_context_not_collected"})
		}
		stream.raw("\n      ],\n      \"server_logs\": [")
		lines := 0
		err = logs.Read(r.Context(), record.TurnID, nil, func(entry diagnostics.LogEntry) error {
			if entry.At.After(capturedAt) {
				return nil
			}
			if lines > 0 {
				stream.raw(",")
			}
			stream.raw("\n")
			stream.value(entry, "        ")
			lines++
			return stream.err
		})
		serverCoverage := coverageItem(record.TurnID, "server_logs", lines)
		if err != nil {
			serverCoverage.Status = "partial"
			serverCoverage.Reasons = append(serverCoverage.Reasons, err.Error())
		}
		if lines == 0 {
			serverCoverage.Status = "partial"
			serverCoverage.Reasons = append(serverCoverage.Reasons, "turn_logs_unavailable")
		}
		coverage = append(coverage, serverCoverage)
		if err != nil || lines == 0 {
			reason := "turn_logs_unavailable"
			if err != nil {
				reason = err.Error()
			}
			issues = append(issues, bundleIssue{record.TurnID, "server_logs", reason})
		}
		stream.raw("\n      ],\n      \"runtime_logs\": [")
		// Shared runtime stderr often lacks native turn identity. Include only exact
		// matches and label the source; never attach the Agent's latest tail.
		runtimeLines := 0
		if record.RuntimeLogSource != nil {
			report := diagnostics.ReadRuntimeLogs(r.Context(), *record.RuntimeLogSource, windows[len(windows)-1], []string{record.TurnID, record.RuntimeTurnID}, func(entry diagnostics.RuntimeLogEntry) error {
				if runtimeLines > 0 {
					stream.raw(",")
				}
				stream.raw("\n")
				stream.value(entry, "        ")
				runtimeLines++
				return stream.err
			})
			coverage = append(coverage, coverageItem(record.TurnID, "runtime_logs", report.Lines, report.Reasons...))
			coverage = append(coverage, coverageItem(record.TurnID, "runtime_process_events", sections["runtime.process_event"]))
		} else {
			for path := range runtimePaths {
				filter := &diagnosticRuntimeLogWriter{turn: record, emit: func(line string) error {
					if runtimeLines > 0 {
						stream.raw(",")
					}
					stream.raw("\n")
					stream.value(map[string]any{"source": "runtime.stderr", "association": "exact_identity", "line": line}, "        ")
					runtimeLines++
					return stream.err
				}}
				file, openErr := os.Open(path)
				if openErr == nil {
					info, statErr := file.Stat()
					if statErr == nil && info.Mode().IsRegular() {
						reader := bufio.NewReader(io.LimitReader(file, info.Size()))
						for {
							if r.Context().Err() != nil {
								break
							}
							line, readErr := reader.ReadSlice('\n')
							_, writeErr := filter.Write(line)
							if writeErr != nil {
								break
							}
							if readErr == bufio.ErrBufferFull {
								continue
							}
							if readErr != nil {
								break
							}
						}
					}
					file.Close()
					filter.finish()
				} else {
					issues = append(issues, bundleIssue{record.TurnID, "runtime_logs", "runtime_logs_unavailable"})
				}
			}

			if record.RuntimeStartMS == nil && len(runtimePaths) == 0 {
				coverage = append(coverage, notApplicable(record.TurnID, "runtime_logs", "runtime_process_not_started"), notApplicable(record.TurnID, "runtime_process_events", "runtime_process_not_started"))
			} else {
				coverage = append(coverage, coverageItem(record.TurnID, "runtime_logs", runtimeLines, "runtime_process_identity_not_collected"), coverageItem(record.TurnID, "runtime_process_events", sections["runtime.process_event"], "runtime_process_observer_not_collected"))
			}
		}

		if runtimeLines == 0 && record.RuntimeStartMS != nil {
			issues = append(issues, bundleIssue{record.TurnID, "runtime_logs", "no_exactly_attributable_runtime_log_lines"})
		}
		stream.raw("\n      ]\n    }")
	}
	stream.raw("\n  ],\n")
	turnIDs := map[string]bool{}
	sourceIDs := map[string]bool{}
	taskIDs := map[string]bool{}
	for _, record := range turns {
		turnIDs[record.TurnID] = true
		sourceIDs[record.SourceID] = true
		if record.ThreadID != "" {
			sourceIDs[record.ThreadID] = true
		}
	}
	for _, task := range tasks {
		taskIDs[task.ID] = true
		if task.SourceMessageID != "" {
			sourceIDs[task.SourceMessageID] = true
		}
	}
	included := []im.Message{}
	for _, message := range messages {
		meta, _ := message.Metadata["csgclaw"].(map[string]any)
		turn, _ := meta["turn_id"].(string)
		if turn == "" {
			meta, _ = message.Metadata["diagnostics"].(map[string]any)
			turn, _ = meta["turn_id"].(string)
		}
		if turnIDs[turn] || sourceIDs[message.ID] || taskIDs[taskmeta.ID(message.Metadata)] {
			included = append(included, message)
		}
	}
	messagesCoverage := coverageItem("", "messages", len(included))
	if messageErr != nil {
		messagesCoverage.Status = "partial"
		messagesCoverage.Reasons = append(messagesCoverage.Reasons, "messages_unavailable")
	}
	coverage = append(coverage, messagesCoverage)
	stream.field("messages", included, true)
	stream.field("tasks", tasks, true)
	if len(tasks) > 0 && h.roomTaskSvc != nil {
		if snapshot, ok := h.roomTaskSvc.DiagnosticSnapshot(room, tasks[0].ID); ok {
			stream.field("task_context", snapshot, true)
		}
	}
	stream.raw("  \"files\": [")
	fileCount := 0
	seenFiles := map[string]bool{}
	for _, message := range included {
		for _, attachment := range message.Attachments {
			if seenFiles[attachment.ID] {
				continue
			}
			seenFiles[attachment.ID] = true
			file := map[string]any{"metadata": attachment, "content_status": "binary_metadata_only"}
			if diagnostics.TextMediaType(attachment.MediaType) {
				stored, readErr := h.im.RoomAttachmentFile(room, attachment.ID)
				if readErr == nil {
					var opened *os.File
					opened, readErr = os.Open(stored.Path)
					if readErr == nil {
						var raw []byte
						raw, readErr = io.ReadAll(io.LimitReader(opened, 128<<20+1))
						opened.Close()
						if readErr == nil && len(raw) <= 128<<20 && utf8.Valid(raw) {
							file["text"] = string(raw)
							file["content_status"] = "available"
						} else {
							file["content_status"] = "unavailable"
						}
					}
				}
				if file["content_status"] != "available" {
					issues = append(issues, bundleIssue{Section: "files", Reason: "text_attachment_unavailable_or_too_large"})
				}
			}
			if fileCount > 0 {
				stream.raw(",")
			}
			stream.raw("\n")
			stream.value(file, "    ")
			fileCount++
		}
	}
	stream.raw("\n  ],\n  \"system_logs\": [")
	systemLines := 0
	err = logs.Read(r.Context(), "", windows, func(entry diagnostics.LogEntry) error {
		if systemLines > 0 {
			stream.raw(",")
		}
		stream.raw("\n")
		stream.value(entry, "    ")
		systemLines++
		return stream.err
	})
	if err != nil {
		issues = append(issues, bundleIssue{Section: "system_logs", Reason: err.Error()})
	}
	systemReasons := logs.Coverage(windows)
	if err != nil {
		systemReasons = append(systemReasons, err.Error())
	}
	coverage = append(coverage, coverageItem("", "system_logs", systemLines, systemReasons...))
	for _, section := range coverage {
		if section.Status == "partial" || section.Status == "in_progress" {
			for _, reason := range section.Reasons {
				issues = append(issues, bundleIssue{TurnID: section.TurnID, Section: section.Section, Reason: reason})
			}
		}
	}
	issues = uniqueBundleIssues(issues)
	stream.raw("\n  ],\n")
	stream.field("collection", map[string]any{"scope": "required_turn_evidence_sections", "evidence_version": 2, "sections": coverage, "complete": len(issues) == 0, "issues": issues, "system_log_margin_seconds": 60, "context_retention_days": 7, "context_max_bytes": diagnostics.ContextBytes, "server_log_max_bytes": diagnostics.LogBytes, "binary_files": "metadata_only", "system_log_windows": windows}, false)
	stream.raw("}\n")
}

func boundedDiagnosticQuery(value string, limit int) string {
	if len(value) > limit {
		return ""
	}
	return value
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (h *Handler) bundleRelated(selected diagnostics.Snapshot, messages []im.Message) ([]diagnostics.Snapshot, []taskcore.Task) {
	taskIDs := map[string]bool{}
	sourceIDs := map[string]bool{}
	if selected.SourceID != "" {
		sourceIDs[selected.SourceID] = true
	}
	if selected.TaskID != "" {
		taskIDs[selected.TaskID] = true
	}
	for _, message := range messages {
		if message.ID == selected.SourceID || message.ID == selected.TurnID+"-final" {
			if id := taskmeta.ID(message.Metadata); id != "" {
				taskIDs[id] = true
			}
		}
	}
	tasks := []taskcore.Task{}
	if h.roomTaskSvc != nil {
		all := h.roomTaskSvc.List(selected.RoomID)
		for _, task := range all {
			if task.SourceMessageID == selected.SourceID {
				taskIDs[task.ID] = true
			}
		}
		// Expand explicit parent/child relations to a fixed point. Room filtering is
		// enforced by List and every diagnostic lookup below remains room-scoped.
		for changed := true; changed; {
			changed = false
			for _, task := range all {
				if taskIDs[task.ID] && task.ParentID != "" && !taskIDs[task.ParentID] {
					taskIDs[task.ParentID] = true
					changed = true
				}
				if taskIDs[task.ParentID] && !taskIDs[task.ID] {
					taskIDs[task.ID] = true
					changed = true
				}
			}
		}
		for _, task := range all {
			if taskIDs[task.ID] {
				tasks = append(tasks, task)
				if task.SourceMessageID != "" {
					sourceIDs[task.SourceMessageID] = true
				}
			}
		}
	}
	for _, message := range messages {
		if taskIDs[taskmeta.ID(message.Metadata)] {
			sourceIDs[message.ID] = true
		}
	}
	related := []diagnostics.Snapshot{}
	for _, item := range h.im.Diagnostics().List(selected.RoomID, "", "", "", "") {
		if item.ID == selected.ID {
			continue
		}
		if sourceIDs[item.SourceID] || taskIDs[item.TaskID] {
			if full, ok := h.im.Diagnostics().Get(selected.RoomID, item.ID); ok {
				related = append(related, full)
			}
		}
	}
	sort.Slice(related, func(i, j int) bool { return related[i].StartedAt.Before(related[j].StartedAt) })
	return append([]diagnostics.Snapshot{selected}, related...), tasks
}

type bundleStream struct {
	out io.Writer
	err error
}

func (s *bundleStream) raw(value string) {
	if s.err == nil {
		_, s.err = io.WriteString(s.out, value)
	}
}
func (s *bundleStream) value(value any, indent string) {
	if s.err != nil {
		return
	}
	raw := diagnostics.Protect(value)
	var buffer bytes.Buffer
	if err := json.Indent(&buffer, raw, indent, "  "); err != nil {
		s.err = err
		return
	}
	s.raw(indent + buffer.String())
}
func (s *bundleStream) namedValue(name string, value any, indent string, comma bool) {
	s.raw(indent + fmt.Sprintf("%q", name) + ": ")
	raw := diagnostics.Protect(value)
	var buffer bytes.Buffer
	if err := json.Indent(&buffer, raw, indent, "  "); err != nil {
		s.err = err
		return
	}
	s.raw(buffer.String())
	if comma {
		s.raw(",")
	}
	s.raw("\n")
}
func (s *bundleStream) field(name string, value any, comma bool) {
	s.namedValue(name, value, "  ", comma)
}

type diagnosticRuntimeLogWriter struct {
	turn     diagnostics.Snapshot
	pending  []byte
	discard  bool
	patterns []*regexp.Regexp
	emit     func(string) error
}

func (w *diagnosticRuntimeLogWriter) Write(data []byte) (int, error) {
	size := len(data)
	for len(data) > 0 {
		part, rest, found := bytes.Cut(data, []byte{'\n'})
		if len(w.pending)+len(part) > 1<<20 {
			w.pending = nil
			w.discard = true
		}
		if !w.discard {
			w.pending = append(w.pending, part...)
		}
		if !found {
			break
		}
		if !w.discard {
			if err := w.line(); err != nil {
				return 0, err
			}
		}
		w.pending = nil
		w.discard = false
		data = rest
	}
	return size, nil
}
func (w *diagnosticRuntimeLogWriter) line() error {
	line := string(w.pending)
	w.pending = nil
	if w.patterns == nil {
		for _, id := range []string{w.turn.TurnID, w.turn.RuntimeTurnID} {
			if id != "" {
				w.patterns = append(w.patterns, regexp.MustCompile(`(?:^|[\s{,\[])"?turn_id"?[=:]\s*"?`+regexp.QuoteMeta(id)+`"?(?:[\s,}\]]|$)`))
			}
		}
	}
	for _, pattern := range w.patterns {
		if pattern.MatchString(line) {
			return w.emit(diagnostics.ProtectText(line))
		}
	}
	return nil
}
func (w *diagnosticRuntimeLogWriter) finish() {
	if !w.discard {
		_ = w.line()
	}
}
