package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"csgclaw/internal/skill/remote"
)

var githubSkillID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*/[a-zA-Z0-9][a-zA-Z0-9._-]*/[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func ValidPublicURL(raw string) bool {
	id, ok := strings.CutPrefix(raw, "https://skills.sh/")
	return ok && githubSkillID.MatchString(id)
}

// Use the same keyword search protocol as the pinned skills CLI, but preserve
// network/HTTP failures instead of reporting them as an empty catalog.
func SearchPublic(ctx context.Context, query string) ([]Candidate, error) {
	if disabled, _ := strconv.ParseBool(os.Getenv("CSGCLAW_DISABLE_PUBLIC_SKILLS")); disabled {
		return nil, ErrDisabled
	}
	if ValidPublicURL(query) {
		id := strings.TrimPrefix(query, "https://skills.sh/")
		parts := strings.Split(id, "/")
		return []Candidate{{Source: Public, Key: id, Name: parts[2], URL: query}}, nil
	}
	return searchPublic(ctx, http.DefaultClient, "https://skills.sh/api/search", query)
}

func searchPublic(ctx context.Context, client *http.Client, endpoint, query string) ([]Candidate, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+url.Values{"q": {query}, "limit": {strconv.Itoa(Limit)}}.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		return nil, remote.ErrRateLimited
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("skills.sh search returned HTTP %d", resp.StatusCode)
	}
	const limit = 1 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	var body struct {
		Skills []struct{ ID, Source, Name string }
	}
	if len(data) > limit || json.Unmarshal(data, &body) != nil || body.Skills == nil {
		return nil, errors.New("invalid skills.sh search response")
	}
	var out []Candidate
	for _, s := range body.Skills {
		if !githubSkillID.MatchString(s.ID) || !strings.HasPrefix(s.ID, s.Source+"/") || strings.Count(s.Source, "/") != 1 {
			continue
		}
		out = append(out, Candidate{Source: Public, Name: s.Name, Key: s.ID, URL: "https://skills.sh/" + s.ID})
		if len(out) == Limit {
			break
		}
	}
	if len(body.Skills) > 0 && len(out) == 0 {
		return nil, errUnsupportedResults
	}
	return out, nil
}
