package connectorcli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type offlineTransport struct{ calls int }

func (t *offlineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++
	return nil, errors.New("network disabled")
}

func preinstalledFixture(t *testing.T) (Installer, Package, *offlineTransport) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell executable fixture")
	}
	spec := InstallSpec{Version: testVersion, Executable: "lark-cli", VersionArgs: []string{"-v"},
		Releases: map[string]Archive{runtime.GOOS + "-" + runtime.GOARCH: {URL: "https://example.test/cli.tar.gz", SHA256: strings.Repeat("0", 64)}},
		Skills:   SkillArchive{Archive: Archive{URL: "https://example.test/skills.tar.gz", SHA256: strings.Repeat("0", 64)}, Version: "skills-commit", Prefix: "lark-"}}
	root := t.TempDir()
	source := filepath.Join(root, spec.Executable, spec.Version)
	pkg := Package{Executable: filepath.Join(source, "bin", spec.Executable), SkillsDir: filepath.Join(source, "skills"), Version: spec.Version, SkillsVersion: spec.Skills.Version, VersionArgs: spec.VersionArgs}
	if err := os.MkdirAll(filepath.Dir(pkg.Executable), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pkg.Executable, []byte("#!/bin/sh\nprintf 'lark-cli "+testVersion+"\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := installSkills(pkg.SkillsDir, skillArchive(t, "repo/skills/lark-test/SKILL.md", 0), spec.Skills.Prefix); err != nil {
		t.Fatal(err)
	}
	if err := writePackageManifest(pkg); err != nil {
		t.Fatal(err)
	}
	transport := &offlineTransport{}
	return Installer{Root: filepath.Join(t.TempDir(), "resource", "package"), Spec: spec, PreinstalledDir: root, Client: &http.Client{Transport: transport}}, pkg, transport
}

func TestPreinstalledPackageInstallsAndRepairsOffline(t *testing.T) {
	i, source, transport := preinstalledFixture(t)
	ctx := context.Background()
	pkg, err := i.Install(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Executable != filepath.Join(i.Root, testVersion, "bin", "lark-cli") {
		t.Fatalf("executable=%s", pkg.Executable)
	}
	if err := os.WriteFile(filepath.Join(pkg.SkillsDir, "lark-test", "SKILL.md"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Install(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := verifyPackage(ctx, source); err != nil {
		t.Fatalf("source modified: %v", err)
	}
	if err := os.RemoveAll(i.PreinstalledDir); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Install(ctx, nil); err != nil {
		t.Fatalf("persisted package was not reused: %v", err)
	}
	if transport.calls != 0 {
		t.Fatalf("network requests=%d", transport.calls)
	}
}

func TestMissingOrDifferentPreinstalledPackageUsesDownload(t *testing.T) {
	for _, variant := range []string{"missing", "version", "skills"} {
		t.Run(variant, func(t *testing.T) {
			i, _, transport := preinstalledFixture(t)
			switch variant {
			case "missing":
				i.PreinstalledDir = t.TempDir()
			case "version":
				i.Spec.Version = "2.0.0"
			case "skills":
				i.Spec.Skills.Version = "updated-skills"
			}
			if _, err := i.Install(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "network disabled") || transport.calls != 1 {
				t.Fatalf("requests=%d err=%v", transport.calls, err)
			}
		})
	}
}

func TestInvalidPreinstalledPackageDoesNotReplaceExistingFiles(t *testing.T) {
	for _, variant := range []string{"binary", "skills", "manifest", "symlink", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			i, source, transport := preinstalledFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sourceRoot := filepath.Dir(source.SkillsDir)
			var err error
			switch variant {
			case "binary":
				err = os.WriteFile(source.Executable, []byte("modified"), 0700)
			case "skills":
				err = os.WriteFile(filepath.Join(source.SkillsDir, "lark-test", "SKILL.md"), []byte("modified"), 0600)
			case "manifest":
				err = os.WriteFile(filepath.Join(sourceRoot, "package.json"), []byte("invalid"), 0600)
			case "symlink":
				err = os.Symlink(source.Executable, filepath.Join(sourceRoot, "unexpected-link"))
			case "cancelled":
				cancel()
			}
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(i.Root, i.Spec.Version)
			if err := os.MkdirAll(target, 0700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(target, "existing")
			if err := os.WriteFile(marker, []byte("preserved"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := i.Install(ctx, nil); err == nil {
				t.Fatal("invalid package accepted")
			}
			if data, err := os.ReadFile(marker); err != nil || string(data) != "preserved" {
				t.Fatalf("existing files changed: %v", err)
			}
			entries, err := os.ReadDir(i.Root)
			if err != nil || len(entries) != 1 || transport.calls != 0 {
				t.Fatalf("entries=%v requests=%d err=%v", entries, transport.calls, err)
			}
		})
	}
}

func TestPreinstalledPackageRejectsWrongExecutableVersion(t *testing.T) {
	i, source, _ := preinstalledFixture(t)
	if err := os.WriteFile(source.Executable, []byte("#!/bin/sh\nprintf 'other-version\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := writePackageManifest(source); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Install(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "version verification") {
		t.Fatalf("err=%v", err)
	}
}

func TestPreinstalledManifestVersionMismatchUsesDownload(t *testing.T) {
	i, source, transport := preinstalledFixture(t)
	data, _ := json.Marshal(packageManifest{Version: "other", SkillsCommit: source.SkillsVersion})
	if err := os.WriteFile(filepath.Join(filepath.Dir(source.SkillsDir), "package.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Install(context.Background(), nil); err == nil || transport.calls != 1 {
		t.Fatalf("requests=%d err=%v", transport.calls, err)
	}
}
