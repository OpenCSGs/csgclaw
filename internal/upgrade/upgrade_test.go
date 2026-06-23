package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestClientCheckUpdateAvailable(t *testing.T) {
	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want GET", req.Method)
			}
			if req.URL.String() != "https://example.test/releases/latest" {
				t.Fatalf("url = %q, want latest endpoint", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `{
				"name":"v0.2.7",
				"assets":[
					{"name":"csgclaw-cli_v0.2.7_darwin_arm64.tar.gz"},
					{"name":"csgclaw_v0.2.7_darwin_arm64.tar.gz","browser_download_url":"http://csgclaw.opencsg.com/releases/v0.2.7/csgclaw_v0.2.7_darwin_arm64.tar.gz","size":123,"sha256":"abc"}
				]
			}`), nil
		}),
		LatestURL: "https://example.test/releases/latest",
		GOOS:      "darwin",
		GOARCH:    "arm64",
	}

	result, err := client.Check(context.Background(), "v0.2.5")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !result.UpdateAvailable {
		t.Fatal("Check().UpdateAvailable = false, want true")
	}
	if got, want := result.CurrentVersion, "v0.2.5"; got != want {
		t.Fatalf("CurrentVersion = %q, want %q", got, want)
	}
	if got, want := result.LatestVersion, "v0.2.7"; got != want {
		t.Fatalf("LatestVersion = %q, want %q", got, want)
	}
	if result.Asset == nil {
		t.Fatal("Asset = nil, want matched asset")
	}
	if got, want := result.Asset.Name, "csgclaw_v0.2.7_darwin_arm64.tar.gz"; got != want {
		t.Fatalf("Asset.Name = %q, want %q", got, want)
	}
	if got, want := result.Asset.DownloadURL, "http://csgclaw.opencsg.com/releases/v0.2.7/csgclaw_v0.2.7_darwin_arm64.tar.gz"; got != want {
		t.Fatalf("Asset.DownloadURL = %q, want %q", got, want)
	}
}

func TestClientCheckRequiresNameField(t *testing.T) {
	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"version":"v0.2.7","assets":[]}`), nil
		}),
		LatestURL: "https://example.test/releases/latest",
	}

	_, err := client.Check(context.Background(), "v0.2.7")
	if err == nil || !strings.Contains(err.Error(), `latest version "" is not a valid semver release`) {
		t.Fatalf("Check() error = %v, want missing name validation error", err)
	}
}
func TestClientCheckNoUpdate(t *testing.T) {
	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"name":"v0.2.7","assets":[]}`), nil
		}),
		LatestURL: "https://example.test/releases/latest",
		GOOS:      "darwin",
		GOARCH:    "arm64",
	}

	result, err := client.Check(context.Background(), "v0.2.7")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.UpdateAvailable {
		t.Fatal("Check().UpdateAvailable = true, want false")
	}
	if result.Asset != nil {
		t.Fatalf("Asset = %#v, want nil", result.Asset)
	}
}

func TestClientCheckSkipsInvalidCurrentVersion(t *testing.T) {
	client := Client{LatestURL: "https://example.test/releases/latest"}
	result, err := client.Check(context.Background(), "dev")
	if err != nil {
		t.Fatalf("Check() error = %v, want nil", err)
	}
	if got, want := result.CurrentVersion, "dev"; got != want {
		t.Fatalf("CurrentVersion = %q, want %q", got, want)
	}
	if result.UpdateAvailable {
		t.Fatal("UpdateAvailable = true, want false")
	}
	if got := result.LatestVersion; got != "" {
		t.Fatalf("LatestVersion = %q, want empty", got)
	}
}

func TestClientCheckLocalCurrentVersionReportsLatestWithoutUpgrade(t *testing.T) {
	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{
				"name":"v0.3.5",
				"assets":[
					{"name":"csgclaw_v0.3.5_darwin_arm64.tar.gz","browser_download_url":"http://csgclaw.opencsg.com/releases/v0.3.5/csgclaw_v0.3.5_darwin_arm64.tar.gz","size":123,"sha256":"abc"}
				]
			}`), nil
		}),
		LatestURL: "https://example.test/releases/latest",
		GOOS:      "darwin",
		GOARCH:    "arm64",
	}
	result, err := client.Check(context.Background(), "v0.3.5-test6+local")
	if err != nil {
		t.Fatalf("Check() error = %v, want nil", err)
	}
	if got, want := result.CurrentVersion, "v0.3.5-test6+local"; got != want {
		t.Fatalf("CurrentVersion = %q, want %q", got, want)
	}
	if result.UpdateAvailable {
		t.Fatal("UpdateAvailable = true, want false")
	}
	if got, want := result.LatestVersion, "v0.3.5"; got != want {
		t.Fatalf("LatestVersion = %q, want %q", got, want)
	}
	if result.Asset != nil {
		t.Fatalf("Asset = %#v, want nil", result.Asset)
	}
}

func TestClientCheckAllowsPrereleaseTagCurrentVersion(t *testing.T) {
	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{
				"name":"v0.3.5",
				"assets":[
					{"name":"csgclaw_v0.3.5_darwin_arm64.tar.gz","browser_download_url":"http://csgclaw.opencsg.com/releases/v0.3.5/csgclaw_v0.3.5_darwin_arm64.tar.gz","size":123,"sha256":"abc"}
				]
			}`), nil
		}),
		LatestURL: "https://example.test/releases/latest",
		GOOS:      "darwin",
		GOARCH:    "arm64",
	}

	result, err := client.Check(context.Background(), "v0.3.5-test6")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if got, want := result.CurrentVersion, "v0.3.5-test6"; got != want {
		t.Fatalf("CurrentVersion = %q, want %q", got, want)
	}
	if got, want := result.LatestVersion, "v0.3.5"; got != want {
		t.Fatalf("LatestVersion = %q, want %q", got, want)
	}
	if !result.UpdateAvailable {
		t.Fatal("UpdateAvailable = false, want true")
	}
	if result.Asset == nil {
		t.Fatal("Asset = nil, want matched asset")
	}
}

func TestClientCheckErrorsWhenAssetMissing(t *testing.T) {
	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"name":"v0.2.7","assets":[{"name":"csgclaw_v0.2.7_linux_amd64.tar.gz"}]}`), nil
		}),
		LatestURL: "https://example.test/releases/latest",
		GOOS:      "darwin",
		GOARCH:    "arm64",
	}

	_, err := client.Check(context.Background(), "v0.2.5")
	if err == nil || !strings.Contains(err.Error(), "no release asset for darwin/arm64") {
		t.Fatalf("Check() error = %v, want asset error", err)
	}
}

func TestClientCheckSelectsWindowsZipAsset(t *testing.T) {
	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{
				"name":"v0.2.7",
				"assets":[
					{"name":"csgclaw_v0.2.7_windows_amd64.zip","browser_download_url":"http://csgclaw.opencsg.com/releases/v0.2.7/csgclaw_v0.2.7_windows_amd64.zip","size":123,"sha256":"abc"},
					{"name":"csgclaw_v0.2.7_windows_arm64.zip","browser_download_url":"http://csgclaw.opencsg.com/releases/v0.2.7/csgclaw_v0.2.7_windows_arm64.zip","size":456,"sha256":"def"}
				]
			}`), nil
		}),
		LatestURL: "https://example.test/releases/latest",
		GOOS:      "windows",
		GOARCH:    "amd64",
	}

	result, err := client.Check(context.Background(), "v0.2.5")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.Asset == nil {
		t.Fatal("Asset = nil, want matched asset")
	}
	if got, want := result.Asset.Name, "csgclaw_v0.2.7_windows_amd64.zip"; got != want {
		t.Fatalf("Asset.Name = %q, want %q", got, want)
	}
}

func TestClientPrepareReleaseDownloadsVerifiesAndExtracts(t *testing.T) {
	archive := releaseTarball(t, map[string]string{
		"csgclaw/bin/csgclaw": "#!/bin/sh\n",
		"csgclaw/bin/boxlite": "#!/bin/sh\n",
		"csgclaw/README.md":   "bundle\n",
	})
	sum := sha256.Sum256(archive)

	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want GET", req.Method)
			}
			if req.URL.String() != "https://downloads.example.test/csgclaw.tar.gz" {
				t.Fatalf("url = %q, want asset download URL", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(archive)),
			}, nil
		}),
	}

	prepared, err := client.PrepareRelease(context.Background(), ReleaseAsset{
		Name:        "csgclaw_v0.2.7_darwin_arm64.tar.gz",
		DownloadURL: "https://downloads.example.test/csgclaw.tar.gz",
		Size:        int64(len(archive)),
		SHA256:      hex.EncodeToString(sum[:]),
	}, t.TempDir())
	if err != nil {
		t.Fatalf("PrepareRelease() error = %v", err)
	}

	if prepared.ArchivePath == "" || prepared.BundleDir == "" || prepared.WorkDir == "" {
		t.Fatalf("PrepareRelease() = %#v, want populated paths", prepared)
	}
	if _, err := os.Stat(prepared.ArchivePath); err != nil {
		t.Fatalf("Stat(%q) error = %v", prepared.ArchivePath, err)
	}
	for _, path := range []string{
		filepath.Join(prepared.BundleDir, "bin", "csgclaw"),
		filepath.Join(prepared.BundleDir, "bin", "boxlite"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("Stat(%q) error = %v", path, err)
		}
	}
}

func TestClientPrepareReleaseAllowsBundleWithoutBoxLite(t *testing.T) {
	archive := releaseTarball(t, map[string]string{
		"csgclaw/bin/csgclaw": "#!/bin/sh\n",
		"csgclaw/README.md":   "bundle\n",
	})
	sum := sha256.Sum256(archive)

	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(archive)),
			}, nil
		}),
	}

	prepared, err := client.PrepareRelease(context.Background(), ReleaseAsset{
		Name:        "csgclaw_v0.2.7_linux_amd64.tar.gz",
		DownloadURL: "https://downloads.example.test/csgclaw.tar.gz",
		Size:        int64(len(archive)),
		SHA256:      hex.EncodeToString(sum[:]),
	}, t.TempDir())
	if err != nil {
		t.Fatalf("PrepareRelease() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(prepared.BundleDir, "bin", "csgclaw")); err != nil {
		t.Fatalf("Stat(csgclaw) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(prepared.BundleDir, "bin", "boxlite")); !os.IsNotExist(err) {
		t.Fatalf("Stat(boxlite) error = %v, want not exist", err)
	}
}

func TestClientPrepareReleaseExtractsWindowsZipBundle(t *testing.T) {
	archive := releaseZipArchive(t, map[string]string{
		"csgclaw/bin/csgclaw.exe": "@echo off\r\n",
		"csgclaw/README.md":       "bundle\n",
	})
	sum := sha256.Sum256(archive)

	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(archive)),
			}, nil
		}),
	}

	prepared, err := client.PrepareRelease(context.Background(), ReleaseAsset{
		Name:        "csgclaw_v0.2.7_windows_amd64.zip",
		DownloadURL: "https://downloads.example.test/csgclaw.zip",
		Size:        int64(len(archive)),
		SHA256:      hex.EncodeToString(sum[:]),
	}, t.TempDir())
	if err != nil {
		t.Fatalf("PrepareRelease() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(prepared.BundleDir, "bin", "csgclaw.exe")); err != nil {
		t.Fatalf("Stat(csgclaw.exe) error = %v", err)
	}
}

func TestClientPrepareReleaseRejectsSizeMismatch(t *testing.T) {
	archive := releaseTarball(t, map[string]string{
		"csgclaw/bin/csgclaw": "#!/bin/sh\n",
		"csgclaw/bin/boxlite": "#!/bin/sh\n",
	})
	sum := sha256.Sum256(archive)

	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(archive)),
			}, nil
		}),
	}

	_, err := client.PrepareRelease(context.Background(), ReleaseAsset{
		Name:        "csgclaw_v0.2.7_darwin_arm64.tar.gz",
		DownloadURL: "https://downloads.example.test/csgclaw.tar.gz",
		Size:        int64(len(archive) - 1),
		SHA256:      hex.EncodeToString(sum[:]),
	}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "downloaded size mismatch") {
		t.Fatalf("PrepareRelease() error = %v, want size mismatch", err)
	}
}

func TestClientPrepareReleaseAllowsMissingSHA256MetadataTemporarily(t *testing.T) {
	archive := releaseTarball(t, map[string]string{
		"csgclaw/bin/csgclaw": "#!/bin/sh\n",
		"csgclaw/bin/boxlite": "#!/bin/sh\n",
	})

	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(archive)),
			}, nil
		}),
	}

	prepared, err := client.PrepareRelease(context.Background(), ReleaseAsset{
		Name:        "csgclaw_v0.2.7_darwin_arm64.tar.gz",
		DownloadURL: "https://downloads.example.test/csgclaw.tar.gz",
		Size:        int64(len(archive)),
	}, t.TempDir())
	if err != nil {
		t.Fatalf("PrepareRelease() error = %v, want temporary success without sha256 metadata", err)
	}
	if prepared.BundleDir == "" {
		t.Fatalf("PrepareRelease() = %#v, want extracted bundle", prepared)
	}
}

func TestClientPrepareReleaseRejectsInvalidBundle(t *testing.T) {
	archive := releaseTarball(t, map[string]string{
		"csgclaw/bin/boxlite": "#!/bin/sh\n",
	})
	sum := sha256.Sum256(archive)

	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(archive)),
			}, nil
		}),
	}

	_, err := client.PrepareRelease(context.Background(), ReleaseAsset{
		Name:        "csgclaw_v0.2.7_darwin_arm64.tar.gz",
		DownloadURL: "https://downloads.example.test/csgclaw.tar.gz",
		Size:        int64(len(archive)),
		SHA256:      hex.EncodeToString(sum[:]),
	}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "release bundle is missing bin/csgclaw") {
		t.Fatalf("PrepareRelease() error = %v, want bundle validation error", err)
	}
}

func TestClientPrepareReleaseRejectsMissingBundleMarker(t *testing.T) {
	archive := releaseTarballWithoutMarker(t, map[string]string{
		"csgclaw/bin/csgclaw": "#!/bin/sh\n",
	})

	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(archive)),
			}, nil
		}),
	}

	_, err := client.PrepareRelease(context.Background(), ReleaseAsset{
		Name:        "csgclaw_v0.2.7_darwin_arm64.tar.gz",
		DownloadURL: "https://downloads.example.test/csgclaw.tar.gz",
		Size:        int64(len(archive)),
	}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "release bundle is missing .csgclaw-bundle.json") {
		t.Fatalf("PrepareRelease() error = %v, want missing marker error", err)
	}
}

func TestClientPrepareReleaseRejectsMalformedTarArchive(t *testing.T) {
	archive := releaseTarball(t, map[string]string{
		"../escape":           "bad\n",
		"csgclaw/bin/csgclaw": "#!/bin/sh\n",
		"csgclaw/bin/boxlite": "#!/bin/sh\n",
		"csgclaw/README.md":   "bundle\n",
	})

	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(archive)),
			}, nil
		}),
	}

	_, err := client.PrepareRelease(context.Background(), ReleaseAsset{
		Name:        "csgclaw_v0.2.7_darwin_arm64.tar.gz",
		DownloadURL: "https://downloads.example.test/csgclaw.tar.gz",
		Size:        int64(len(archive)),
	}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), `release archive contains invalid entry "../escape"`) {
		t.Fatalf("PrepareRelease() error = %v, want invalid tar entry", err)
	}
}

func TestClientPrepareReleaseRejectsMalformedZipArchive(t *testing.T) {
	archive := releaseZipArchive(t, map[string]string{
		"../escape":               "bad\n",
		"csgclaw/bin/csgclaw.exe": "@echo off\r\n",
		"csgclaw/README.md":       "bundle\n",
	})

	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(archive)),
			}, nil
		}),
	}

	_, err := client.PrepareRelease(context.Background(), ReleaseAsset{
		Name:        "csgclaw_v0.2.7_windows_amd64.zip",
		DownloadURL: "https://downloads.example.test/csgclaw.zip",
		Size:        int64(len(archive)),
	}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), `release archive contains invalid entry "../escape"`) {
		t.Fatalf("PrepareRelease() error = %v, want invalid zip entry", err)
	}
}

func TestClientInstallPreparedRejectsNonBundleInstall(t *testing.T) {
	client := Client{
		ExecutablePath: func() (string, error) {
			return filepath.Join(t.TempDir(), "csgclaw"), nil
		},
	}

	_, err := client.InstallPrepared(PreparedBundle{BundleDir: writeBundleDir(t, t.TempDir(), "new")})
	if err == nil || !strings.Contains(err.Error(), "not installed from an official csgclaw bundle") {
		t.Fatalf("InstallPrepared() error = %v, want non-bundle install error", err)
	}
}

func TestClientInstallPreparedRejectsSourceLikeBundleWithoutMarker(t *testing.T) {
	installRoot := writeBundleFilesWithoutMarker(t, t.TempDir(), map[string]string{
		filepath.Join("csgclaw", "bin", "csgclaw"): "#!/bin/sh\n",
		filepath.Join("csgclaw", "go.mod"):         "module csgclaw\n",
		filepath.Join("csgclaw", "keep-me"):        "user file",
	})
	client := Client{
		ExecutablePath: func() (string, error) {
			return filepath.Join(installRoot, "bin", "csgclaw"), nil
		},
	}

	_, err := client.InstallPrepared(PreparedBundle{BundleDir: writeBundleDir(t, t.TempDir(), "new")})
	if err == nil || !strings.Contains(err.Error(), "not installed from an official csgclaw bundle") {
		t.Fatalf("InstallPrepared() error = %v, want non-bundle install error", err)
	}
	assertFileContent(t, filepath.Join(installRoot, "keep-me"), "user file")
}

func TestInstallBundleRejectsUnsafeInstallRoot(t *testing.T) {
	sharedRoot := t.TempDir()
	keepPath := filepath.Join(sharedRoot, "keep-me")
	if err := os.WriteFile(keepPath, []byte("user file"), 0o644); err != nil {
		t.Fatalf("WriteFile(keep-me) error = %v", err)
	}

	err := installBundle(writeBundleDir(t, t.TempDir(), "new"), sharedRoot)
	if !errors.Is(err, ErrNotOfficialBundle) {
		t.Fatalf("installBundle() error = %v, want ErrNotOfficialBundle", err)
	}
	assertFileContent(t, keepPath, "user file")
}

func TestClientInstallPreparedAllowsLegacyOfficialBundleWithoutMarker(t *testing.T) {
	installRoot := writeBundleFilesWithoutMarker(t, legacyOfficialInstallParent(t), map[string]string{
		filepath.Join("csgclaw", "bin", "csgclaw"): "#!/bin/sh\n# old\n",
		filepath.Join("csgclaw", "README.md"):      "old",
	})
	preparedRoot := writeBundleDir(t, t.TempDir(), "new")
	client := Client{
		ExecutablePath: func() (string, error) {
			return filepath.Join(installRoot, "bin", "csgclaw"), nil
		},
	}

	installed, err := client.InstallPrepared(PreparedBundle{BundleDir: preparedRoot})
	if err != nil {
		t.Fatalf("InstallPrepared() error = %v", err)
	}
	if got, want := installed.InstallRoot, installRoot; got != want {
		t.Fatalf("InstallRoot = %q, want %q", got, want)
	}
	assertFileContent(t, filepath.Join(installRoot, "README.md"), "new")
	assertFileContent(t, filepath.Join(installRoot, bundleMarkerFileName), `{"app":"csgclaw","layout":"official-bundle","version":"test"}`)
}

func TestClientInstallPreparedRejectsLegacyOfficialPathWithSourceMarker(t *testing.T) {
	installRoot := writeBundleFilesWithoutMarker(t, legacyOfficialInstallParent(t), map[string]string{
		filepath.Join("csgclaw", "bin", "csgclaw"): "#!/bin/sh\n",
		filepath.Join("csgclaw", "go.mod"):         "module csgclaw\n",
		filepath.Join("csgclaw", "keep-me"):        "user file",
	})
	client := Client{
		ExecutablePath: func() (string, error) {
			return filepath.Join(installRoot, "bin", "csgclaw"), nil
		},
	}

	_, err := client.InstallPrepared(PreparedBundle{BundleDir: writeBundleDir(t, t.TempDir(), "new")})
	if err == nil || !strings.Contains(err.Error(), "not installed from an official csgclaw bundle") {
		t.Fatalf("InstallPrepared() error = %v, want non-bundle install error", err)
	}
	assertFileContent(t, filepath.Join(installRoot, "keep-me"), "user file")
}

func TestClientInstallPreparedReplacesBundleFromSymlinkedExecutable(t *testing.T) {
	installParent := t.TempDir()
	installRoot := writeBundleDir(t, installParent, "old")
	linkDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(linkDir) error = %v", err)
	}
	linkPath := filepath.Join(linkDir, "csgclaw")
	if err := os.Symlink(filepath.Join(installRoot, "bin", "csgclaw"), linkPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	preparedRoot := writeBundleDir(t, t.TempDir(), "new")
	client := Client{
		ExecutablePath: func() (string, error) {
			return linkPath, nil
		},
	}

	installed, err := client.InstallPrepared(PreparedBundle{BundleDir: preparedRoot})
	if err != nil {
		t.Fatalf("InstallPrepared() error = %v", err)
	}
	gotRoot, err := filepath.EvalSymlinks(installed.InstallRoot)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", installed.InstallRoot, err)
	}
	wantRoot, err := filepath.EvalSymlinks(installRoot)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", installRoot, err)
	}
	if got, want := gotRoot, wantRoot; got != want {
		t.Fatalf("InstallRoot = %q, want %q", got, want)
	}
	assertFileContent(t, filepath.Join(installRoot, "README.md"), "new")
	assertFileContent(t, filepath.Join(installRoot, "bin", "boxlite"), "#!/bin/sh\n# new boxlite\n")
}

func TestClientInstallPreparedRefreshesSandboxCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	installParent := t.TempDir()
	installRoot := writeBundleDir(t, installParent, "old")
	preparedRoot := writeBundleFiles(t, t.TempDir(), map[string]string{
		filepath.Join("csgclaw", "bin", "csgclaw"):                    "#!/bin/sh\n# new\n",
		filepath.Join("csgclaw", "bin", "boxlite"):                    "#!/bin/sh\n# new boxlite\n",
		filepath.Join("csgclaw", "bin", "csgclaw_dir", "csgclaw-cli"): "#!/bin/sh\n# new sandbox cli\n",
		filepath.Join("csgclaw", "README.md"):                         "new",
	})
	client := Client{
		ExecutablePath: func() (string, error) {
			return filepath.Join(installRoot, "bin", "csgclaw"), nil
		},
	}

	if _, err := client.InstallPrepared(PreparedBundle{BundleDir: preparedRoot}); err != nil {
		t.Fatalf("InstallPrepared() error = %v", err)
	}
	assertFileContent(t, filepath.Join(home, ".csgclaw", "sandbox-tools", "csgclaw-cli"), "#!/bin/sh\n# new sandbox cli\n")
}

func TestClientInstallPreparedReplacesWindowsBundle(t *testing.T) {
	installParent := t.TempDir()
	installRoot := writeBundleFiles(t, installParent, map[string]string{
		filepath.Join("csgclaw", "bin", "csgclaw.exe"): "@echo off\r\nREM old\r\n",
		filepath.Join("csgclaw", "README.md"):          "old",
	})
	preparedRoot := writeBundleFiles(t, t.TempDir(), map[string]string{
		filepath.Join("csgclaw", "bin", "csgclaw.exe"): "@echo off\r\nREM new\r\n",
		filepath.Join("csgclaw", "README.md"):          "new",
	})

	client := Client{
		ExecutablePath: func() (string, error) {
			return filepath.Join(installRoot, "bin", "csgclaw.exe"), nil
		},
	}

	installed, err := client.InstallPrepared(PreparedBundle{BundleDir: preparedRoot})
	if err != nil {
		t.Fatalf("InstallPrepared() error = %v", err)
	}
	if got, want := installed.InstallRoot, installRoot; got != want {
		t.Fatalf("InstallRoot = %q, want %q", got, want)
	}
	assertFileContent(t, filepath.Join(installRoot, "README.md"), "new")
	assertFileContent(t, filepath.Join(installRoot, "bin", "csgclaw.exe"), "@echo off\r\nREM new\r\n")
}

func TestClientInstallPreparedUsesOriginalExecutableEnv(t *testing.T) {
	installParent := t.TempDir()
	installRoot := writeBundleFiles(t, installParent, map[string]string{
		filepath.Join("csgclaw", "bin", "csgclaw.exe"): "@echo off\r\nREM old\r\n",
		filepath.Join("csgclaw", "README.md"):          "old",
	})
	preparedRoot := writeBundleFiles(t, t.TempDir(), map[string]string{
		filepath.Join("csgclaw", "bin", "csgclaw.exe"): "@echo off\r\nREM new\r\n",
		filepath.Join("csgclaw", "README.md"):          "new",
	})

	t.Setenv(originalExecutableEnvVar, filepath.Join(installRoot, "bin", "csgclaw.exe"))

	installed, err := Client{}.InstallPrepared(PreparedBundle{BundleDir: preparedRoot})
	if err != nil {
		t.Fatalf("InstallPrepared() error = %v", err)
	}
	if got, want := installed.InstallRoot, installRoot; got != want {
		t.Fatalf("InstallRoot = %q, want %q", got, want)
	}
	assertFileContent(t, filepath.Join(installRoot, "README.md"), "new")
}

func TestClientInstallPreparedResolvesWindowsLauncherLayout(t *testing.T) {
	appHome := filepath.Join(t.TempDir(), "csgclaw")
	installRoot := writeBundleFilesWithoutMarker(t, filepath.Join(appHome, "lib", "csgclaw", "v0.3.10"), map[string]string{
		filepath.Join("csgclaw", "bin", "csgclaw.exe"): "@echo off\r\nREM old\r\n",
		filepath.Join("csgclaw", "README.md"):          "old",
	})
	launcherDir := filepath.Join(appHome, "bin")
	if err := os.MkdirAll(launcherDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", launcherDir, err)
	}
	launcherPath := filepath.Join(launcherDir, "csgclaw.exe")
	if err := os.WriteFile(launcherPath, []byte("launcher"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", launcherPath, err)
	}
	if err := os.WriteFile(bundleMarkerPath(appHome), []byte(`{"app":"csgclaw","layout":"official-bundle"}`), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", bundleMarkerPath(appHome), err)
	}
	preparedRoot := writeBundleFiles(t, t.TempDir(), map[string]string{
		filepath.Join("csgclaw", "bin", "csgclaw.exe"): "@echo off\r\nREM new\r\n",
		filepath.Join("csgclaw", "README.md"):          "new",
	})

	client := Client{
		ExecutablePath: func() (string, error) {
			return launcherPath, nil
		},
	}

	installed, err := client.InstallPrepared(PreparedBundle{BundleDir: preparedRoot})
	if err != nil {
		t.Fatalf("InstallPrepared() error = %v", err)
	}
	if got, want := installed.InstallRoot, installRoot; got != want {
		t.Fatalf("InstallRoot = %q, want %q", got, want)
	}
	assertFileContent(t, filepath.Join(installRoot, "README.md"), "new")
}

func TestClientInstallPreparedDoesNotReplaceLauncherRootWhenInnerBundleMissing(t *testing.T) {
	appHome := filepath.Join(t.TempDir(), "csgclaw")
	launcherDir := filepath.Join(appHome, "bin")
	libDir := filepath.Join(appHome, "lib", "csgclaw")
	if err := os.MkdirAll(launcherDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", launcherDir, err)
	}
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", libDir, err)
	}
	launcherPath := filepath.Join(launcherDir, "csgclaw.exe")
	if err := os.WriteFile(launcherPath, []byte("launcher"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", launcherPath, err)
	}
	if err := os.WriteFile(bundleMarkerPath(appHome), []byte(`{"app":"csgclaw","layout":"official-bundle"}`), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", bundleMarkerPath(appHome), err)
	}
	keepPath := filepath.Join(appHome, "keep-me")
	if err := os.WriteFile(keepPath, []byte("user file"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", keepPath, err)
	}

	client := Client{
		ExecutablePath: func() (string, error) {
			return launcherPath, nil
		},
	}

	_, err := client.InstallPrepared(PreparedBundle{BundleDir: writeBundleFiles(t, t.TempDir(), map[string]string{
		filepath.Join("csgclaw", "bin", "csgclaw.exe"): "@echo off\r\nREM new\r\n",
		filepath.Join("csgclaw", "README.md"):          "new",
	})})
	if err == nil || !strings.Contains(err.Error(), "not installed from an official csgclaw bundle") {
		t.Fatalf("InstallPrepared() error = %v, want non-bundle install error", err)
	}
	assertFileContent(t, keepPath, "user file")
}

func TestClientInstallPreparedRollsBackOnRenameFailure(t *testing.T) {
	installParent := t.TempDir()
	installRoot := writeBundleDir(t, installParent, "old")
	preparedRoot := writeBundleDir(t, t.TempDir(), "new")

	originalRename := renamePath
	renameCount := 0
	renamePath = func(oldPath, newPath string) error {
		renameCount++
		if renameCount == 2 {
			return fmt.Errorf("boom")
		}
		return originalRename(oldPath, newPath)
	}
	t.Cleanup(func() { renamePath = originalRename })

	client := Client{
		ExecutablePath: func() (string, error) {
			return filepath.Join(installRoot, "bin", "csgclaw"), nil
		},
	}

	_, err := client.InstallPrepared(PreparedBundle{BundleDir: preparedRoot})
	if err == nil || !strings.Contains(err.Error(), "install new bundle") {
		t.Fatalf("InstallPrepared() error = %v, want install failure", err)
	}
	assertFileContent(t, filepath.Join(installRoot, "README.md"), "old")
	assertFileContent(t, filepath.Join(installRoot, "bin", "boxlite"), "#!/bin/sh\n# old boxlite\n")
}

func TestClientAutoUpgradeSupportReportsOfficialBundle(t *testing.T) {
	installRoot := writeBundleDir(t, t.TempDir(), "old")
	client := Client{
		ExecutablePath: func() (string, error) {
			return filepath.Join(installRoot, "bin", "csgclaw"), nil
		},
	}

	got := client.AutoUpgradeSupport("v0.3.11")
	if !got.Supported {
		t.Fatalf("AutoUpgradeSupport().Supported = false, reason=%q", got.Reason)
	}
	if got.InstallRoot != installRoot {
		t.Fatalf("InstallRoot = %q, want %q", got.InstallRoot, installRoot)
	}
}

func TestClientAutoUpgradeSupportSkipsBundleDetectionForLocalBuild(t *testing.T) {
	client := Client{ExecutablePath: func() (string, error) {
		t.Fatal("local builds should not inspect the executable path")
		return "", nil
	}}

	for _, version := range []string{"dev", "dev+local", "v0.3.11+local"} {
		got := client.AutoUpgradeSupport(version)
		if got.Supported {
			t.Fatalf("AutoUpgradeSupport(%q).Supported = true, want false", version)
		}
		if got.Reason != "local_build" {
			t.Fatalf("AutoUpgradeSupport(%q).Reason = %q, want local_build", version, got.Reason)
		}
	}
}

func TestClientAutoUpgradeSupportReportsNonOfficialBundle(t *testing.T) {
	installRoot := writeBundleFilesWithoutMarker(t, t.TempDir(), map[string]string{
		filepath.Join("csgclaw", "bin", "csgclaw"): "#!/bin/sh\n",
	})
	client := Client{
		ExecutablePath: func() (string, error) {
			return filepath.Join(installRoot, "bin", "csgclaw"), nil
		},
	}

	got := client.AutoUpgradeSupport("v0.3.11")
	if got.Supported {
		t.Fatal("AutoUpgradeSupport().Supported = true, want false")
	}
	if got.Reason != "not_official_bundle" {
		t.Fatalf("Reason = %q, want not_official_bundle", got.Reason)
	}
}

func TestClientAutoUpgradeSupportReportsLegacyOfficialBundle(t *testing.T) {
	installRoot := writeBundleFilesWithoutMarker(t, legacyOfficialInstallParent(t), map[string]string{
		filepath.Join("csgclaw", "bin", "csgclaw"): "#!/bin/sh\n",
	})
	client := Client{
		ExecutablePath: func() (string, error) {
			return filepath.Join(installRoot, "bin", "csgclaw"), nil
		},
	}

	got := client.AutoUpgradeSupport("v0.3.11")
	if !got.Supported {
		t.Fatalf("AutoUpgradeSupport().Supported = false, reason=%q", got.Reason)
	}
	if got.InstallRoot != installRoot {
		t.Fatalf("InstallRoot = %q, want %q", got.InstallRoot, installRoot)
	}
}

func TestClientRestartIfRunningSkipsWhenPIDFileMissing(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)

	client := Client{}
	result, err := client.RestartIfRunning(context.Background(), InstalledBundle{InstallRoot: t.TempDir()}, RestartOptions{})
	if err != nil {
		t.Fatalf("RestartIfRunning() error = %v", err)
	}
	if result.DaemonWasRunning || result.Restarted {
		t.Fatalf("RestartIfRunning() = %#v, want no daemon and no restart", result)
	}
}

func TestClientRestartIfRunningRemovesStalePID(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	pidPath := filepath.Join(configHome, ".csgclaw", "server.pid")
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(pidPath), err)
	}
	if err := os.WriteFile(pidPath, []byte("424242\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", pidPath, err)
	}

	originalFindProcess := findProcessByPID
	findProcessByPID = func(int) (*os.Process, error) {
		return &os.Process{Pid: 424242}, nil
	}
	t.Cleanup(func() { findProcessByPID = originalFindProcess })

	originalExec := execCommandContext
	execCommandContext = func(context.Context, string, ...string) *exec.Cmd {
		t.Fatal("execCommandContext should not be called for stale pid")
		return nil
	}
	t.Cleanup(func() { execCommandContext = originalExec })

	result, err := Client{}.RestartIfRunning(context.Background(), InstalledBundle{InstallRoot: t.TempDir()}, RestartOptions{})
	if err != nil {
		t.Fatalf("RestartIfRunning() error = %v", err)
	}
	if result.DaemonWasRunning || result.Restarted {
		t.Fatalf("RestartIfRunning() = %#v, want no daemon and no restart", result)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("pid file still exists, stat err = %v", err)
	}
}

func TestClientRestartIfRunningStopsAndStartsDaemon(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	pidPath := filepath.Join(configHome, ".csgclaw", "server.pid")
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(pidPath), err)
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", pidPath, err)
	}

	installParent := t.TempDir()
	installRoot := writeBundleDir(t, installParent, "restart")
	var calls [][]string
	originalExec := execCommandContext
	execCommandContext = func(_ context.Context, name string, args ...string) *exec.Cmd {
		calls = append(calls, append([]string{name}, args...))
		return exec.Command("sh", "-c", "exit 0")
	}
	t.Cleanup(func() { execCommandContext = originalExec })

	result, err := Client{}.RestartIfRunning(context.Background(), InstalledBundle{InstallRoot: installRoot}, RestartOptions{
		ConfigPath: "/tmp/custom.toml",
	})
	if err != nil {
		t.Fatalf("RestartIfRunning() error = %v", err)
	}
	if !result.DaemonWasRunning || !result.Restarted {
		t.Fatalf("RestartIfRunning() = %#v, want restarted daemon", result)
	}
	want := [][]string{
		{filepath.Join(installRoot, "bin", "csgclaw"), "stop"},
		{filepath.Join(installRoot, "bin", "csgclaw"), "--config", "/tmp/custom.toml", "serve", "--daemon"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("exec calls = %#v, want %#v", calls, want)
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a    string
		b    string
		want int
	}{
		{a: "v0.2.5", b: "v0.2.7", want: -1},
		{a: "v0.2.7", b: "v0.2.7", want: 0},
		{a: "v0.3.0", b: "v0.2.9", want: 1},
		{a: "v0.2.7-rc.1", b: "v0.2.7", want: -1},
		{a: "v0.2.7", b: "v0.2.7-rc.1", want: 1},
	}

	for _, tt := range tests {
		if got := compareSemver(tt.a, tt.b); got != tt.want {
			t.Fatalf("compareSemver(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestIsUpdatableReleaseVersion(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{version: "dev", want: false},
		{version: "v0.3.5", want: true},
		{version: "v0.3.5-test6", want: true},
		{version: "v0.3.5+local", want: false},
		{version: "v0.3.5-test6+local", want: false},
	}

	for _, tc := range cases {
		if got := isUpdatableReleaseVersion(tc.version); got != tc.want {
			t.Fatalf("isUpdatableReleaseVersion(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}

func TestIsLocalBuildVersion(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{version: "dev", want: false},
		{version: "v0.3.5", want: false},
		{version: "v0.3.5-test6", want: false},
		{version: "v0.3.5+local", want: true},
		{version: "v0.3.5-test6+local", want: true},
	}

	for _, tc := range cases {
		if got := isLocalBuildVersion(tc.version); got != tc.want {
			t.Fatalf("isLocalBuildVersion(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func releaseTarball(t *testing.T, files map[string]string) []byte {
	t.Helper()

	files = withBundleMarker(files)
	return releaseTarballWithoutMarker(t, files)
}

func releaseTarballWithoutMarker(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%q) error = %v", name, err)
		}
		if _, err := io.WriteString(tw, content); err != nil {
			t.Fatalf("WriteString(%q) error = %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close error = %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("gzip close error = %v", err)
	}
	return buf.Bytes()
}

func writeBundleDir(t *testing.T, parentDir, marker string) string {
	t.Helper()

	return writeBundleFiles(t, parentDir, map[string]string{
		filepath.Join("csgclaw", "bin", "csgclaw"): "#!/bin/sh\n# " + marker + "\n",
		filepath.Join("csgclaw", "bin", "boxlite"): "#!/bin/sh\n# " + marker + " boxlite\n",
		filepath.Join("csgclaw", "README.md"):      marker,
	})
}

func releaseZipArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()

	files = withBundleMarker(files)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
		if _, err := io.WriteString(w, content); err != nil {
			t.Fatalf("WriteString(%q) error = %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close error = %v", err)
	}
	return buf.Bytes()
}

func writeBundleFiles(t *testing.T, parentDir string, files map[string]string) string {
	t.Helper()

	files = withBundleMarker(files)
	return writeBundleFilesWithoutMarker(t, parentDir, files)
}

func writeBundleFilesWithoutMarker(t *testing.T, parentDir string, files map[string]string) string {
	t.Helper()

	root := filepath.Join(parentDir, "csgclaw")
	for relPath, content := range files {
		path := filepath.Join(parentDir, relPath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}
	return root
}

func withBundleMarker(files map[string]string) map[string]string {
	out := make(map[string]string, len(files)+1)
	for name, content := range files {
		out[name] = content
	}
	if _, ok := out[filepath.Join("csgclaw", bundleMarkerFileName)]; !ok {
		out[filepath.Join("csgclaw", bundleMarkerFileName)] = `{"app":"csgclaw","layout":"official-bundle","version":"test"}`
	}
	return out
}

func legacyOfficialInstallParent(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".local", "lib", "csgclaw", "v0.3.10")
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("file %q = %q, want %q", path, string(got), want)
	}
}
