package agentmemory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"csgclaw/internal/diagnostics"
)

const (
	maxDocumentBytes = 64 << 10
	maxContextBytes  = 8 << 10
	maxStateBytes    = 128 << 10
)

var (
	ErrConflict    = errors.New("memory revision changed")
	ErrUnavailable = errors.New("memory is unavailable")
	ErrCapacity    = errors.New("memory capacity exceeded")
	ErrInvalid     = errors.New("invalid memory update")
)

type Scope struct {
	AgentID string `json:"agent_id"`
	UserID  string `json:"user_id"`
}

type Source struct {
	RoomID    string `json:"room_id"`
	MessageID string `json:"message_id"`
}

type Message struct {
	Source
	Text string `json:"text"`
}

type Entry struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	Source    *Source   `json:"source,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Failure struct {
	Message   string    `json:"message"`
	Operation string    `json:"operation"`
	At        time.Time `json:"at"`
}

func failureDetails(err error, operation string, now time.Time) *Failure {
	message := cleanText(err.Error())
	if errors.Is(err, context.DeadlineExceeded) {
		message = "Memory extraction timed out. Please retry."
	}
	return &Failure{Message: boundText(message, 1024), Operation: operation, At: now}
}

type Document struct {
	Available      bool     `json:"available"`
	Enabled        bool     `json:"enabled"`
	Ready          bool     `json:"ready"`
	Name           string   `json:"name"`
	Content        string   `json:"content"`
	Entries        []Entry  `json:"entries"`
	Revision       uint64   `json:"revision"`
	Status         string   `json:"status"`
	Initialization string   `json:"initialization,omitempty"`
	LastError      *Failure `json:"last_error,omitempty"`
}

type Update struct {
	Revision *uint64  `json:"revision"`
	Enabled  *bool    `json:"enabled,omitempty"`
	Entries  *[]Entry `json:"entries,omitempty"`
	Content  *string  `json:"content,omitempty"`
	Retry    bool     `json:"retry_initialization,omitempty"`
	Reset    bool     `json:"reset,omitempty"`
}

type Extraction struct {
	Entries  []Entry   `json:"entries"`
	Messages []Message `json:"messages"`
	History  bool      `json:"history,omitempty"`
}

type Change struct {
	EntryID         string `json:"entry_id"`
	Kind            string `json:"kind"`
	Text            string `json:"text"`
	SourceMessageID string `json:"source_message_id"`
}

var headings = []struct{ kind, title string }{
	{"preference", "Preferences"},
	{"convention", "Working agreements"},
	{"fact", "Confirmed facts"},
}

func validKind(kind string) bool {
	for _, heading := range headings {
		if heading.kind == kind {
			return true
		}
	}
	return false
}

func cleanText(value string) string {
	value = strings.TrimSpace(diagnostics.ProtectText(value))
	// Remove terminal/control characters while retaining paragraph breaks.
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, value)
}

func boundText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	for !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit]
}

func newID() string {
	var data [16]byte
	_, _ = rand.Read(data[:])
	return hex.EncodeToString(data[:])
}

func markdown(entries []Entry) string {
	if len(entries) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("# Agent memory\n")
	for _, heading := range headings {
		hasHeading := false
		for _, entry := range entries {
			if entry.Kind != heading.kind {
				continue
			}
			if !hasHeading {
				out.WriteString("\n## " + heading.title + "\n")
				hasHeading = true
			}
			out.WriteString("- " + strings.ReplaceAll(entry.Text, "\n", "\n  ") + "\n")
		}
	}
	return out.String()
}

func contextMarkdown(entries []Entry, query string) string {
	if content := markdown(entries); len(content) <= maxContextBytes {
		return content
	}
	terms := searchTerms(boundText(query, maxContextBytes))
	type candidate struct {
		entry Entry
		hits  int
	}
	ranked := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		lower := strings.ToLower(entry.Text)
		hits := 0
		for term := range terms {
			if strings.Contains(lower, term) {
				hits++
			}
		}
		ranked = append(ranked, candidate{entry, hits})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.hits != b.hits {
			return a.hits > b.hits
		}
		if (a.entry.Kind == "preference") != (b.entry.Kind == "preference") {
			return a.entry.Kind == "preference"
		}
		return a.entry.UpdatedAt.After(b.entry.UpdatedAt)
	})
	var selected []Entry
	for _, item := range ranked {
		entry := item.entry
		if len(markdown([]Entry{entry})) > maxContextBytes {
			// Keep the beginning (which can state scope), and favor the matching
			// part of a long entry for the rest of the excerpt.
			offset := len(boundText(entry.Text, 512))
			lower := strings.ToLower(entry.Text)
			hit := len(lower)
			for term := range terms {
				if position := strings.Index(lower, term); position >= 0 && position < hit {
					hit = position
				}
			}
			if hit < len(lower) {
				offset = min(len(entry.Text)-1, max(0, hit-512))
			}
			for !utf8.RuneStart(entry.Text[offset]) {
				offset--
			}
			prefix := boundText(entry.Text, 512)
			if offset < len(prefix) {
				offset = len(prefix)
			}
			entry.Text = prefix + "\n[excerpt]\n" + boundText(entry.Text[offset:], maxContextBytes/3)
		}
		next := append(selected, entry)
		if len(markdown(next)) <= maxContextBytes {
			selected = next
		}
	}
	return markdown(selected)
}

func searchTerms(text string) map[string]struct{} {
	terms := make(map[string]struct{})
	var word strings.Builder
	var previous rune
	flush := func() {
		if word.Len() > 1 {
			terms[word.String()] = struct{}{}
		}
		word.Reset()
	}
	for _, r := range strings.ToLower(text) {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			flush()
			if previous != 0 {
				terms[string([]rune{previous, r})] = struct{}{}
			}
			previous = r
			continue
		}
		previous = 0
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			word.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return terms
}
