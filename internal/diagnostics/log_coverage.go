package diagnostics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Coverage intervals describe an installed collector, not the presence of a
// log message. Empty quiet intervals are different from missing log shards.
type logCoverageState struct {
	Modified map[string]time.Time `json:"modified"`
	Windows  []LogWindow          `json:"windows"`
	Files    map[string]int64     `json:"files"`
}

func (j *LogJournal) loadCoverage() {
	if j.dir == "" {
		return
	}
	raw, err := os.ReadFile(filepath.Join(j.dir, ".coverage.json"))
	if err != nil {
		if !os.IsNotExist(err) {
			j.dropped.Store(true)
		}
		return
	}
	if len(raw) > 8<<20 || json.Unmarshal(raw, &j.history) != nil {
		j.dropped.Store(true)
		return
	}
	for name, size := range j.history.Files {
		if at, ok := j.history.Modified[name]; ok && time.Since(at) > Retention {
			continue
		}
		if filepath.Base(name) != name {
			j.dropped.Store(true)
			continue
		}
		info, err := os.Stat(filepath.Join(j.dir, name))
		if err != nil || info.Size() < size {
			j.dropped.Store(true)
		}
	}
}
func (j *LogJournal) checkpointCoverage() {
	if j == nil || j.dir == "" {
		return
	}
	j.disk.Lock()
	defer j.disk.Unlock()
	windows := append([]LogWindow(nil), j.history.Windows...)
	windows = append(windows, LogWindow{Start: j.started, End: time.Now().UTC()})
	windows = mergeLogWindows(windows)
	modified := map[string]time.Time{}
	for name := range j.sizes {
		if info, err := os.Stat(filepath.Join(j.dir, name)); err == nil {
			modified[name] = info.ModTime()
		} else {
			j.dropped.Store(true)
		}
	}
	state := logCoverageState{Windows: windows, Files: j.sizes, Modified: modified}
	raw, err := json.Marshal(state)
	if err == nil {
		err = os.MkdirAll(j.dir, 0700)
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(j.dir, ".coverage.json.tmp"), raw, 0600)
	}
	if err == nil {
		err = os.Rename(filepath.Join(j.dir, ".coverage.json.tmp"), filepath.Join(j.dir, ".coverage.json"))
	}
	if err != nil {
		j.dropped.Store(true)
	}
}
func mergeLogWindows(windows []LogWindow) []LogWindow {
	cutoff := time.Now().Add(-Retention)
	filtered := windows[:0]
	for _, window := range windows {
		if !window.End.Before(cutoff) && !window.Start.After(window.End) {
			filtered = append(filtered, window)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Start.Before(filtered[j].Start) })
	result := []LogWindow{}
	for _, window := range filtered {
		if len(result) > 0 && !window.Start.After(result[len(result)-1].End) {
			if window.End.After(result[len(result)-1].End) {
				result[len(result)-1].End = window.End
			}
		} else {
			result = append(result, window)
		}
	}
	if len(result) > 512 {
		result = result[len(result)-512:]
	}
	return result
}
func (j *LogJournal) Coverage(windows []LogWindow) []string {
	if j == nil {
		return []string{"server_logs_not_collected"}
	}
	j.disk.Lock()
	available := append([]LogWindow(nil), j.history.Windows...)
	j.disk.Unlock()
	available = mergeLogWindows(append(available, LogWindow{Start: j.started, End: time.Now().UTC()}))
	reasons := []string{}
	for _, requested := range mergeLogWindows(append([]LogWindow(nil), windows...)) {
		covered := false
		for _, window := range available {
			if !window.Start.After(requested.Start) && !window.End.Before(requested.End) {
				covered = true
				break
			}
		}
		if !covered {
			reasons = append(reasons, "system_log_window_not_fully_collected")
			break
		}
	}
	if j.dropped.Load() {
		reasons = append(reasons, "server_logs_incomplete")
	}
	return reasons
}
