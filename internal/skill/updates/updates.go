// Package updates checks published skill releases and applies explicitly reviewed replacements.
package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"csgclaw/internal/skill/discovery"
	"csgclaw/internal/skill/installstate"
	"csgclaw/internal/skill/local"
	"csgclaw/internal/skill/remote"
	"golang.org/x/mod/semver"
)

type Connection struct{ API, Site, Token string }

type Target struct {
	ID      string              `json:"id"`
	Name    string              `json:"name"`
	Version string              `json:"version"`
	State   string              `json:"state"`
	Changes []local.FileChange  `json:"changes,omitempty"`
	Root    string              `json:"-"`
	Record  installstate.Record `json:"-"`
}

type Info struct {
	Name          string    `json:"name"`
	Available     bool      `json:"available"`
	Skipped       bool      `json:"skipped,omitempty"`
	LocalVersion  string    `json:"localVersion"`
	LatestVersion string    `json:"latestVersion"`
	Commit        string    `json:"commit"`
	Changelog     string    `json:"changelog,omitempty"`
	PublishedAt   time.Time `json:"publishedAt"`
	CheckedAt     time.Time `json:"checkedAt"`
	SourceURL     string    `json:"sourceUrl"`
	Targets       []Target  `json:"targets"`
	State         string    `json:"state"`
	Error         string    `json:"error,omitempty"`
	RemotePath    string    `json:"remotePath"`
	// Retain every installation for confirmation checks and applying a pinned release.
	allTargets []Target
}

type Decision struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Commit   string `json:"commit"`
	State    string `json:"state"`
	Strategy string `json:"strategy"`
}

type Result struct {
	Name       string `json:"name"`
	TargetID   string `json:"targetId"`
	TargetName string `json:"targetName"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}

// SourcePath only resolves origins belonging to the currently selected Hub.
// A same-name skill or a URL from another environment is never an update source.
func SourcePath(record installstate.Record, site string) (string, error) {
	if record.SourceKind() != installstate.SourceMarket {
		return "", errors.New("skill_update_source_unavailable")
	}
	u, err := url.Parse(record.SourceURL)
	if err != nil {
		return "", err
	}
	base, err := url.Parse(strings.TrimRight(site, "/"))
	if err != nil {
		return "", err
	}
	prefix := strings.TrimRight(base.Path, "/") + "/skills/"
	if !strings.EqualFold(u.Host, base.Host) || u.Scheme != base.Scheme || !strings.HasPrefix(u.Path, prefix) {
		return "", errors.New("skill_update_source_unavailable")
	}
	path := strings.TrimPrefix(u.Path, prefix)
	if record.RemotePath != "" && record.RemotePath != path {
		return "", errors.New("skill_update_source_unavailable")
	}
	path, _, err = remote.NormalizeAgenticHubSkillRequest(path, "")
	return path, err
}

func Check(ctx context.Context, conn Connection, name string, targets []Target, preview bool) (Info, error) {
	info := Info{Name: name, CheckedAt: time.Now().UTC(), Targets: []Target{}}
	if len(targets) == 0 {
		return info, errors.New("skill_update_source_unavailable")
	}
	for i := range targets {
		record, state, err := installstate.Snapshot(targets[i].Root, name)
		if err != nil {
			return info, err
		}
		targets[i].Record, targets[i].State = record, state
		targets[i].Version = record.Version
	}
	info.Targets = targets
	info.LocalVersion = targets[0].Version
	info.SourceURL = targets[0].Record.SourceURL
	path, err := SourcePath(targets[0].Record, conn.Site)
	if err != nil {
		return info, err
	}
	info.RemotePath = path
	for _, target := range targets {
		other, err := SourcePath(target.Record, conn.Site)
		if err != nil || other != path {
			return info, errors.New("skill_update_source_unavailable")
		}
	}
	releases, err := remote.GetAgenticHubSkillReleases(ctx, conn.API, conn.Token, path)
	if err != nil {
		return info, err
	}
	release, ok := releases.Latest()
	if !ok {
		return info, nil
	}
	info.LatestVersion, info.Commit, info.Changelog, info.PublishedAt = release.Version, release.Commit, release.Changelog, release.CreatedAt
	info.Skipped = targets[0].Record.SkippedVersion == release.Version+"@"+release.Commit
	var archive []byte
	loadArchive := func() ([]byte, error) {
		if archive != nil {
			return archive, nil
		}
		var err error
		archive, err = remote.FetchAgenticHubSkillArchive(ctx, conn.API, conn.Token, path, release.Commit)
		return archive, err
	}
	var latestDigest string
	for i := range targets {
		target := &targets[i]
		if target.Version == "" {
			if version, ok := releases.VersionForCommit(target.Record.Commit); ok {
				target.Version = version
			} else if version, ok := releases.VersionForCommit(target.Record.Ref); ok {
				target.Version = version
			}
			if target.Version == "" {
				for _, published := range releases.Versions {
					if published.Version == target.Record.Ref && published.Commit != "" {
						target.Version = published.Version
						break
					}
				}
			}
			if target.Version == "" {
				if definition, err := discovery.ReadDefinition(filepath.Join(target.Root, name)); err == nil {
					target.Version = definition.Version
				}
			}
		}
		if target.Version != "" {
			if Newer(release.Version, target.Version) {
				info.Available = true
			}
			continue
		}
		if info.Skipped && !preview {
			continue
		}
		// A missing version is not evidence that the installation is current.
		// Compare the original install digest so local edits alone do not invent
		// upstream updates. Resolve exact matches without writing local metadata.
		if latestDigest == "" {
			data, err := loadArchive()
			if err != nil {
				return info, err
			}
			latestDigest, err = local.ArchiveDigest(name, remote.AgenticHubSkillArchiveName(path), data)
			if err != nil {
				return info, err
			}
		}
		baseline := target.Record.SHA256
		if baseline == "" {
			var err error
			baseline, err = installstate.BundleSHA256(filepath.Join(target.Root, name))
			if err != nil {
				return info, err
			}
		}
		if baseline == latestDigest {
			target.Version = release.Version
		} else {
			info.Available = true
		}
	}
	info.allTargets = targets
	info.Targets = make([]Target, 0, len(targets))
	for _, target := range targets {
		if !Newer(target.Version, release.Version) {
			info.Targets = append(info.Targets, target)
		}
	}
	info.LocalVersion = targets[0].Version
	info.Available = info.Available && !info.Skipped
	info.State = stateOf(targets)
	if preview && len(info.Targets) > 0 {
		archive, err := loadArchive()
		if err != nil {
			return info, err
		}
		for i := range info.Targets {
			info.Targets[i].Changes, err = local.PreviewArchive(info.Targets[i].Root, name, remote.AgenticHubSkillArchiveName(path), archive)
			if err != nil {
				return info, err
			}
			_, state, err := installstate.Snapshot(info.Targets[i].Root, name)
			if err != nil {
				return info, err
			}
			if state != info.Targets[i].State {
				return info, installstate.ErrInstallationChanged
			}
		}
	}
	return info, nil
}

func Newer(latest, current string) bool {
	a, b := "v"+strings.TrimPrefix(latest, "v"), "v"+strings.TrimPrefix(current, "v")
	return semver.IsValid(a) && semver.IsValid(b) && semver.Compare(a, b) > 0
}

func stateOf(targets []Target) string {
	type state struct{ ID, Root, Token string }
	items := make([]state, 0, len(targets))
	for _, target := range targets {
		items = append(items, state{target.ID, target.Root, target.State})
	}
	data, _ := json.Marshal(items)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Apply never follows the latest pointer after confirmation. Each target is
// compared again under its installation lock, and partial results stay explicit.
func Apply(ctx context.Context, conn Connection, decision Decision, info Info, withTarget func(Target, func() error) error) ([]Result, error) {
	if decision.State == "" || decision.State != info.State {
		return nil, installstate.ErrInstallationChanged
	}
	if decision.Strategy != "update" && decision.Strategy != "keep-local" {
		return nil, fmt.Errorf("invalid skill update strategy")
	}
	releases, err := remote.GetAgenticHubSkillReleases(ctx, conn.API, conn.Token, info.RemotePath)
	if err != nil {
		return nil, err
	}
	release, err := releases.Find(decision.Version, decision.Commit)
	if err != nil {
		return nil, err
	}
	if decision.Strategy == "keep-local" {
		target := info.allTargets[0]
		record := target.Record
		record.SkippedVersion = release.Version + "@" + release.Commit
		err = installstate.UpdateChecked(target.Root, info.Name, &record, target.State, nil)
		if err != nil {
			return nil, err
		}
		return []Result{{Name: info.Name, TargetID: target.ID, TargetName: target.Name, Status: "kept"}}, nil
	}
	archive, err := remote.FetchAgenticHubSkillArchive(ctx, conn.API, conn.Token, info.RemotePath, release.Commit)
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(info.Targets))
	for _, target := range info.allTargets {
		// Recheck against the confirmed release, even if the Hub's latest pointer moved.
		if Newer(target.Version, release.Version) {
			continue
		}
		result := Result{Name: info.Name, TargetID: target.ID, TargetName: target.Name, Status: "updated"}
		err := withTarget(target, func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			record := target.Record
			record.Version, record.Commit, record.Ref, record.RemotePath, record.SkippedVersion = release.Version, release.Commit, release.Commit, info.RemotePath, ""
			_, err := local.InstallArchiveWithOptions(target.Root, remote.AgenticHubSkillArchiveName(info.RemotePath), archive, local.InstallArchiveOptions{Replace: true, Origin: record, ExpectedState: target.State, ExpectedName: info.Name})
			return err
		})
		if err != nil {
			result.Status = "failed"
			result.Error = err.Error()
		}
		results = append(results, result)
	}
	return results, nil
}
