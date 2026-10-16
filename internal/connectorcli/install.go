package connectorcli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	skillstate "csgclaw/internal/skill/state"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	skilllocal "csgclaw/internal/skill/local"
)

type Package struct {
	Executable    string
	SkillsDir     string
	Version       string
	SkillsVersion string
	VersionArgs   []string
}
type Installer struct {
	Spec            InstallSpec
	Root            string
	Client          *http.Client
	PreinstalledDir string
}

func (i Installer) Install(ctx context.Context, progress func(string)) (Package, error) {
	if err := i.Spec.Validate(); err != nil {
		return Package{}, err
	}
	report := func(stage string) {
		if progress != nil {
			progress(stage)
		}
	}
	report("checking_environment")
	name := i.Spec.Executable
	suffix := ".tar.gz"
	if runtime.GOOS == "windows" {
		name += ".exe"
		suffix = ".zip"
	}
	target := filepath.Join(i.Root, i.Spec.Version)
	pkg := Package{Executable: filepath.Join(target, "bin", name), SkillsDir: filepath.Join(target, "skills"), Version: i.Spec.Version, SkillsVersion: i.Spec.Skills.Version, VersionArgs: i.Spec.VersionArgs}
	if err := verifyPackage(ctx, pkg); err == nil {
		return pkg, nil
	}
	platform := runtime.GOOS + "-" + runtime.GOARCH
	release, ok := i.Spec.Releases[platform]
	if !ok {
		return Package{}, errors.New("CLI is unsupported on this platform")
	}
	if err := os.MkdirAll(i.Root, 0700); err != nil {
		return Package{}, err
	}
	stage, err := os.MkdirTemp(i.Root, ".install-")
	if err != nil {
		return Package{}, err
	}
	defer os.RemoveAll(stage)
	staged := Package{Executable: filepath.Join(stage, "bin", name), SkillsDir: filepath.Join(stage, "skills"), Version: i.Spec.Version, SkillsVersion: i.Spec.Skills.Version, VersionArgs: i.Spec.VersionArgs}
	imported, err := i.importPreinstalled(ctx, stage)
	if err != nil {
		return Package{}, err
	}
	if !imported {
		if err := i.downloadPackage(ctx, staged, release, suffix, report); err != nil {
			return Package{}, err
		}
	}
	report("verifying_installation")
	if err := verifyPackage(ctx, staged); err != nil {
		return Package{}, err
	}
	if err := ctx.Err(); err != nil {
		return Package{}, err
	}
	// 已损坏的同版本目录保留供检查，其他版本保持原状。
	backup := target + ".previous"
	if _, err := os.Stat(target); err == nil {
		if err := os.RemoveAll(backup); err != nil {
			return Package{}, err
		}
		if err := os.Rename(target, backup); err != nil {
			return Package{}, err
		}
	}
	if err := os.Rename(stage, target); err != nil {
		_ = os.Rename(backup, target)
		return Package{}, err
	}
	_ = os.RemoveAll(backup)
	report("completed")
	return pkg, nil
}

func (i Installer) downloadPackage(ctx context.Context, staged Package, release Archive, suffix string, report func(string)) error {
	report("downloading_cli")
	data, err := i.download(ctx, release.URL, release.SHA256, 128<<20)
	if err != nil {
		return err
	}
	binary, err := extractBinary(data, filepath.Base(staged.Executable), suffix)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(staged.Executable), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(staged.Executable, binary, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(staged.SkillsDir, 0700); err != nil {
		return err
	}
	if i.Spec.Skills.URL != "" {
		report("installing_skills")
		data, err = i.download(ctx, i.Spec.Skills.URL, i.Spec.Skills.SHA256, 64<<20)
		if err != nil {
			return err
		}
		if err := installSkills(staged.SkillsDir, data, i.Spec.Skills.Prefix); err != nil {
			return err
		}
	}
	return writePackageManifest(staged)
}

func verifyPackage(ctx context.Context, p Package) error {
	actual, err := packageDigest(p)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(p.SkillsDir), "package.json"))
	if err != nil {
		return err
	}
	var expected packageManifest
	if err := json.Unmarshal(data, &expected); err != nil {
		return err
	}
	if actual != expected {
		return errors.New("installed package checksum mismatch")
	}
	check, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	data, err = exec.CommandContext(check, p.Executable, p.VersionArgs...).Output()
	if err != nil || !strings.Contains(string(data), p.Version) {
		return errors.New("CLI version verification failed")
	}
	entries, err := os.ReadDir(p.SkillsDir)
	if err != nil {
		return errors.New("Skills are missing")
	}
	for _, entry := range entries {
		if _, err := skilllocal.ResolveDir(p.SkillsDir, entry.Name()); err != nil {
			return err
		}
	}
	return nil
}

func (i Installer) download(ctx context.Context, url, want string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := i.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CLI package download returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("package exceeds download limit")
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != strings.ToLower(want) {
		return nil, errors.New("CLI package checksum mismatch")
	}
	return data, nil
}

func extractBinary(data []byte, name, suffix string) ([]byte, error) {
	if suffix == ".zip" {
		archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		for _, f := range archive.File {
			if path.Base(f.Name) == name && f.Mode().IsRegular() {
				reader, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer reader.Close()
				return readBounded(reader, 128<<20)
			}
		}
	} else {
		compressed, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer compressed.Close()
		reader := tar.NewReader(compressed)
		for {
			entry, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if entry.Typeflag == tar.TypeReg && path.Base(entry.Name) == name {
				return readBounded(reader, 128<<20)
			}
		}
	}
	return nil, errors.New("package does not contain the configured executable")
}
func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("archive entry is too large")
	}
	return data, nil
}

func installSkills(root string, data []byte, prefix string) error {
	compressed, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	buffers := map[string]*bytes.Buffer{}
	writers := map[string]*zip.Writer{}
	defer func() {
		for _, w := range writers {
			_ = w.Close()
		}
	}()
	var total int64
	for {
		entry, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		parts := strings.SplitN(entry.Name, "/", 3)
		if len(parts) != 3 || parts[1] != "skills" {
			continue
		}
		rel := parts[2]
		segments := strings.SplitN(rel, "/", 2)
		if len(segments) != 2 || !strings.HasPrefix(segments[0], prefix) {
			continue
		}
		if entry.Typeflag == tar.TypeDir {
			continue
		}
		if entry.Typeflag != tar.TypeReg || path.Clean(rel) != rel || strings.Contains(rel, "\\") || strings.HasPrefix(rel, "/") {
			return errors.New("unsafe Skills archive")
		}
		total += entry.Size
		if entry.Size < 0 || total > 64<<20 {
			return errors.New("Skills archive exceeds size limit")
		}
		name := segments[0]
		if writers[name] == nil {
			buffers[name] = &bytes.Buffer{}
			writers[name] = zip.NewWriter(buffers[name])
		}
		file, err := writers[name].Create(rel)
		if err != nil {
			return err
		}
		if _, err := io.Copy(file, reader); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(writers))
	for name := range writers {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return errors.New("Skills archive is empty")
	}
	for _, name := range names {
		if err := writers[name].Close(); err != nil {
			return err
		}
		delete(writers, name)
		if _, err := skilllocal.InstallArchiveWithOptions(root, name+".zip", buffers[name].Bytes(), skilllocal.InstallArchiveOptions{}); err != nil {
			return err
		}
	}
	return nil
}

type packageManifest struct {
	Version          string `json:"version"`
	SkillsCommit     string `json:"skills_commit"`
	ExecutableSHA256 string `json:"executable_sha256"`
	SkillsSHA256     string `json:"skills_sha256"`
}

func packageDigest(p Package) (packageManifest, error) {
	info, err := os.Lstat(p.Executable)
	if err != nil {
		return packageManifest{}, err
	}
	if !info.Mode().IsRegular() {
		return packageManifest{}, errors.New("invalid CLI executable")
	}
	binary, err := os.ReadFile(p.Executable)
	if err != nil {
		return packageManifest{}, err
	}
	sum := sha256.Sum256(binary)
	skills, err := skillstate.DirectoryDigest(p.SkillsDir)
	return packageManifest{Version: p.Version, SkillsCommit: p.SkillsVersion, ExecutableSHA256: hex.EncodeToString(sum[:]), SkillsSHA256: skills}, err
}
func writePackageManifest(p Package) error {
	manifest, err := packageDigest(p)
	if err != nil {
		return err
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return AtomicWrite(filepath.Join(filepath.Dir(p.SkillsDir), "package.json"), data)
}
