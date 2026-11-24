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
	Installed = "installed"
	Global    = "global"
	Site      = "site"
	Public    = "skills_sh"
	Limit     = 12
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
	Repository  string `json:"repository,omitempty"`
	Version     string `json:"version,omitempty"`
	Site        string `json:"site,omitempty"`
	Selection   string `json:"selection,omitempty"`
}

type Result struct {
	Candidates []Candidate       `json:"candidates"`
	Sources    map[string]string `json:"sources"`
}

type Lookup func(context.Context, string) ([]Candidate, error)

type Lookups struct{ Installed, Global, Site, Public Lookup }

// Search stops at the first hit. An unavailable higher-priority catalog
// isn't a confirmed miss: don't send the query to a less preferred source.
func Search(ctx context.Context, query, source string, lookups Lookups) (Result, error) {
	out := Result{Candidates: []Candidate{}, Sources: map[string]string{}}
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 512 {
		return out, errors.New("a search query of 1–512 bytes is required")
	}
	if source != "" && source != Installed && source != Global && source != Site && source != Public {
		return out, errors.New("unknown skill source")
	}
	for _, step := range []struct {
		key string
		fn  Lookup
	}{
		{Installed, lookups.Installed}, {Global, lookups.Global}, {Site, lookups.Site}, {Public, lookups.Public},
	} {
		if step.fn == nil || !searchStepAllowed(source, step.key) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return out, err
		}
		child := ctx
		cancel := func() {}
		if step.key == Site || step.key == Public {
			child, cancel = context.WithTimeout(ctx, 8*time.Second)
		}
		items, err := step.fn(child, query)
		cancel()
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		out.Sources[step.key] = lookupStatus(err)
		if err != nil {
			return out, nil
		}
		if len(items) > 0 {
			out.Candidates = items
			return out, nil
		}
	}
	return out, nil
}

func searchStepAllowed(source, step string) bool {
	switch source {
	case "":
		return true
	case Installed, Global:
		return step == source
	default:
		return step == Installed || step == Global || step == source // local lookups apply the explicit origin filter
	}
}

func lookupStatus(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, remote.ErrAgenticHubSignInRequired):
		return "login_required"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, remote.ErrRateLimited):
		return "rate_limited"
	case errors.Is(err, ErrDisabled):
		return "disabled"
	case errors.Is(err, errUnsupportedResults):
		return "unsupported_source"
	default:
		return "unavailable"
	}
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
		if IsRepositoryKey(q) && !MatchesRepository(c, q) {
			continue
		}
		if strings.HasPrefix(q, "https://skills.sh/") && !strings.EqualFold(q, c.URL) {
			continue
		}
		n := strings.ToLower(c.Name)
		text := n + " " + strings.ToLower(c.Description+" "+c.Key+" "+c.URL+" "+c.Repository)
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
