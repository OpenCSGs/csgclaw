package diagnostics

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/version"
)

// ContextBytes and LogBytes are independent of the small timeline budget.
const ContextBytes int64 = 1 << 30
const LogBytes int64 = 1 << 30
const maxCaptureQueueBytes = 256 << 20

type ContextEntry struct {
	At       time.Time       `json:"at"`
	Sequence uint64          `json:"sequence"`
	Kind     string          `json:"kind"`
	Data     json.RawMessage `json:"data"`
}

type captureWrite struct {
	id     string
	data   []byte
	record *Record
}
type contextStore struct {
	mu           sync.Mutex
	disk         sync.Mutex
	dir          string
	pending      []captureWrite
	pendingBytes int
	writing      bool
	submitted    uint64
	completed    uint64
	memory       map[string][]byte
	sizes        map[string]int64
	total        int64
	limit        int64
	lastPrune    time.Time
}

func newContextStore(dir string) *contextStore {
	c := &contextStore{dir: dir, memory: map[string][]byte{}, sizes: map[string]int64{}, limit: ContextBytes}
	if dir != "" {
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl.gz") {
				info, err := entry.Info()
				if err != nil {
					continue
				}
				id := strings.TrimSuffix(entry.Name(), ".jsonl.gz")
				if time.Since(info.ModTime()) > Retention {
					_ = os.Remove(filepath.Join(dir, entry.Name()))
					continue
				}
				c.sizes[id] = info.Size()
				c.total += info.Size()
			}
		}
	}
	return c
}

var hostDetails struct {
	once    sync.Once
	version string
}

func Environment() map[string]any {
	hostDetails.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var output []byte
		switch runtime.GOOS {
		case "darwin":
			output, _ = exec.CommandContext(ctx, "/usr/bin/sw_vers", "-productVersion").Output()
		case "windows":
			output, _ = exec.CommandContext(ctx, "cmd", "/c", "ver").Output()
		default:
			output, _ = exec.CommandContext(ctx, "uname", "-sr").Output()
		}
		hostDetails.version = strings.TrimSpace(string(output))
	})
	executable, _ := os.Executable()
	return map[string]any{"captured_at": time.Now().UTC(), "version": version.Current(), "commit": version.Commit, "build_time": version.BuildTime, "os": runtime.GOOS, "os_version": hostDetails.version, "arch": runtime.GOARCH, "go_version": runtime.Version(), "cpus": runtime.NumCPU(), "executable": executable, "proxy": map[string]any{"http": ProtectText(os.Getenv("HTTP_PROXY")), "https": ProtectText(os.Getenv("HTTPS_PROXY")), "all": ProtectText(os.Getenv("ALL_PROXY")), "no_proxy": ProtectText(os.Getenv("NO_PROXY"))}}
}
func (s *Store) SetEnvironment(value map[string]any) {
	raw := Protect(value)
	s.mu.Lock()
	s.environment = raw
	s.mu.Unlock()
}
func (s *Store) environmentSnapshot() map[string]any {
	result := Environment()
	s.mu.Lock()
	raw := append(json.RawMessage(nil), s.environment...)
	s.mu.Unlock()
	var extra map[string]any
	_ = json.Unmarshal(raw, &extra)
	for key, value := range extra {
		result[key] = value
	}
	return result
}

// Protect preserves full diagnostic text. The timeline's Redact deliberately
// truncates strings and must never be used to serialize complete context.
func Protect(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{"unavailable":"unsupported diagnostic value"}`)
	}
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&decoded) != nil {
		return json.RawMessage(`null`)
	}
	encoded, _ := json.Marshal(protectValue(decoded, ""))
	return encoded
}

func secretField(key string) bool {
	key = strings.ToLower(strings.NewReplacer("-", "", "_", "", ".", "").Replace(key))
	for _, word := range []string{"authorization", "cookie", "apikey", "accesstoken", "refreshtoken", "idtoken", "clientsecret", "password", "credential", "privatekey", "signingsecret"} {
		if strings.Contains(key, word) {
			return true
		}
	}
	return strings.HasSuffix(key, "token") || strings.Contains(key, "secret") || key == "auth" || key == "env" || key == "environmentvariables"
}

func protectValue(value any, key string) any {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
	if normalized == "headers" || normalized == "httpheaders" {
		if headers, ok := value.(map[string]any); ok {
			for name, item := range headers {
				switch strings.ToLower(name) {
				case "content-type", "accept", "user-agent", "x-request-id":
					headers[name] = protectValue(item, "")
				default:
					headers[name] = "[redacted]"
				}
			}
			return headers
		}
		return "[redacted]"
	}
	if secretField(key) {
		return "[redacted]"
	}
	switch v := value.(type) {
	case map[string]any:
		for k, item := range v {
			v[k] = protectValue(item, k)
		}
		return v
	case []any:
		for i, item := range v {
			v[i] = protectValue(item, "")
		}
		return v
	case string:
		if strings.HasPrefix(v, "data:") {
			if header, data, ok := strings.Cut(v, ","); ok && strings.Contains(header, ";base64") && !strings.HasPrefix(header, "data:text/") {
				sum := sha256.Sum256([]byte(data))
				return map[string]any{"content_status": "binary_metadata_only", "media_type": strings.TrimPrefix(strings.TrimSuffix(header, ";base64"), "data:"), "encoded_bytes": len(data), "encoded_sha256": hex.EncodeToString(sum[:])}
			}
		}
		// Embedded JSON occurs in tool arguments and protocol event payloads.
		var nested any
		if (strings.HasPrefix(strings.TrimSpace(v), "{") || strings.HasPrefix(strings.TrimSpace(v), "[")) && json.Unmarshal([]byte(v), &nested) == nil {
			raw, _ := json.Marshal(protectValue(nested, ""))
			return string(raw)
		}
		return ProtectText(v)
	default:
		return value
	}
}

var privateKeyBlock = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
var cookieHeader = regexp.MustCompile(`(?im)((?:set-cookie|cookie)\s*[:=]\s*)[^\r\n]+`)

func ProtectText(value string) string {
	value = privateKeyBlock.ReplaceAllString(value, "[redacted private key]")
	value = cookieHeader.ReplaceAllString(value, "${1}[redacted]")
	value = bearer.ReplaceAllString(value, "Bearer [redacted]")
	value = credential.ReplaceAllString(value, "$1$2[redacted]")
	value = keyToken.ReplaceAllString(value, "[redacted]")
	value = urlCredentials.ReplaceAllString(value, "${1}[redacted]@")
	return strings.ToValidUTF8(value, "�")
}

func (r *Record) Issue(reason string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	for _, old := range r.data.CollectionIssues {
		if old == reason {
			r.mu.Unlock()
			return
		}
	}
	r.data.CollectionIssues = append(r.data.CollectionIssues, reason)
	r.mu.Unlock()
	r.store.schedule(r)
}

func (r *Record) Capture(kind string, value any) { r.captureAt(kind, value, time.Now().UTC()) }
func (r *Record) captureAt(kind string, value any, at time.Time) {
	if r == nil || r.store == nil {
		return
	}
	r.captureMu.Lock()
	defer r.captureMu.Unlock()
	r.captureSequence++
	entry := ContextEntry{At: at, Sequence: r.captureSequence, Kind: kind, Data: Protect(value)}
	raw, err := json.Marshal(entry)
	if err != nil {
		r.Issue("context_encode_failed")
		return
	}
	raw = append(raw, '\n')
	r.store.context.enqueue(captureWrite{id: r.data.ID, data: raw, record: r})
}

func (c *contextStore) enqueue(item captureWrite) {
	c.mu.Lock()
	if c.pendingBytes+len(item.data) > maxCaptureQueueBytes {
		c.mu.Unlock()
		item.record.Issue("context_queue_full")
		return
	}
	c.pending = append(c.pending, item)
	c.submitted++
	c.pendingBytes += len(item.data)
	if !c.writing {
		c.writing = true
		go c.writePending()
	}
	c.mu.Unlock()
}

var captureCompressors = sync.Pool{New: func() any { writer, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed); return writer }}

func (c *contextStore) writePending() {
	for {
		c.mu.Lock()
		if len(c.pending) == 0 {
			c.writing = false
			c.mu.Unlock()
			return
		}
		count := min(len(c.pending), 64)
		batch := append([]captureWrite(nil), c.pending[:count]...)
		clear(c.pending[:count])
		c.pending = c.pending[count:]
		for _, item := range batch {
			c.pendingBytes -= len(item.data)
		}
		c.mu.Unlock()
		grouped := map[string][]captureWrite{}
		for _, item := range batch {
			grouped[item.id] = append(grouped[item.id], item)
		}
		for _, items := range grouped {
			var compressed bytes.Buffer
			writer := captureCompressors.Get().(*gzip.Writer)
			writer.Reset(&compressed)
			var err error
			for _, item := range items {
				if _, err = writer.Write(item.data); err != nil {
					break
				}
			}
			if closeErr := writer.Close(); err == nil {
				err = closeErr
			}
			captureCompressors.Put(writer)
			if err == nil {
				err = c.append(items[0], compressed.Bytes())
			}
			if err != nil {
				items[0].record.Issue("context_storage_unavailable")
			}
		}
		c.mu.Lock()
		c.completed += uint64(count)
		c.mu.Unlock()
	}
}

func (c *contextStore) append(item captureWrite, data []byte) error {
	c.disk.Lock()
	defer c.disk.Unlock()
	// Deletion can race queued writes. A removed record must never be recreated.
	item.record.store.mu.Lock()
	current := item.record.store.records[item.id]
	item.record.store.mu.Unlock()
	if current != item.record {
		return nil
	}
	if time.Since(c.lastPrune) > time.Hour || c.total+int64(len(data)) > c.limit {
		c.lastPrune = time.Now()
		c.pruneLocked(item.record.store, int64(len(data)))
	}
	if c.total+int64(len(data)) > c.limit {
		return errors.New("context capacity reached")
	}
	if c.dir == "" {
		c.memory[item.id] = append(c.memory[item.id], data...)
	} else {
		if err := os.MkdirAll(c.dir, 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(filepath.Join(c.dir, item.id+".jsonl.gz"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		n, writeErr := f.Write(data)
		closeErr := f.Close()
		c.sizes[item.id] += int64(n)
		c.total += int64(n)
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	c.sizes[item.id] += int64(len(data))
	c.total += int64(len(data))
	return nil
}

func (c *contextStore) pruneLocked(store *Store, needed int64) {
	type candidate struct {
		id string
		at time.Time
	}
	candidates := []candidate{}
	store.mu.Lock()
	for id := range c.sizes {
		record := store.records[id]
		if record == nil {
			candidates = append(candidates, candidate{id, time.Time{}})
			continue
		}
		snap := record.summary()
		if snap.Status != "running" && snap.Status != "queued" {
			candidates = append(candidates, candidate{id, snap.StartedAt})
		}
	}
	store.mu.Unlock()
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].at.Before(candidates[j].at) })
	for _, candidate := range candidates {
		if time.Since(candidate.at) <= Retention && c.total+needed <= c.limit {
			break
		}
		c.deleteLocked(candidate.id)
	}
}

func (c *contextStore) deleteLocked(id string) {
	if c.dir != "" {
		if err := os.Remove(filepath.Join(c.dir, id+".jsonl.gz")); err != nil && !os.IsNotExist(err) {
			return
		}
	}
	c.total -= c.sizes[id]
	delete(c.sizes, id)
	delete(c.memory, id)
}
func (c *contextStore) delete(id string) { c.disk.Lock(); defer c.disk.Unlock(); c.deleteLocked(id) }

func (c *contextStore) flush(ctx context.Context) error {
	c.mu.Lock()
	target := c.submitted
	c.mu.Unlock()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		c.mu.Lock()
		done := c.completed >= target
		c.mu.Unlock()
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

// ReadContext snapshots the committed file length, then decodes entries one at
// a time. It never loads a complete turn journal into memory.
func (s *Store) ReadContext(ctx context.Context, id string, yield func(ContextEntry) error) error {
	if err := s.context.flush(ctx); err != nil {
		return err
	}
	c := s.context
	c.disk.Lock()
	var source io.Reader
	var file *os.File
	if c.dir == "" {
		source = bytes.NewReader(append([]byte(nil), c.memory[id]...))
	} else {
		var err error
		file, err = os.Open(filepath.Join(c.dir, id+".jsonl.gz"))
		if err != nil {
			c.disk.Unlock()
			return err
		}
		info, err := file.Stat()
		if err != nil {
			file.Close()
			c.disk.Unlock()
			return err
		}
		source = io.LimitReader(file, info.Size())
	}
	c.disk.Unlock()
	if file != nil {
		defer file.Close()
	}
	reader, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer reader.Close()
	decoder := json.NewDecoder(reader)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var entry ContextEntry
		err := decoder.Decode(&entry)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err = yield(entry); err != nil {
			return err
		}
	}
}

// Capture resolves native identity again at completion to cover the protocol
// start-response race, including a turn that has just completed.
func Capture(ctx context.Context, kind string, value any) bool {
	record := ResolveRecord(ctx)
	if record != nil {
		record.Capture(kind, value)
		return true
	}
	if ctx != nil && deferrableFailureKind(kind) {
		if lookup, ok := ctx.Value(nativeLookupKey{}).(nativeLookup); ok && lookup.store != nil {
			return lookup.store.deferNativeFailure(lookup.agent, lookup.session, lookup.turn, kind, value)
		}
	}
	return false
}

func (r *Record) Task(id string, attempt int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.data.TaskID = id
	r.data.TaskAttempt = attempt
	r.mu.Unlock()
	r.store.schedule(r)
}

func WithNativeIdentity(ctx context.Context, session, turn string) context.Context {
	lookup, ok := ctx.Value(nativeLookupKey{}).(nativeLookup)
	if !ok {
		return ctx
	}
	lookup.session = session
	lookup.turn = turn
	return context.WithValue(ctx, nativeLookupKey{}, lookup)
}

type ingressIDKey struct{}

func WithIngressID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ingressIDKey{}, id)
}
func IngressID(ctx context.Context) string { id, _ := ctx.Value(ingressIDKey{}).(string); return id }
