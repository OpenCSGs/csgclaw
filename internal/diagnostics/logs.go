package diagnostics

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type LogEntry struct {
	At      time.Time      `json:"at"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	TurnID  string         `json:"turn_id,omitempty"`
	Fields  map[string]any `json:"fields,omitempty"`
	bytes   int
}
type LogWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}
type LogJournal struct {
	started        time.Time
	history        logCoverageState
	mu             sync.Mutex
	disk           sync.Mutex
	dir            string
	pending        []LogEntry
	pendingBytes   int
	writing        bool
	submitted      uint64
	completed      uint64
	active         map[string]bool
	lastPrune      time.Time
	issuePersisted bool
	dropped        atomic.Bool
	sizes          map[string]int64
	total          int64
	memory         map[string][]byte
	limit          int64
}

var currentLogJournal atomic.Pointer[LogJournal]

func CurrentLogs() *LogJournal { return currentLogJournal.Load() }
func SetCurrentLogs(journal *LogJournal) func() {
	old := currentLogJournal.Swap(journal)
	return func() { currentLogJournal.Store(old) }
}

func NewLogJournal(dir string) *LogJournal {
	j := &LogJournal{started: time.Now().UTC(), dir: dir, sizes: map[string]int64{}, memory: map[string][]byte{}, limit: LogBytes, active: map[string]bool{}}
	if dir != "" {
		if info, err := os.Stat(filepath.Join(dir, ".incomplete")); err == nil {
			if time.Since(info.ModTime()) <= Retention {
				j.dropped.Store(true)
				j.issuePersisted = true
			} else {
				_ = os.Remove(filepath.Join(dir, ".incomplete"))
			}
		}
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".gz") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if time.Since(info.ModTime()) > Retention {
				_ = os.Remove(filepath.Join(dir, entry.Name()))
				continue
			}
			j.sizes[entry.Name()] = info.Size()
			j.total += info.Size()
		}
	}
	j.loadCoverage()
	return j
}
func logTurnKey(turn string) string {
	sum := sha256.Sum256([]byte(turn))
	return "turn-" + hex.EncodeToString(sum[:]) + ".gz"
}
func (j *LogJournal) Add(entry LogEntry) {
	if j == nil {
		return
	}
	// Serialize before returning so callers can reuse attribute maps safely.
	var copy LogEntry
	raw := Protect(entry)
	if len(raw) > 1<<20 {
		j.dropped.Store(true)
		j.Add(LogEntry{At: entry.At, TurnID: entry.TurnID, Level: "WARN", Message: "diagnostic log event exceeded capture limit"})
		return
	}
	_ = json.Unmarshal(raw, &copy)
	copy.bytes = len(raw)
	j.mu.Lock()
	if len(j.pending) >= 4096 || j.pendingBytes+copy.bytes > 32<<20 {
		j.dropped.Store(true)
		j.mu.Unlock()
		return
	}
	j.pending = append(j.pending, copy)
	j.pendingBytes += copy.bytes
	j.submitted++
	if !j.writing {
		j.writing = true
		go j.writePending()
	}
	j.mu.Unlock()
}
func (j *LogJournal) writePending() {
	for {
		j.mu.Lock()
		if len(j.pending) == 0 {
			j.writing = false
			j.mu.Unlock()
			return
		}
		count := min(len(j.pending), 64)
		batch := append([]LogEntry(nil), j.pending[:count]...)
		for _, entry := range batch {
			j.pendingBytes -= entry.bytes
		}
		clear(j.pending[:count])
		j.pending = j.pending[count:]
		j.mu.Unlock()
		groups := map[string][]LogEntry{}
		for _, entry := range batch {
			key := "system-" + entry.At.UTC().Format("20060102T15") + ".gz"
			if entry.TurnID != "" {
				key = logTurnKey(entry.TurnID)
			}
			groups[key] = append(groups[key], entry)
		}
		for key, entries := range groups {
			var data bytes.Buffer
			writer := captureCompressors.Get().(*gzip.Writer)
			writer.Reset(&data)
			encoder := json.NewEncoder(writer)
			for _, entry := range entries {
				_ = encoder.Encode(entry)
			}
			_ = writer.Close()
			captureCompressors.Put(writer)
			j.disk.Lock()
			if err := j.appendLocked(key, data.Bytes()); err != nil {
				j.dropped.Store(true)
			}
			j.disk.Unlock()
		}
		j.persistIssue()
		j.mu.Lock()
		j.completed += uint64(count)
		j.mu.Unlock()
	}
}
func (j *LogJournal) appendLocked(key string, data []byte) error {
	if j.dir != "" && time.Since(j.lastPrune) > time.Hour {
		j.lastPrune = time.Now()
		for name := range j.sizes {
			info, err := os.Stat(filepath.Join(j.dir, name))
			if err == nil && time.Since(info.ModTime()) > Retention && !j.active[name] {
				if os.Remove(filepath.Join(j.dir, name)) == nil {
					j.total -= j.sizes[name]
					delete(j.sizes, name)
				}
			}
		}
	}

	if j.total+int64(len(data)) > j.limit {
		keys := make([]string, 0, len(j.sizes))
		for name := range j.sizes {
			if name != key && !j.active[name] {
				keys = append(keys, name)
			}
		}
		sort.Slice(keys, func(a, b int) bool {
			if j.dir == "" {
				return keys[a] < keys[b]
			}
			x, _ := os.Stat(filepath.Join(j.dir, keys[a]))
			y, _ := os.Stat(filepath.Join(j.dir, keys[b]))
			if x == nil || y == nil {
				return keys[a] < keys[b]
			}
			return x.ModTime().Before(y.ModTime())
		})
		for _, name := range keys {
			if j.total+int64(len(data)) <= j.limit {
				break
			}
			if j.dir != "" {
				if err := os.Remove(filepath.Join(j.dir, name)); err != nil && !os.IsNotExist(err) {
					continue
				}
			}
			j.total -= j.sizes[name]
			j.dropped.Store(true)
			delete(j.sizes, name)
			delete(j.memory, name)
		}
	}
	if j.total+int64(len(data)) > j.limit {
		return fmt.Errorf("log capacity reached")
	}
	if j.dir == "" {
		j.memory[key] = append(j.memory[key], data...)
		j.sizes[key] += int64(len(data))
		j.total += int64(len(data))
		return nil
	}
	if err := os.MkdirAll(j.dir, 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(j.dir, key), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, err := file.Write(data)
	closeErr := file.Close()
	j.sizes[key] += int64(n)
	j.total += int64(n)
	if err != nil {
		return err
	}
	return closeErr
}
func (j *LogJournal) Flush(ctx context.Context) error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	target := j.submitted
	j.mu.Unlock()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		j.mu.Lock()
		done := j.completed >= target
		j.mu.Unlock()
		if done {
			j.checkpointCoverage()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}
func (j *LogJournal) Read(ctx context.Context, turn string, windows []LogWindow, yield func(LogEntry) error) error {
	if j == nil {
		return fmt.Errorf("server_logs_not_collected")
	}
	if err := j.Flush(ctx); err != nil {
		return err
	}
	keys := map[string]bool{}
	if turn != "" {
		keys[logTurnKey(turn)] = true
	} else {
		for _, window := range windows {
			for at := window.Start.UTC().Truncate(time.Hour); !at.After(window.End); at = at.Add(time.Hour) {
				keys["system-"+at.Format("20060102T15")+".gz"] = true
			}
		}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		if err := ctx.Err(); err != nil {
			return err
		}
		j.disk.Lock()
		knownSize := j.sizes[key]
		var source io.Reader
		var file *os.File
		if j.dir == "" {
			source = bytes.NewReader(append([]byte(nil), j.memory[key]...))
		} else {
			var err error
			file, err = os.Open(filepath.Join(j.dir, key))
			if err != nil {
				j.disk.Unlock()
				if os.IsNotExist(err) && knownSize == 0 {
					continue
				}
				return err
			}
			info, err := file.Stat()
			if err != nil {
				file.Close()
				j.disk.Unlock()
				return err
			}
			source = io.LimitReader(file, info.Size())
		}
		j.disk.Unlock()
		err := func() error {
			if file != nil {
				defer file.Close()
			}
			reader, err := gzip.NewReader(source)
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			defer reader.Close()
			decoder := json.NewDecoder(reader)
			for {
				var entry LogEntry
				err := decoder.Decode(&entry)
				if err == io.EOF {
					return nil
				}
				if err != nil {
					return err
				}
				include := turn != "" && entry.TurnID == turn
				if turn == "" && entry.TurnID == "" {
					for _, window := range windows {
						if !entry.At.Before(window.Start) && !entry.At.After(window.End) {
							include = true
							break
						}
					}
				}
				if include {
					if err = yield(entry); err != nil {
						return err
					}
				}
			}
		}()
		if err != nil {
			return err
		}
	}
	if j.dropped.Load() {
		return fmt.Errorf("server_logs_incomplete")
	}
	return nil
}

// ServerLogHandler formats each line independently, allowing a literal turn
// prefix before the standard slog timestamp without global mutable turn state.
type ServerLogHandler struct {
	out        io.Writer
	level      slog.Leveler
	journal    *LogJournal
	outputMu   *sync.Mutex
	operations []handlerOperation
}
type handlerOperation struct {
	group string
	attrs []slog.Attr
}

func NewServerLogHandler(out io.Writer, level slog.Leveler, journal *LogJournal) slog.Handler {
	return &ServerLogHandler{out: out, level: level, journal: journal, outputMu: &sync.Mutex{}}
}
func (h *ServerLogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}
func (h *ServerLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	copy := *h
	copy.operations = append(append([]handlerOperation(nil), h.operations...), handlerOperation{attrs: append([]slog.Attr(nil), attrs...)})
	return &copy
}
func (h *ServerLogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	copy := *h
	copy.operations = append(append([]handlerOperation(nil), h.operations...), handlerOperation{group: name})
	return &copy
}
func (h *ServerLogHandler) Handle(ctx context.Context, record slog.Record) error {
	var encoded bytes.Buffer
	var jsonHandler slog.Handler = slog.NewJSONHandler(&encoded, &slog.HandlerOptions{Level: h.level})
	for _, op := range h.operations {
		if op.group != "" {
			jsonHandler = jsonHandler.WithGroup(op.group)
		} else {
			jsonHandler = jsonHandler.WithAttrs(op.attrs)
		}
	}
	_ = jsonHandler.Handle(ctx, record)
	fields := map[string]any{}
	_ = json.Unmarshal(encoded.Bytes(), &fields)
	turn, _ := fields["turn_id"].(string)
	if resolved := logRecord(ctx); resolved != nil {
		resolved.mu.Lock()
		turn = resolved.data.TurnID
		resolved.mu.Unlock()
	}
	if turn == "" {
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "turn_id" {
				turn = attr.Value.String()
			}
			return true
		})
	}
	delete(fields, "time")
	delete(fields, "level")
	delete(fields, "msg")
	delete(fields, "turn_id")
	entry := LogEntry{At: record.Time, Level: record.Level.String(), Message: record.Message, TurnID: turn, Fields: fields}
	if entry.At.IsZero() {
		entry.At = time.Now().UTC()
	}
	h.journal.Add(entry)
	var line bytes.Buffer
	var textHandler slog.Handler = slog.NewTextHandler(&line, &slog.HandlerOptions{Level: h.level, ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
		if secretField(attr.Key) {
			return slog.String(attr.Key, "[redacted]")
		}
		if attr.Value.Kind() == slog.KindAny {
			if err, ok := attr.Value.Any().(error); ok {
				return slog.String(attr.Key, ProtectText(err.Error()))
			}
		}
		if attr.Value.Kind() == slog.KindString {
			return slog.String(attr.Key, ProtectText(attr.Value.String()))
		}
		return attr
	}})
	for _, op := range h.operations {
		if op.group != "" {
			textHandler = textHandler.WithGroup(op.group)
		} else {
			textHandler = textHandler.WithAttrs(op.attrs)
		}
	}
	record.Message = ProtectText(record.Message)
	if turn != "" {
		record.AddAttrs(slog.String("turn_id", turn))
	}
	if err := textHandler.Handle(ctx, record); err != nil {
		return err
	}
	h.outputMu.Lock()
	defer h.outputMu.Unlock()
	if turn != "" {
		if _, err := fmt.Fprintf(h.out, "[turn_id=%s] ", strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' || r == ']' {
				return '_'
			}
			return r
		}, turn)); err != nil {
			return err
		}
	}
	_, err := h.out.Write(line.Bytes())
	return err
}

func (j *LogJournal) MarkActive(turn string, active bool) {
	if j == nil {
		return
	}
	j.disk.Lock()
	defer j.disk.Unlock()
	if active {
		j.active[logTurnKey(turn)] = true
	} else {
		delete(j.active, logTurnKey(turn))
	}
}

func (j *LogJournal) persistIssue() {
	if j.dir == "" || j.issuePersisted || !j.dropped.Load() {
		return
	}
	j.disk.Lock()
	defer j.disk.Unlock()
	if os.MkdirAll(j.dir, 0700) == nil && os.WriteFile(filepath.Join(j.dir, ".incomplete"), []byte("server_logs_incomplete\n"), 0600) == nil {
		j.issuePersisted = true
	}
}
