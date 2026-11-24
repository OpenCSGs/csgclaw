package catalog

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"csgclaw/internal/skill/installstate"
)

// A bare namespace/name is an identity, not an instruction to clone GitHub.
func IsRepositoryKey(query string) bool {
	owner, name, ok := strings.Cut(query, "/")
	return ok && validComponent(owner) && validComponent(name)
}
func validComponent(s string) bool {
	return s != "" && filepath.IsLocal(s) && !strings.HasPrefix(s, ".") && !strings.ContainsAny(s, "/\\?#% \t\r\n")
}

func MatchesRepository(c Candidate, key string) bool {
	if strings.EqualFold(c.Key, key) || strings.EqualFold(c.Repository, key) {
		return true
	}
	u, err := url.Parse(c.URL)
	if err != nil || u.Host == "" {
		return false
	}
	if ValidPublicURL(c.URL) {
		return strings.HasPrefix(strings.ToLower(u.Path), "/"+strings.ToLower(key)+"/")
	}
	return strings.HasSuffix(strings.TrimRight(strings.ToLower(u.Path), "/"), "/skills/"+strings.ToLower(key))
}

// FindInstalled includes disabled skills from the original installation root,
// never the enabled-only runtime view. Only exact identities can skip install.
func FindInstalled(ctx context.Context, root, query string, accept func(installstate.Record) bool) ([]Candidate, error) {
	if !validComponent(query) && !IsRepositoryKey(query) && !ValidPublicURL(query) {
		return nil, nil
	}
	if _, err := os.Stat(root); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	matches := func(name string, r installstate.Record) bool {
		if !validComponent(name) || !accept(r) {
			return false
		}
		if ValidPublicURL(query) {
			return r.DisplayURL() == query
		}
		if IsRepositoryKey(query) {
			return MatchesRepository(Candidate{Key: r.RemotePath, URL: r.DisplayURL()}, query)
		}
		return name == query
	}
	records, err := installstate.Read(root)
	if err != nil {
		return nil, err
	}
	var names []string
	if validComponent(query) {
		names = []string{query}
	} else {
		for name, r := range records {
			if matches(name, r) {
				names = append(names, name)
			}
		}
		sort.Strings(names)
	}
	var out []Candidate
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !matches(name, records[name]) {
			continue
		}
		if fi, err := os.Lstat(filepath.Join(root, name, "SKILL.md")); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		} else if !fi.Mode().IsRegular() {
			continue
		}
		rec, token, err := installstate.Snapshot(root, name)
		if err != nil {
			return nil, err
		}
		if matches(name, rec) {
			out = append(out, Candidate{Source: Installed, Name: name, Key: name, URL: rec.DisplayURL(), Version: rec.Version, Ref: rec.Ref, Selection: token})
		}
		if len(out) == Limit {
			break
		}
	}
	return out, nil
}
