// Package catalog coordinates worker discovery without giving public sources
// access to the authenticated Hub client or credentials.
package catalog

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"csgclaw/internal/skill/remote"
)

const (
	Global = "global"
	Site   = "site"
	Public = "skills_sh"
	Limit  = 12
)

var ErrDisabled = errors.New("public skill discovery is disabled")
var errUnsupportedResults = errors.New("no installable GitHub skills in search results")

type Candidate struct {
	Source      string `json:"source"`
	Name        string `json:"name"`
	Key         string `json:"key"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
	Ref         string `json:"ref,omitempty"`
	Version     string `json:"version,omitempty"`
	Site        string `json:"site,omitempty"`
	Selection   string `json:"selection,omitempty"`
}

type Result struct {
	Candidates []Candidate       `json:"candidates"`
	Sources    map[string]string `json:"sources"`
}

type Lookup func(context.Context, string) ([]Candidate, error)

// Search stops on a local match; results and failures from independent remote
// catalogs are retained in stable order regardless of completion order.
func Search(ctx context.Context, query, source string, local, site, public Lookup) (Result, error) {
	out := Result{Candidates: []Candidate{}, Sources: map[string]string{}}
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 512 {
		return out, errors.New("a search query of 1–512 bytes is required")
	}
	if source != "" && source != Global && source != Site && source != Public {
		return out, errors.New("unknown skill source")
	}
	if local != nil {
		items, err := local(ctx, query)
		if err != nil {
			out.Sources[Global] = "unavailable"
		} else {
			out.Sources[Global] = "ok"
			out.Candidates = append(out.Candidates, items...)
		}
		if len(out.Candidates) > 0 || source == Global {
			return out, nil
		}
	}
	type batch struct {
		name  string
		items []Candidate
		err   error
	}
	results := make(chan batch, 2)
	count := 0
	for key, fn := range map[string]Lookup{Site: site, Public: public} {
		if source != "" && source != key {
			continue
		}
		count++
		go func() {
			child, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			items, err := fn(child, query)
			results <- batch{key, items, err}
		}()
	}
	bySource := map[string][]Candidate{}
	for range count {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case b := <-results:
			status := "ok"
			switch {
			case b.err == nil:
				bySource[b.name] = b.items
			case errors.Is(b.err, remote.ErrAgenticHubSignInRequired):
				status = "login_required"
			case errors.Is(b.err, context.DeadlineExceeded):
				status = "timeout"
			case errors.Is(b.err, remote.ErrRateLimited):
				status = "rate_limited"
			case errors.Is(b.err, ErrDisabled):
				status = "disabled"
			case errors.Is(b.err, errUnsupportedResults):
				status = "unsupported_source"
			default:
				status = "unavailable"
			}
			out.Sources[b.name] = status
		}
	}
	out.Candidates = append(out.Candidates, bySource[Site]...)
	out.Candidates = append(out.Candidates, bySource[Public]...)
	return out, nil
}

// Match searches names and descriptions. Prefer an exact name over a name
// prefix, then require all search terms rather than matching a single word.
func Match(items []Candidate, q string) []Candidate {
	q = strings.ToLower(strings.TrimSpace(q))
	terms := strings.Fields(q)
	type hit struct {
		item Candidate
		rank int
	}
	var hits []hit
	for _, c := range items {
		if strings.HasPrefix(q, "https://skills.sh/") && !strings.EqualFold(q, c.URL) {
			continue
		}
		n := strings.ToLower(c.Name)
		text := n + " " + strings.ToLower(c.Description+" "+c.Key+" "+c.URL)
		match := true
		for _, term := range terms {
			if !strings.Contains(text, term) {
				match = false
				break
			}
		}
		if match {
			rank := 2
			if n == q {
				rank = 0
			} else if strings.HasPrefix(n, q) {
				rank = 1
			}
			hits = append(hits, hit{c, rank})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].rank != hits[j].rank {
			return hits[i].rank < hits[j].rank
		}
		return hits[i].item.Key < hits[j].item.Key
	})
	var out []Candidate
	for _, h := range hits[:min(len(hits), Limit)] {
		out = append(out, h.item)
	}
	return out
}
