package diagnostics

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// SpanDetails contains bounded display fields, never model prompts or outputs.
type SpanDetails struct {
	Source             string   `json:"source,omitempty"`
	Category           string   `json:"category,omitempty"`
	TraceID            string   `json:"trace_id,omitempty"`
	ParentSpanID       string   `json:"parent_span_id,omitempty"`
	ReportedDurationMS *float64 `json:"reported_duration_ms,omitempty"`
	ExitCode           *int     `json:"exit_code,omitempty"`
	Label              string   `json:"label,omitempty"`
	Command            string   `json:"command,omitempty"`
	Directory          string   `json:"directory,omitempty"`
	Arguments          string   `json:"arguments,omitempty"`
	EventType          string   `json:"event_type,omitempty"`
	HTTPStatus         int      `json:"http_status,omitempty"`
	FirstResponseMS    *float64 `json:"first_response_ms,omitempty"`
}

func displayText(s string) string {
	s = Redact(s)
	if len(s) > 1024 {
		s = s[:1024] + "…"
	}
	return strings.ToValidUTF8(s, "�")
}
func (r *Record) Annotate(id string, details SpanDetails) {
	if r == nil || id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	index, ok := r.active[id]
	if !ok {
		index = -1
		for i := range r.data.Spans {
			if r.data.Spans[i].ID == id {
				index = i
				break
			}
		}
	}
	if index < 0 {
		return
	}
	{
		i := index
		v := SpanDetails{}
		if old := r.data.Spans[i].Details; old != nil {
			v = *old
		}
		if details.Label != "" {
			v.Label = displayText(details.Label)
		}
		if details.Command != "" {
			v.Command = displayText(details.Command)
		}
		if details.Directory != "" {
			v.Directory = displayText(details.Directory)
		}
		if details.Arguments != "" {
			v.Arguments = displayText(details.Arguments)
		}
		if details.EventType != "" {
			v.EventType = displayText(details.EventType)
		}
		if details.ReportedDurationMS != nil {
			n := *details.ReportedDurationMS
			v.ReportedDurationMS = &n
		}
		if details.ExitCode != nil {
			n := *details.ExitCode
			v.ExitCode = &n
		}
		if details.HTTPStatus != 0 {
			v.HTTPStatus = details.HTTPStatus
		}
		if details.FirstResponseMS != nil {
			value := *details.FirstResponseMS
			v.FirstResponseMS = &value
		}
		oldBytes, _ := json.Marshal(r.data.Spans[i].Details)
		newBytes, _ := json.Marshal(v)
		delta := len(newBytes) - len(oldBytes)
		if r.detailBytes+delta > 256<<10 {
			r.data.Incomplete = true
			return
		}
		r.detailBytes += delta
		r.data.Spans[i].Details = &v
		return
	}
}
func ToolDetails(title, input string, payload any) SpanDetails {
	d := SpanDetails{Label: displayText(title)}
	p, _ := payload.(map[string]any)
	if n, ok := displayNumber(p["durationMs"]); ok && n >= 0 {
		d.ReportedDurationMS = &n
	}
	if n, ok := displayNumber(p["exitCode"]); ok && n == math.Trunc(n) && n >= -2147483648 && n <= 2147483647 {
		code := int(n)
		d.ExitCode = &code
	}
	for _, entry := range []map[string]any{p, object(p["rawInput"]), object(p["arguments"])} {
		for _, k := range []string{"command", "cmd"} {
			if text, ok := entry[k].(string); ok {
				d.Command = displayText(text)
			}
		}
		for _, k := range []string{"cwd", "workdir"} {
			if text, ok := entry[k].(string); ok {
				d.Directory = displayText(text)
			}
		}
	}
	var args any
	if p["arguments"] != nil {
		args = p["arguments"]
	} else if p["rawInput"] != nil {
		args = p["rawInput"]
	} else if input != "" {
		args = input
	}
	if text, ok := args.(string); ok {
		var decoded any
		if json.Unmarshal([]byte(text), &decoded) == nil {
			args = decoded
		} else {
			d.Arguments = displayText(text)
			return d
		}
	}
	if args != nil {
		if raw, err := json.Marshal(safeArguments(args, 0)); err == nil {
			d.Arguments = displayText(string(raw))
		}
	}
	return d
}
func object(value any) map[string]any {
	if s, ok := value.(string); ok {
		var v map[string]any
		_ = json.Unmarshal([]byte(s), &v)
		return v
	}
	v, _ := value.(map[string]any)
	return v
}
func safeArguments(value any, depth int) any {
	if depth > 3 {
		return "[truncated]"
	}
	switch v := value.(type) {
	case map[string]any:
		out := map[string]any{}
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for i, key := range keys {
			if i >= 20 {
				out["..."] = "[truncated]"
				break
			}
			lower := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
			secret := false
			for _, word := range []string{"password", "secret", "token", "authorization", "api_key", "apikey", "headers", "credential"} {
				if strings.Contains(lower, word) {
					secret = true
				}
			}
			switch lower {
			case "env", "environment", "prompt", "messages", "content", "data", "image", "base64":
				secret = true
			}
			if secret {
				out[key] = "[redacted]"
			} else {
				out[key] = safeArguments(v[key], depth+1)
			}
		}
		return out
	case []any:
		out := []any{}
		for i, item := range v {
			if i == 10 {
				out = append(out, "[truncated]")
				break
			}
			out = append(out, safeArguments(item, depth+1))
		}
		return out
	case string:
		return displayText(v)
	case nil, bool, float64, int, int64, json.Number:
		return v
	default:
		return "[omitted]"
	}
}

type nativeLookup struct {
	store                *Store
	agent, session, turn string
}
type nativeLookupKey struct{}

func WithNativeLookup(ctx context.Context, store *Store, agent, session, turn string) context.Context {
	return context.WithValue(ctx, nativeLookupKey{}, nativeLookup{store, agent, session, turn})
}
func Resolve(ctx context.Context) *Record {
	if r := From(ctx); r != nil {
		return r
	}
	lookup, ok := ctx.Value(nativeLookupKey{}).(nativeLookup)
	if !ok || lookup.store == nil {
		return nil
	}
	return lookup.store.Native(lookup.agent, lookup.session, lookup.turn)
}

// Native requires exact identity; never guess the most recent turn of an Agent.
func (s *Store) Native(agent, session, turn string) *Record {
	if agent == "" || session == "" || turn == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var found *Record
	for _, r := range s.records {
		r.mu.Lock()
		matches := r.data.Status == "running" && r.data.AgentID == agent && r.data.RuntimeSessionID == session && r.data.RuntimeTurnID == turn
		r.mu.Unlock()
		if matches {
			if found != nil {
				return nil
			}
			found = r
		}
	}
	return found
}

// ObserveRequest can bind after the native start response, avoiding an HTTP /
// protocol-reader race without ever assigning another turn's request.
func ObserveRequest(ctx context.Context, operation string) (func(int, *float64), func(string)) {
	var mu sync.Mutex
	started := time.Now()
	record := Resolve(ctx)
	id := record.Start("llm.request", "llm", "")
	details := SpanDetails{Label: operation}
	record.Annotate(id, details)
	update := func(status int, first *float64) {
		mu.Lock()
		defer mu.Unlock()
		details.HTTPStatus = status
		details.FirstResponseMS = first
		record.Annotate(id, details)
	}
	finish := func(status string) {
		mu.Lock()
		defer mu.Unlock()
		if record == nil {
			record = Resolve(ctx)
			if record == nil {
				return
			}
			record.mu.Lock()
			if len(record.data.Spans) < maxSpans {
				end := ms(time.Since(record.origin))
				id = "llm-" + started.Format("150405.000000000")
				record.data.Spans = append(record.data.Spans, Span{ID: id, Name: "llm.request", Owner: "llm", StartMS: ms(started.Sub(record.origin)), EndMS: &end, Status: status})
			} else {
				record.data.Incomplete = true
			}
			record.mu.Unlock()
			record.Annotate(id, details)
		} else {
			record.End(id, status)
		}
		record.store.schedule(record)
	}
	return update, finish
}

func Enabled(ctx context.Context) bool {
	return From(ctx) != nil || ctx.Value(nativeLookupKey{}) != nil
}

func displayNumber(value any) (float64, bool) {
	var n float64
	switch v := value.(type) {
	case float64:
		n = v
	case int:
		n = float64(v)
	case int64:
		n = float64(v)
	case json.Number:
		var err error
		n, err = v.Float64()
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return n, !math.IsNaN(n) && !math.IsInf(n, 0)
}
