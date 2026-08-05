package upgrade

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type platformTarget struct {
	GOOS   string
	GOARCH string
}

type codexPlatformTarget struct {
	platformTarget
	DownloadOS   string
	DownloadArch string
	Binary       string
}

func TestReleasePlatformsHaveCodexDownloadMappings(t *testing.T) {
	releasePlatforms := readReleasePlatforms(t)
	codexPlatforms := readCodexPlatforms(t)

	for _, platform := range releasePlatforms {
		key := platform.GOOS + "/" + platform.GOARCH
		codex, ok := codexPlatforms[key]
		if !ok {
			t.Fatalf("release platform %s has no Codex CLI download mapping", key)
		}
		if codex.DownloadOS == "" || codex.DownloadArch == "" || codex.Binary == "" {
			t.Fatalf("Codex CLI mapping for %s is incomplete: %#v", key, codex)
		}
		if platform.GOOS == "windows" && codex.Binary != "codex.exe" {
			t.Fatalf("Codex CLI binary for %s = %q, want codex.exe", key, codex.Binary)
		}
		if platform.GOOS != "windows" && codex.Binary == "codex.exe" {
			t.Fatalf("Codex CLI binary for %s = %q, want a non-Windows binary", key, codex.Binary)
		}
	}
}

func TestSelectAssetSelectsEveryReleasePlatform(t *testing.T) {
	const version = "v9.9.9"
	platforms := readReleasePlatforms(t)
	assets := make([]ReleaseAsset, 0, len(platforms))
	for _, platform := range platforms {
		assets = append(assets, ReleaseAsset{
			Name:               officialAssetName(version, platform.GOOS, platform.GOARCH),
			BrowserDownloadURL: "https://downloads.example.test/" + platform.GOOS + "-" + platform.GOARCH,
		})
	}
	release := LatestRelease{Name: version, Assets: assets}

	for _, platform := range platforms {
		platform := platform
		t.Run(platform.GOOS+"_"+platform.GOARCH, func(t *testing.T) {
			got, err := selectAsset(release, platform.GOOS, platform.GOARCH)
			if err != nil {
				t.Fatalf("selectAsset() error = %v", err)
			}
			want := officialAssetName(version, platform.GOOS, platform.GOARCH)
			if got.Name != want {
				t.Fatalf("selectAsset() name = %q, want %q", got.Name, want)
			}
		})
	}
}

func readReleasePlatforms(t *testing.T) []platformTarget {
	t.Helper()

	lines := readPlatformMapFile(t, "release-platforms.txt")
	platforms := make([]platformTarget, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		if len(line.fields) != 2 {
			t.Fatalf("release platform map line %d has %d fields, want 2", line.number, len(line.fields))
		}
		platform := platformTarget{GOOS: line.fields[0], GOARCH: line.fields[1]}
		key := platform.GOOS + "/" + platform.GOARCH
		if _, exists := seen[key]; exists {
			t.Fatalf("release platform map has duplicate target %s", key)
		}
		seen[key] = struct{}{}
		platforms = append(platforms, platform)
	}
	return platforms
}

func readCodexPlatforms(t *testing.T) map[string]codexPlatformTarget {
	t.Helper()

	lines := readPlatformMapFile(t, "codex-cli-platforms.txt")
	platforms := make(map[string]codexPlatformTarget, len(lines))
	for _, line := range lines {
		if len(line.fields) != 5 {
			t.Fatalf("Codex CLI platform map line %d has %d fields, want 5", line.number, len(line.fields))
		}
		platform := codexPlatformTarget{
			platformTarget: platformTarget{GOOS: line.fields[0], GOARCH: line.fields[1]},
			DownloadOS:     line.fields[2],
			DownloadArch:   line.fields[3],
			Binary:         line.fields[4],
		}
		key := platform.GOOS + "/" + platform.GOARCH
		if _, exists := platforms[key]; exists {
			t.Fatalf("Codex CLI platform map has duplicate target %s", key)
		}
		platforms[key] = platform
	}
	return platforms
}

type platformMapLine struct {
	number int
	fields []string
}

func readPlatformMapFile(t *testing.T, name string) []platformMapLine {
	t.Helper()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	path := filepath.Join(filepath.Dir(sourceFile), "..", "..", "scripts", name)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	var lines []platformMapLine
	for number, raw := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lines = append(lines, platformMapLine{number: number + 1, fields: strings.Fields(trimmed)})
	}
	if len(lines) == 0 {
		t.Fatalf("platform map %s is empty", fmt.Sprintf("scripts/%s", name))
	}
	return lines
}
