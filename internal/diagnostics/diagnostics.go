// Package diagnostics records bounded, redacted turn timelines.
// Wall clock times are for display; all offsets use a monotonic origin.
package diagnostics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const Retention = 7 * 24 * time.Hour
const MaxBytes int64 = 256 << 20
const maxSpans = 2048
const maxRecords = 2000

type Span struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Owner   string       `json:"owner"`
	StartMS float64      `json:"start_ms"`
	EndMS   *float64     `json:"end_ms,omitempty"`
	Status  string       `json:"status"`
	Details *SpanDetails `json:"details,omitempty"`
}
type Failure struct {
	Code    string `json:"code"`
	Stage   string `json:"stage"`
	Message string `json:"message"`
}
type Timing struct {
	FirstTextMS *float64   `json:"first_text_ms,omitempty"`
	FirstTextAt *time.Time `json:"first_text_at,omitempty"`
	FirstMS     *float64   `json:"first_ms,omitempty"`
	CompleteMS  *float64   `json:"complete_ms,omitempty"`
}
type Snapshot struct {
	ID               string    `json:"id"`
	RoomID           string    `json:"room_id"`
	SourceID         string    `json:"source_id"`
	ThreadID         string    `json:"thread_id,omitempty"`
	AgentID          string    `json:"agent_id"`
	AgentName        string    `json:"agent_name,omitempty"`
	TurnID           string    `json:"turn_id"`
	Runtime          string    `json:"runtime,omitempty"`
	Model            string    `json:"model,omitempty"`
	RuntimeSessionID string    `json:"runtime_session_id,omitempty"`
	RuntimeTurnID    string    `json:"runtime_turn_id,omitempty"`
	RuntimeRequestID string    `json:"runtime_request_id,omitempty"`
	StartedAt        time.Time `json:"started_at"`
	Status           string    `json:"status"`
	TotalMS          float64   `json:"total_ms"`
	RuntimeStartMS   *float64  `json:"runtime_start_ms,omitempty"`
	RuntimeEndMS     *float64  `json:"runtime_end_ms,omitempty"`
	FirstOutputMS    *float64  `json:"first_output_ms,omitempty"`
	Spans            []Span    `json:"spans,omitempty"`
	Error            *Failure  `json:"error,omitempty"`
	Incomplete       bool      `json:"incomplete,omitempty"`
	Browser          *Timing   `json:"browser,omitempty"`
}
type Record struct {
	mu          sync.Mutex
	store       *Store
	origin      time.Time
	data        Snapshot
	active      map[string]int
	detailBytes int
	eventSpans  int
}
type source struct {
	started time.Time
	ready   time.Time
}
type Store struct {
	mu          sync.Mutex
	diskMu      sync.Mutex
	dir         string
	records     map[string]*Record
	sources     map[string]source
	dirty       map[string]*Record
	writing     bool
	lastWarning time.Time
	lastPrune   time.Time
	sizes       map[string]int64
	bytes       int64
}

func New(dir string) *Store {
	s := &Store{dir: dir, records: map[string]*Record{}, sources: map[string]source{}, dirty: map[string]*Record{}, sizes: map[string]int64{}}
	if dir == "" {
		return s
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil || info.Size() > 1<<20 {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var data Snapshot
		if json.Unmarshal(raw, &data) != nil || data.ID != strings.TrimSuffix(e.Name(), ".json") {
			continue
		}
		if time.Since(data.StartedAt) > Retention {
			_ = os.Remove(path)
			continue
		}
		if data.Status == "running" || data.Status == "queued" {
			data.Status = "interrupted"
			data.Incomplete = true
		}
		s.sizes[data.ID] = info.Size()
		s.bytes += info.Size()
		s.records[data.ID] = &Record{store: s, origin: data.StartedAt, data: data, active: map[string]int{}}
	}
	s.prune()
	return s
}
func key(room, sourceID string) string { return room + "\x00" + sourceID }
func (s *Store) Source(room, sourceID string, started time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sources) >= maxRecords {
		for k, v := range s.sources {
			if time.Since(v.ready) > time.Hour {
				delete(s.sources, k)
			}
		}
		if len(s.sources) >= maxRecords {
			for k := range s.sources {
				delete(s.sources, k)
				break
			}
		}
	}
	s.sources[key(room, sourceID)] = source{started: started, ready: time.Now()}
}
func (s *Store) Begin(room, sourceID, thread, agent, turn string) *Record {
	if s == nil {
		return nil
	}
	sum := sha256.Sum256([]byte(key(room, turn)))
	id := hex.EncodeToString(sum[:16])
	s.mu.Lock()
	if r := s.records[id]; r != nil {
		s.mu.Unlock()
		v := r.Snapshot()
		if v.Status != "queued" && v.Status != "running" {
			return nil
		}
		return r
	}
	now := time.Now()
	src, exists := s.sources[key(room, sourceID)]
	if !exists {
		src = source{started: now, ready: now}
	}
	r := &Record{store: s, origin: src.started, active: map[string]int{}, data: Snapshot{ID: id, RoomID: room, SourceID: sourceID, ThreadID: thread, AgentID: agent, TurnID: turn, StartedAt: src.started, Status: "queued", Incomplete: !exists}}
	s.records[id] = r
	s.mu.Unlock()
	r.Interval("message.accept", "csgclaw", src.started, src.ready)
	r.Interval("message.dispatch", "csgclaw", src.ready, now)
	r.Start("channel.queue", "csgclaw", "channel.queue")
	s.schedule(r)
	return r
}
func WithRecord(ctx context.Context, r *Record) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if r == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, r)
}

type contextKey struct{}

func From(ctx context.Context) *Record {
	if ctx == nil {
		return nil
	}
	r, _ := ctx.Value(contextKey{}).(*Record)
	return r
}
func Measure(ctx context.Context, name, owner string) func() {
	r := From(ctx)
	if r == nil {
		return func() {}
	}
	id := r.Start(name, owner, "")
	return func() { r.End(id, "completed") }
}
func (r *Record) Start(name, owner, id string) string {
	if len(name) > 128 {
		name = name[:128]
	}
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.data.Status != "running" && r.data.Status != "queued" {
		return ""
	}
	if id != "" {
		for _, span := range r.data.Spans {
			if span.ID == id && span.EndMS != nil {
				return ""
			}
		}
		if _, ok := r.active[id]; ok {
			return id
		}
	}
	if len(r.data.Spans) >= maxSpans {
		r.data.Incomplete = true
		return ""
	}
	if name == "event.deliver" {
		if r.eventSpans >= maxSpans/2 {
			r.data.Incomplete = true
			return ""
		}
		r.eventSpans++
	}
	if id == "" {
		id = fmt.Sprintf("span-%d", len(r.data.Spans)+1)
	}
	r.active[id] = len(r.data.Spans)
	r.data.Spans = append(r.data.Spans, Span{ID: id, Name: name, Owner: owner, StartMS: ms(time.Since(r.origin)), Status: "running"})
	return id
}
func (r *Record) End(id, status string) {
	if r == nil || id == "" {
		return
	}
	r.mu.Lock()
	if i, ok := r.active[id]; ok {
		end := ms(time.Since(r.origin))
		r.data.Spans[i].EndMS = &end
		r.data.Spans[i].Status = status
		delete(r.active, id)
	}
	r.mu.Unlock()
}
func (r *Record) Interval(name, owner string, start, end time.Time) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.data.Spans) >= maxSpans {
		r.data.Incomplete = true
		return
	}
	e := ms(end.Sub(r.origin))
	r.data.Spans = append(r.data.Spans, Span{ID: fmt.Sprintf("span-%d", len(r.data.Spans)+1), Name: name, Owner: owner, StartMS: ms(start.Sub(r.origin)), EndMS: &e, Status: "completed"})
}
func (r *Record) Running() {
	if r == nil {
		return
	}
	r.End("channel.queue", "completed")
	r.mu.Lock()
	r.data.Status = "running"
	r.mu.Unlock()
}
func (r *Record) AgentName(name string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.data.AgentName = Redact(name)
	r.mu.Unlock()
}

func (r *Record) RuntimeRef(session, turn, request string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if session != "" {
		r.data.RuntimeSessionID = session
	}
	if turn != "" {
		r.data.RuntimeTurnID = turn
	}
	if request != "" {
		r.data.RuntimeRequestID = request
	}
}

func (r *Record) RuntimeInfo(kind, model string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.data.Runtime = kind
	r.data.Model = Redact(model)
	r.mu.Unlock()
}
func (r *Record) RuntimeStart() {
	if r == nil {
		return
	}
	r.mu.Lock()
	v := ms(time.Since(r.origin))
	if r.data.RuntimeStartMS == nil {
		r.data.RuntimeStartMS = &v
	}
	r.data.RuntimeEndMS = nil
	r.mu.Unlock()
}

// RuntimeEndAt must be called with the protocol reader's receive timestamp.
func (r *Record) RuntimeEndAt(at time.Time) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.data.RuntimeStartMS != nil {
		v := ms(at.Sub(r.origin))
		r.data.RuntimeEndMS = &v
	}
	r.mu.Unlock()
}
func (r *Record) FirstOutput() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.data.FirstOutputMS == nil {
		v := ms(time.Since(r.origin))
		r.data.FirstOutputMS = &v
	}
	r.mu.Unlock()
}
func (r *Record) Failure(code, stage, message string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if stage == "execution" && r.data.Error != nil {
		r.mu.Unlock()
		return
	}
	r.data.Error = &Failure{Code: Redact(code), Stage: stage, Message: Redact(message)}
	r.mu.Unlock()
}
func (r *Record) Finish(status string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	v := ms(time.Since(r.origin))
	r.data.TotalMS = v
	r.data.Status = status
	for _, i := range r.active {
		r.data.Spans[i].EndMS = &v
		r.data.Spans[i].Status = status
		if status == "succeeded" {
			r.data.Incomplete = true
			r.data.Spans[i].Status = "interrupted"
		}
	}
	clear(r.active)
	if r.data.RuntimeStartMS != nil && r.data.RuntimeEndMS == nil {
		r.data.Incomplete = true
	}
	r.mu.Unlock()
	r.store.schedule(r)
}
func (r *Record) Snapshot() Snapshot { return r.snapshot(true) }
func (r *Record) summary() Snapshot  { return r.snapshot(false) }
func (r *Record) snapshot(full bool) Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.data
	out.Spans = nil
	if full {
		out.Spans = append([]Span(nil), r.data.Spans...)
	}
	if out.Status == "running" || out.Status == "queued" {
		out.TotalMS = ms(time.Since(r.origin))
	}
	return out
}
func ms(d time.Duration) float64 { return max(0, float64(d)/float64(time.Millisecond)) }
func (s *Store) Get(room, id string) (Snapshot, bool) {
	s.mu.Lock()
	r := s.records[id]
	s.mu.Unlock()
	if r == nil {
		return Snapshot{}, false
	}
	v := r.Snapshot()
	return v, v.RoomID == room && time.Since(v.StartedAt) <= Retention
}
func (s *Store) List(room, sourceID, agent, status, thread string) []Snapshot {
	s.mu.Lock()
	rs := make([]*Record, 0, len(s.records))
	for _, r := range s.records {
		rs = append(rs, r)
	}
	s.mu.Unlock()
	out := []Snapshot{}
	for _, r := range rs {
		v := r.summary()
		if v.RoomID != room || time.Since(v.StartedAt) > Retention || sourceID != "" && v.SourceID != sourceID || agent != "" && v.AgentID != agent || status != "" && v.Status != status || thread != "" && v.ThreadID != thread {
			continue
		}
		v.Spans = nil
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out
}
func (s *Store) BrowserTiming(room, sourceID, turnID string, t Timing) bool {
	s.mu.Lock()
	ok := false
	rs := []*Record{}
	for _, r := range s.records {
		rs = append(rs, r)
	}
	s.mu.Unlock()
	for _, r := range rs {
		r.mu.Lock()
		matches := r.data.RoomID == room && r.data.SourceID == sourceID && (turnID == "" || r.data.TurnID == turnID)
		if matches {
			merged := t
			if old := r.data.Browser; old != nil {
				if old.FirstTextMS != nil && (merged.FirstTextMS == nil || *old.FirstTextMS <= *merged.FirstTextMS) {
					merged.FirstTextMS = old.FirstTextMS
					merged.FirstTextAt = old.FirstTextAt
				}
				if old.FirstMS != nil && (merged.FirstMS == nil || *old.FirstMS < *merged.FirstMS) {
					merged.FirstMS = old.FirstMS
				}
				if old.CompleteMS != nil && (merged.CompleteMS == nil || *old.CompleteMS < *merged.CompleteMS) {
					merged.CompleteMS = old.CompleteMS
				}
			}
			r.data.Browser = &merged
			ok = true
		}
		r.mu.Unlock()
		if matches {
			s.schedule(r)
		}
	}
	return ok
}
func (s *Store) DeleteRoom(room string) {
	s.diskMu.Lock()
	defer s.diskMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, r := range s.records {
		if r.summary().RoomID == room {
			s.bytes -= s.sizes[id]
			delete(s.sizes, id)
			delete(s.records, id)
			delete(s.dirty, id)
			if s.dir != "" {
				_ = os.Remove(filepath.Join(s.dir, id+".json"))
			}
		}
	}
	for k := range s.sources {
		if strings.HasPrefix(k, room+"\x00") {
			delete(s.sources, k)
		}
	}
}
func (s *Store) schedule(r *Record) {
	if s.dir == "" {
		return
	}
	s.mu.Lock()
	s.dirty[r.data.ID] = r
	if !s.writing {
		s.writing = true
		go s.writePending()
	}
	s.mu.Unlock()
}
func (s *Store) writePending() {
	// Coalesce bursts without running a permanent goroutine per Store.
	time.Sleep(50 * time.Millisecond)
	for {
		s.mu.Lock()
		var id string
		var r *Record
		for k, v := range s.dirty {
			id = k
			r = v
			delete(s.dirty, k)
			break
		}
		if r == nil {
			s.writing = false
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		s.diskMu.Lock()
		s.mu.Lock()
		present := s.records[id] == r
		s.mu.Unlock()
		if present {
			data, err := json.Marshal(r.Snapshot())
			if err == nil {
				err = os.MkdirAll(s.dir, 0700)
			}
			if err == nil {
				err = os.WriteFile(filepath.Join(s.dir, id+".tmp"), data, 0600)
			}
			if err == nil {
				err = os.Rename(filepath.Join(s.dir, id+".tmp"), filepath.Join(s.dir, id+".json"))
				if err == nil {
					s.mu.Lock()
					s.bytes += int64(len(data)) - s.sizes[id]
					s.sizes[id] = int64(len(data))
					s.mu.Unlock()
				}
			}
			if err != nil {
				r.mu.Lock()
				r.data.Incomplete = true
				r.mu.Unlock()
				s.mu.Lock()
				if time.Since(s.lastWarning) > time.Minute {
					slog.Warn("turn diagnostics could not be persisted")
					s.lastWarning = time.Now()
				}
				s.mu.Unlock()
			}
		}
		s.diskMu.Unlock()
		s.prune()
	}
}
func (s *Store) prune() {
	s.mu.Lock()
	if time.Since(s.lastPrune) < time.Minute && len(s.records) <= maxRecords && s.bytes <= MaxBytes {
		s.mu.Unlock()
		return
	}
	s.lastPrune = time.Now()
	s.mu.Unlock()
	s.diskMu.Lock()
	defer s.diskMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	type old struct {
		id   string
		at   time.Time
		size int64
	}
	items := []old{}
	var total int64
	for id, r := range s.records {
		v := r.summary()
		size := s.sizes[id]
		total += size
		if v.Status != "running" && v.Status != "queued" {
			items = append(items, old{id, v.StartedAt, size})
		}
	}

	sort.Slice(items, func(i, j int) bool { return items[i].at.Before(items[j].at) })
	for _, v := range items {
		if time.Since(v.at) <= Retention && total <= MaxBytes && len(s.records) <= maxRecords {
			break
		}
		delete(s.records, v.id)
		delete(s.dirty, v.id)
		total -= v.size
		s.bytes -= s.sizes[v.id]
		delete(s.sizes, v.id)
		if s.dir != "" {
			_ = os.Remove(filepath.Join(s.dir, v.id+".json"))
		}
	}
}

var credential = regexp.MustCompile(`(?i)(authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|token|password|secret)(["']?\s*[:=]\s*["']?)([^\s,"';&}]+)`)
var bearer = regexp.MustCompile(`(?i)bearer\s+[a-z0-9._~+/=-]+`)
var keyToken = regexp.MustCompile(`\b(?:sk-|ghp_|github_pat_)[a-zA-Z0-9_-]+`)
var urlCredentials = regexp.MustCompile(`(https?://)[^/\s@]+@`)

func Redact(value string) string {
	value = bearer.ReplaceAllString(value, "Bearer [redacted]")
	value = credential.ReplaceAllString(value, "$1$2[redacted]")
	value = keyToken.ReplaceAllString(value, "[redacted]")
	value = urlCredentials.ReplaceAllString(value, "${1}[redacted]@")
	if len(value) > 4096 {
		value = value[:4096] + "…"
	}
	return strings.ToValidUTF8(value, "�")
}

// Flush waits for pending snapshots during graceful shutdown.
func (s *Store) Flush(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		s.mu.Lock()
		done := !s.writing
		s.mu.Unlock()
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
