package remote

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SkillRelease is a published AgenticHub version bound to an immutable commit.
type SkillRelease struct {
	Version   string    `json:"version"`
	Commit    string    `json:"commit"`
	Changelog string    `json:"changelog"`
	CreatedAt time.Time `json:"created_at"`
}

type SkillReleases struct {
	DefaultBranch string         `json:"default_branch"`
	LatestVersion string         `json:"latest_version"`
	Versions      []SkillRelease `json:"versions"`
}

// ResolveSkillRelease uses the release commit when available, and preserves
// branch-based installs on sites without release metadata.
func ResolveSkillRelease(ctx context.Context, baseURL, token, path, requested string) (version, commit, ref string, err error) {
	_, ref, err = NormalizeAgenticHubSkillRequest(path, requested)
	if err != nil {
		return
	}
	releases, err := GetAgenticHubSkillReleases(ctx, baseURL, token, path)
	if err != nil {
		if errors.Is(err, errCatalogNotFound) {
			return "", "", ref, nil
		}
		return "", "", "", err
	}
	if r, ok := releases.Latest(); ok && (requested == "" || requested == "main" || requested == releases.DefaultBranch || requested == r.Commit) {
		version, commit, ref = r.Version, r.Commit, r.Commit
	}
	for _, r := range releases.Versions {
		if r.Version == requested || r.Commit == requested {
			version, commit, ref = r.Version, r.Commit, r.Commit
			break
		}
	}
	return
}

func GetAgenticHubSkillReleases(ctx context.Context, baseURL, token, remotePath string) (SkillReleases, error) {
	remotePath, _, err := NormalizeAgenticHubSkillRequest(remotePath, "")
	if err != nil {
		return SkillReleases{}, err
	}
	parts := append([]string{"api", "v1", "skills"}, strings.Split(remotePath, "/")...)
	endpoint, err := buildAgenticHubURL(baseURL, parts, false, nil)
	if err != nil {
		return SkillReleases{}, err
	}
	var response struct {
		Data SkillReleases `json:"data"`
	}
	err = getAgenticHubJSONWithAccessToken(ctx, &http.Client{Timeout: agenticHubRequestTimeout}, endpoint, token, &response)
	return response.Data, err
}

func (r SkillReleases) Latest() (SkillRelease, bool) {
	for _, version := range r.Versions {
		if version.Version == r.LatestVersion && version.Commit != "" {
			return version, true
		}
	}
	return SkillRelease{}, false
}

// VersionForCommit identifies a published installation even if SKILL.md has no version.
func (r SkillReleases) VersionForCommit(commit string) (string, bool) {
	if commit == "" {
		return "", false
	}
	if latest, ok := r.Latest(); ok && latest.Commit == commit {
		return latest.Version, true
	}
	var matched *SkillRelease
	for _, item := range r.Versions {
		if item.Commit == commit && item.Version != "" && (matched == nil || item.CreatedAt.After(matched.CreatedAt)) {
			version := item
			matched = &version
		}
	}
	if matched == nil {
		return "", false
	}
	return matched.Version, true
}

func (r SkillReleases) Find(version, commit string) (SkillRelease, error) {
	for _, item := range r.Versions {
		if item.Version == version && item.Commit == commit && commit != "" {
			return item, nil
		}
	}
	return SkillRelease{}, fmt.Errorf("skill_release_changed")
}
