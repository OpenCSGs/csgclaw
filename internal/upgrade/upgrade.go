package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	DefaultLatestReleaseURL = "https://csgclaw.opencsg.com/releases/latest"
	defaultSiteBaseURL      = "https://csgclaw.opencsg.com"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	HTTPClient     HTTPClient
	LatestURL      string
	GOOS           string
	GOARCH         string
	ExecutablePath func() (string, error)
}

type LatestRelease struct {
	Name   string         `json:"name"`
	Assets []ReleaseAsset `json:"assets"`
}

type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	SHA256             string `json:"sha256"`
	DownloadURL        string `json:"-"`
}

type CheckResult struct {
	CurrentVersion  string        `json:"current_version"`
	LatestVersion   string        `json:"latest_version"`
	UpdateAvailable bool          `json:"update_available"`
	Asset           *ReleaseAsset `json:"asset,omitempty"`
}

func (c Client) Check(ctx context.Context, currentVersion string) (CheckResult, error) {
	currentVersion = strings.TrimSpace(currentVersion)
	if !isSemver(currentVersion) {
		return CheckResult{}, fmt.Errorf("current version %q is not a valid semver release", currentVersion)
	}

	release, err := c.FetchLatest(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	latestVersion := strings.TrimSpace(release.Name)
	if !isSemver(latestVersion) {
		return CheckResult{}, fmt.Errorf("latest version %q is not a valid semver release", latestVersion)
	}

	result := CheckResult{
		CurrentVersion: normalizeSemver(currentVersion),
		LatestVersion:  normalizeSemver(latestVersion),
	}
	if compareSemver(result.CurrentVersion, result.LatestVersion) >= 0 {
		return result, nil
	}
	result.UpdateAvailable = true

	asset, err := selectAsset(release, c.resolvedGOOS(), c.resolvedGOARCH())
	if err != nil {
		return CheckResult{}, err
	}
	result.Asset = &asset
	return result, nil
}

func (c Client) FetchLatest(ctx context.Context) (LatestRelease, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.resolvedLatestURL(), nil)
	if err != nil {
		return LatestRelease{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return LatestRelease{}, fmt.Errorf("fetch latest release metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return LatestRelease{}, fmt.Errorf("fetch latest release metadata: unexpected status %s", resp.Status)
	}

	var release LatestRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return LatestRelease{}, fmt.Errorf("decode latest release metadata: %w", err)
	}
	return release, nil
}

func (c Client) resolvedLatestURL() string {
	if strings.TrimSpace(c.LatestURL) != "" {
		return c.LatestURL
	}
	return DefaultLatestReleaseURL
}

func (c Client) resolvedGOOS() string {
	if strings.TrimSpace(c.GOOS) != "" {
		return strings.TrimSpace(c.GOOS)
	}
	return "unknown"
}

func (c Client) resolvedGOARCH() string {
	if strings.TrimSpace(c.GOARCH) != "" {
		return strings.TrimSpace(c.GOARCH)
	}
	return "unknown"
}

func selectAsset(release LatestRelease, goos, goarch string) (ReleaseAsset, error) {
	version := normalizeSemver(release.Name)
	wantName := "csgclaw_" + version + "_" + goos + "_" + goarch + ".tar.gz"
	for _, asset := range release.Assets {
		if strings.HasPrefix(asset.Name, "csgclaw-cli_") {
			continue
		}
		if asset.Name != wantName {
			continue
		}
		asset.DownloadURL = absolutizeURL(defaultSiteBaseURL, asset.BrowserDownloadURL)
		return asset, nil
	}
	return ReleaseAsset{}, fmt.Errorf("no release asset for %s/%s at version %s", goos, goarch, version)
}

func absolutizeURL(baseURL, raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if u.IsAbs() {
		return u.String()
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return base.ResolveReference(u).String()
}

func isSemver(version string) bool {
	_, ok := parseSemver(version)
	return ok
}

func normalizeSemver(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return ""
	}
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

type semverParts struct {
	major int
	minor int
	patch int
	pre   string
}

func parseSemver(version string) (semverParts, bool) {
	version = normalizeSemver(version)
	if !strings.HasPrefix(version, "v") {
		return semverParts{}, false
	}
	version = strings.TrimPrefix(version, "v")
	main := version
	pre := ""
	if before, after, ok := strings.Cut(version, "-"); ok {
		main = before
		pre = after
	}
	if before, _, ok := strings.Cut(main, "+"); ok {
		main = before
	}
	parts := strings.Split(main, ".")
	if len(parts) != 3 {
		return semverParts{}, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semverParts{}, false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semverParts{}, false
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return semverParts{}, false
	}
	if strings.Contains(pre, "+") {
		pre, _, _ = strings.Cut(pre, "+")
	}
	return semverParts{major: major, minor: minor, patch: patch, pre: pre}, true
}

func compareSemver(a, b string) int {
	av, ok := parseSemver(a)
	if !ok {
		return strings.Compare(a, b)
	}
	bv, ok := parseSemver(b)
	if !ok {
		return strings.Compare(a, b)
	}
	if av.major != bv.major {
		return compareInts(av.major, bv.major)
	}
	if av.minor != bv.minor {
		return compareInts(av.minor, bv.minor)
	}
	if av.patch != bv.patch {
		return compareInts(av.patch, bv.patch)
	}
	return comparePrerelease(av.pre, bv.pre)
}

func compareInts(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func comparePrerelease(a, b string) int {
	if a == "" && b == "" {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}

	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		if cmp := comparePrereleaseIdentifier(aParts[i], bParts[i]); cmp != 0 {
			return cmp
		}
	}
	return compareInts(len(aParts), len(bParts))
}

func comparePrereleaseIdentifier(a, b string) int {
	ai, aErr := strconv.Atoi(a)
	bi, bErr := strconv.Atoi(b)
	switch {
	case aErr == nil && bErr == nil:
		return compareInts(ai, bi)
	case aErr == nil:
		return -1
	case bErr == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}
