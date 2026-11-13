package remote

import (
	"context"
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
