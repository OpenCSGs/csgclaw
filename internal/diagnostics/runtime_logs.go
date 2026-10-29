package diagnostics

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

type RuntimeLogEntry struct {
	Source      string     `json:"source"`
	Association string     `json:"association"`
	InstanceID  string     `json:"instance_id"`
	PID         int        `json:"pid"`
	Offset      int64      `json:"offset"`
	At          *time.Time `json:"at,omitempty"`
	Line        string     `json:"line"`
}
type RuntimeLogReport struct {
	Lines        int      `json:"lines"`
	Unattributed int      `json:"process_context_lines"`
	Truncated    int      `json:"truncated_lines"`
	Reasons      []string `json:"reasons"`
}

var runtimeTurnField = regexp.MustCompile(`(?i)(?:^|[\s{,\[])"?turn_?id"?\s*[=:]\s*"?([^\s,"}\]]+)`)
var runtimeANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)
var runtimeTimestamp = regexp.MustCompile(`^(?:time="?|\[)?(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d))`)

func (r *RuntimeLogReport) missing(reason string) {
	for _, old := range r.Reasons {
		if old == reason {
			return
		}
	}
	r.Reasons = append(r.Reasons, reason)
}

// ReadRuntimeLogs includes exact-turn lines plus explicitly labeled context
// from the same process instance and time window. Other turn IDs are excluded.
func ReadRuntimeLogs(ctx context.Context, source RuntimeLogSource, window LogWindow, turnIDs []string, yield func(RuntimeLogEntry) error) RuntimeLogReport {
	report := RuntimeLogReport{Reasons: []string{}}
	if source.InstanceID == "" || source.PID <= 0 || source.ProcessStartOffset < 0 {
		report.missing("runtime_process_identity_unavailable")
		return report
	}
	file, err := os.Open(source.Path)
	if err != nil {
		report.missing("runtime_logs_unavailable")
		return report
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		report.missing("runtime_log_stat_failed")
		return report
	}
	end := info.Size()
	if source.ProcessEndOffset > 0 {
		end = min(end, source.ProcessEndOffset)
	}
	if end < source.ProcessStartOffset || end < source.TurnStartOffset {
		report.missing("runtime_log_rotated_or_truncated")
		return report
	}
	marker, err := io.ReadAll(io.NewSectionReader(file, source.ProcessStartOffset, min(end-source.ProcessStartOffset, 128<<10)))
	if err != nil || !bytes.Contains(marker, []byte("instance_id="+source.InstanceID)) {
		report.missing("runtime_process_marker_missing_or_rotated")
		return report
	}
	known := map[string]bool{}
	for _, id := range turnIDs {
		if id != "" {
			known[id] = true
		}
	}
	reader := bufio.NewReader(io.NewSectionReader(file, source.ProcessStartOffset, end-source.ProcessStartOffset))
	offset := source.ProcessStartOffset
	var previousTime *time.Time
	previousExcluded := false
	for {
		if ctx.Err() != nil {
			report.missing("runtime_log_collection_canceled")
			return report
		}
		start := offset
		line := []byte{}
		oversize := false
		var readErr error
		for {
			var part []byte
			part, readErr = reader.ReadSlice('\n')
			offset += int64(len(part))
			if len(line)+len(part) > 1<<20 {
				oversize = true
				line = nil
			}
			if !oversize {
				line = append(line, part...)
			}
			if readErr != bufio.ErrBufferFull {
				break
			}
		}
		if oversize {
			report.Truncated++
			report.missing("runtime_log_line_too_large")
		} else if len(line) > 0 {
			text := strings.TrimRight(string(line), "\r\n")
			timestamp := runtimeLogTime(text)
			if timestamp != nil {
				previousTime = timestamp
				previousExcluded = false
			}
			match := runtimeTurnField.FindStringSubmatch(text)
			if len(match) > 1 && !known[match[1]] {
				previousExcluded = true
			} else if !previousExcluded || len(match) > 1 {
				association := "runtime_process_window"
				if len(match) > 1 {
					association = "exact_identity"
					previousExcluded = false
				}
				at := timestamp
				if at == nil && (strings.HasPrefix(text, " ") || strings.HasPrefix(text, "\t")) {
					at = previousTime
				}
				include := at != nil && !at.Before(window.Start) && !at.After(window.End)
				if at == nil && start >= source.TurnStartOffset && (source.TurnEndOffset <= 0 || start < source.TurnEndOffset) {
					include = true
					if association != "exact_identity" {
						association = "runtime_process_byte_range"
					}
					report.missing("runtime_log_timestamps_unavailable")
				}
				if include {
					if association != "exact_identity" {
						report.Unattributed++
					}
					report.Lines++
					if err := yield(RuntimeLogEntry{Source: "runtime.stderr", Association: association, InstanceID: source.InstanceID, PID: source.PID, Offset: start, At: at, Line: ProtectText(text)}); err != nil {
						report.missing("runtime_log_export_failed")
						return report
					}
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			report.missing("runtime_log_read_failed")
			break
		}
	}
	if report.Lines == 0 {
		report.missing("runtime_logs_empty_for_window")
	}
	return report
}
func runtimeLogTime(line string) *time.Time {
	line = strings.TrimSpace(runtimeANSI.ReplaceAllString(line, ""))
	if strings.HasPrefix(line, "[turn_id=") {
		if _, rest, ok := strings.Cut(line, "]"); ok {
			line = strings.TrimSpace(rest)
		}
	}
	var event struct {
		Timestamp string `json:"timestamp"`
		Time      string `json:"time"`
		At        string `json:"at"`
	}
	if strings.HasPrefix(line, "{") && json.Unmarshal([]byte(line), &event) == nil {
		if event.Time == "" {
			event.Time = event.Timestamp
			if event.Time == "" {
				event.Time = event.At
			}
		}
		if at, err := time.Parse(time.RFC3339Nano, event.Time); err == nil {
			return &at
		}
	}
	if match := runtimeTimestamp.FindStringSubmatch(strings.TrimSpace(runtimeANSI.ReplaceAllString(line, ""))); len(match) > 1 {
		if at, err := time.Parse(time.RFC3339Nano, match[1]); err == nil {
			return &at
		}
	}
	return nil
}
