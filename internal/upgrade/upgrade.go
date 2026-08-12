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
	DefaultReleaseManifestURL = "https://opencsg-public-resource.oss-cn-beijing.aliyuncs.com/csgclaw-desktop/channels/release/downloads.json"
	DefaultBetaManifestURL    = "https://opencsg-public-resource.oss-cn-beijing.aliyuncs.com/csgclaw-desktop/channels/beta/downloads.json"
	// DefaultLatestReleaseURL is retained as the stable-channel alias used by
	// callers that only need the default source.
	DefaultLatestReleaseURL = DefaultReleaseManifestURL
	defaultSiteBaseURL      = "https://opencsg-public-resource.oss-cn-beijing.aliyuncs.com/csgclaw-desktop/"
)

type Channel string

const (
	ChannelRelease Channel = "release"
	ChannelBeta    Channel = "beta"
)

func NormalizeChannel(raw string) (Channel, error) {
	switch Channel(strings.ToLower(strings.TrimSpace(raw))) {
	case "", ChannelRelease:
		return ChannelRelease, nil
	case ChannelBeta:
		return ChannelBeta, nil
	default:
		return "", fmt.Errorf("upgrade channel must be release or beta")
	}
}

func ManifestURL(channel Channel) string {
	if channel == ChannelBeta {
		return DefaultBetaManifestURL
	}
	return DefaultReleaseManifestURL
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	HTTPClient     HTTPClient
	LatestURL      string
	Channel        Channel
	GOOS           string
	GOARCH         string
	ExecutablePath func() (string, error)
}

type LatestRelease struct {
	Name   string         `json:"name"`
	Assets []ReleaseAsset `json:"assets"`
}

type downloadsManifest struct {
	SchemaVersion int                                 `json:"schema_version"`
	Channel       string                              `json:"channel"`
	Latest        string                              `json:"latest"`
	Versions      map[string]downloadsManifestVersion `json:"versions"`
}

type downloadsManifestVersion struct {
	Version  string                     `json:"version"`
	Packages []downloadsManifestPackage `json:"packages"`
}

type downloadsManifestPackage struct {
	Kind      string `json:"kind"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
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
	if isLocalBuildVersion(currentVersion) {
		release, err := c.FetchLatest(ctx)
		if err != nil {
			return CheckResult{}, err
		}
		latestVersion := strings.TrimSpace(release.Name)
		if !isSemver(latestVersion) {
			return CheckResult{}, fmt.Errorf("latest version %q is not a valid semver release", latestVersion)
		}
		return CheckResult{
			CurrentVersion: currentVersion,
			LatestVersion:  normalizeSemver(latestVersion),
		}, nil
	}
	if !isUpdatableReleaseVersion(currentVersion) {
		return CheckResult{CurrentVersion: currentVersion}, nil
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

	var payload json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return LatestRelease{}, fmt.Errorf("decode latest release metadata: %w", err)
	}

	var manifest downloadsManifest
	if err := json.Unmarshal(payload, &manifest); err == nil && strings.TrimSpace(manifest.Latest) != "" {
		return latestReleaseFromManifest(manifest)
	}

	var release LatestRelease
	if err := json.Unmarshal(payload, &release); err != nil {
		return LatestRelease{}, fmt.Errorf("decode latest release metadata: %w", err)
	}
	return release, nil
}

func latestReleaseFromManifest(manifest downloadsManifest) (LatestRelease, error) {
	if manifest.SchemaVersion != 1 {
		return LatestRelease{}, fmt.Errorf("unsupported downloads manifest schema %d", manifest.SchemaVersion)
	}
	latest := strings.TrimSpace(manifest.Latest)
	entry, ok := manifest.Versions[latest]
	if !ok {
		return LatestRelease{}, fmt.Errorf("downloads manifest latest version %q is missing", latest)
	}
	assets := make([]ReleaseAsset, 0, len(entry.Packages))
	for _, pkg := range entry.Packages {
		if pkg.Kind != "server" {
			continue
		}
		assets = append(assets, ReleaseAsset{
			Name:               pkg.Name,
			BrowserDownloadURL: pkg.URL,
			DownloadURL:        pkg.URL,
			Size:               pkg.SizeBytes,
			SHA256:             pkg.SHA256,
		})
	}
	version := strings.TrimSpace(entry.Version)
	if version == "" {
		version = latest
	}
	return LatestRelease{Name: normalizeSemver(version), Assets: assets}, nil
}

func (c Client) resolvedLatestURL() string {
	if strings.TrimSpace(c.LatestURL) != "" {
		return c.LatestURL
	}
	channel, err := NormalizeChannel(string(c.Channel))
	if err != nil {
		channel = ChannelRelease
	}
	return ManifestURL(channel)
}

func (c *Client) SetChannel(channel Channel) error {
	normalized, err := NormalizeChannel(string(channel))
	if err != nil {
		return err
	}
	c.Channel = normalized
	return nil
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
	wantName := officialAssetName(version, goos, goarch)
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

func officialAssetName(version, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return "csgclaw_" + version + "_" + goos + "_" + goarch + ext
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

func isUpdatableReleaseVersion(version string) bool {
	version = strings.TrimSpace(version)
	if _, ok := parseSemver(version); !ok {
		return false
	}
	return !isLocalBuildVersion(version)
}

func isLocalBuildVersion(version string) bool {
	version = strings.TrimSpace(version)
	return version != "" && version != "dev" && strings.HasSuffix(version, "+local")
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
