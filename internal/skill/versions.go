package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

type SkillVersionsList struct {
	Registry    RegistryID     `json:"registry"`
	Slug        string         `json:"slug"`
	DisplayName string         `json:"displayName,omitempty"`
	Versions    []SkillVersion `json:"versions"`
}

func (c *Client) ListVersions(ctx context.Context, slug string, limit int) ([]SkillVersion, error) {
	slug = normalizeSlug(slug)
	if slug == "" {
		return nil, fmt.Errorf("skill slug is required")
	}
	if limit <= 0 {
		limit = 100
	}

	versions, ok, err := c.listVersionsFromEndpoint(ctx, slug, limit)
	if err != nil {
		return nil, err
	}
	if ok {
		return sortVersionsNewestFirst(versions), nil
	}

	detail, err := c.Get(ctx, slug)
	if err != nil {
		return nil, err
	}
	out := detail.Versions
	if len(out) > limit {
		out = out[:limit]
	}
	return sortVersionsNewestFirst(out), nil
}

func (c *Client) listVersionsFromEndpoint(ctx context.Context, slug string, limit int) ([]SkillVersion, bool, error) {
	var collected []SkillVersion
	cursor := ""
	for len(collected) < limit {
		pageLimit := limit - len(collected)
		if pageLimit > 50 {
			pageLimit = 50
		}
		values := url.Values{}
		values.Set("limit", fmt.Sprintf("%d", pageLimit))
		if cursor != "" {
			values.Set("cursor", cursor)
		}
		path := "skills/" + url.PathEscape(slug) + "/versions"
		body, status, err := c.request(ctx, http.MethodGet, c.apiURL(path, values), c.maxJSON+1)
		if err != nil {
			return nil, false, err
		}
		if status == http.StatusNotFound {
			return nil, false, nil
		}
		if int64(len(body)) > c.maxJSON {
			return nil, false, fmt.Errorf("clawhub response exceeds %d bytes", c.maxJSON)
		}
		if status < 200 || status >= 300 {
			return nil, false, fmt.Errorf("clawhub request failed with status %d: %s", status, truncateBody(body))
		}

		var payload skillVersionsPage
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, false, fmt.Errorf("decode clawhub versions response: %w", err)
		}
		if len(payload.Items) == 0 && len(payload.Versions) > 0 {
			payload.Items = payload.Versions
		}
		collected = append(collected, payload.Items...)
		cursor = strings.TrimSpace(payload.NextCursor)
		if cursor == "" {
			break
		}
	}
	if len(collected) > limit {
		collected = collected[:limit]
	}
	return collected, true, nil
}

type skillVersionsPage struct {
	Items      []SkillVersion `json:"items"`
	Versions   []SkillVersion `json:"versions"`
	NextCursor string         `json:"nextCursor"`
}

func sortVersionsNewestFirst(versions []SkillVersion) []SkillVersion {
	if len(versions) < 2 {
		return versions
	}
	out := append([]SkillVersion(nil), versions...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt > out[j].CreatedAt
		}
		return out[i].Version > out[j].Version
	})
	return out
}
