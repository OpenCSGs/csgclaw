package dshcli

import (
	"archive/tar"
	"archive/zip"
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
	"runtime"
	"sort"
	"strings"
)

const (
	DefaultNodeMirrorURL   = "https://npmmirror.com/mirrors/node/latest-v24.x"
	OfficialNodeReleaseURL = "https://nodejs.org/dist/latest-v24.x"
	maxNodeManifestBytes   = 4 << 20
	maxNodeArchiveBytes    = 512 << 20
	maxNodeExtractedBytes  = 1 << 30
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type ManagedNodeInstaller func(context.Context, string, ProgressReporter) (nodeRuntime, error)

type nodeRuntime struct {
	NodePath  string
	NPMPath   string
	NPMCLI    string
	BinDir    string
	Version   string
	IsManaged bool
}

type nodeRelease struct {
	Version  string
	Filename string
	Checksum [sha256.Size]byte
}

func (runtime nodeRuntime) npmCommand() (string, []string) {
	if runtime.NPMCLI != "" {
		return runtime.NodePath, []string{runtime.NPMCLI}
	}
	return runtime.NPMPath, nil
}

func (i Installer) resolveNodeRuntime(ctx context.Context, installRoot string, report ProgressReporter) (nodeRuntime, error) {
	system, systemErr := i.systemNodeRuntime(ctx)
	if systemErr == nil {
		return system, nil
	}
	if managed, ok := i.findManagedNodeRuntime(ctx, installRoot); ok {
		return managed, nil
	}
	reportProgress(report, InstallStageNode, 0)
	installer := i.InstallManagedNode
	if installer == nil {
		installer = i.installManagedNode
	}
	managed, err := installer(ctx, installRoot, report)
	if err != nil {
		return nodeRuntime{}, fmt.Errorf("%w: %v (system Node.js unavailable: %v)", ErrNodeInstallFailed, err, systemErr)
	}
	if err := i.validateNodeRuntime(ctx, managed); err != nil {
		return nodeRuntime{}, fmt.Errorf("%w: %v", ErrNodeInstallFailed, err)
	}
	return managed, nil
}

func (i Installer) systemNodeRuntime(ctx context.Context) (nodeRuntime, error) {
	lookPath := i.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	nodePath, err := lookPath("node")
	if err != nil {
		return nodeRuntime{}, ErrNodeNotFound
	}
	versionOutput, err := i.run(ctx, nodePath, "--version")
	if err != nil {
		return nodeRuntime{}, fmt.Errorf("%w: run node --version: %v%s", ErrNodeUnsupported, err, commandOutputSuffix(versionOutput))
	}
	if !supportedNodeVersion(string(versionOutput)) {
		return nodeRuntime{}, fmt.Errorf("%w: found %s", ErrNodeUnsupported, strings.TrimSpace(string(versionOutput)))
	}
	npmPath, err := lookPath("npm")
	if err != nil {
		return nodeRuntime{}, ErrNPMNotFound
	}
	return nodeRuntime{NodePath: nodePath, NPMPath: npmPath, Version: strings.TrimSpace(string(versionOutput))}, nil
}

func (i Installer) findManagedNodeRuntime(ctx context.Context, installRoot string) (nodeRuntime, bool) {
	entries, err := os.ReadDir(installRoot)
	if err != nil {
		return nodeRuntime{}, false
	}
	var roots []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "node-v") {
			roots = append(roots, filepath.Join(installRoot, entry.Name()))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(roots)))
	for _, root := range roots {
		candidate := managedNodeRuntimeAt(root, i.resolvedGOOS())
		if i.validateNodeRuntime(ctx, candidate) == nil {
			return candidate, true
		}
	}
	return nodeRuntime{}, false
}

func (i Installer) validateNodeRuntime(ctx context.Context, candidate nodeRuntime) error {
	if candidate.NodePath == "" || candidate.NPMCLI == "" {
		return errors.New("managed Node.js runtime is incomplete")
	}
	for _, path := range []string{candidate.NodePath, candidate.NPMCLI} {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return fmt.Errorf("managed Node.js file %q is unavailable", path)
		}
	}
	output, err := i.run(ctx, candidate.NodePath, "--version")
	if err != nil {
		return fmt.Errorf("run managed node --version: %w%s", err, commandOutputSuffix(output))
	}
	if !supportedNodeVersion(string(output)) {
		return fmt.Errorf("managed Node.js version %q is unsupported", strings.TrimSpace(string(output)))
	}
	candidate.Version = strings.TrimSpace(string(output))
	return nil
}

func (i Installer) installManagedNode(ctx context.Context, installRoot string, _ ProgressReporter) (nodeRuntime, error) {
	var failures []error
	for _, baseURL := range i.nodeReleaseURLs() {
		managed, err := i.installManagedNodeFromURL(ctx, installRoot, baseURL)
		if err == nil {
			return managed, nil
		}
		failures = append(failures, fmt.Errorf("%s: %w", baseURL, err))
	}
	return nodeRuntime{}, fmt.Errorf("install Node.js 24 LTS: %w", errors.Join(failures...))
}

func (i Installer) nodeReleaseURLs() []string {
	if baseURL := strings.TrimRight(strings.TrimSpace(i.NodeReleaseURL), "/"); baseURL != "" {
		return []string{baseURL}
	}
	return []string{DefaultNodeMirrorURL, OfficialNodeReleaseURL}
}

func (i Installer) installManagedNodeFromURL(ctx context.Context, installRoot, baseURL string) (nodeRuntime, error) {
	goos, goarch := i.resolvedGOOS(), i.resolvedGOARCH()
	manifest, err := i.fetchBytes(ctx, baseURL+"/SHASUMS256.txt", maxNodeManifestBytes)
	if err != nil {
		return nodeRuntime{}, fmt.Errorf("download Node.js checksums: %w", err)
	}
	release, err := selectNodeRelease(string(manifest), goos, goarch)
	if err != nil {
		return nodeRuntime{}, err
	}
	targetRoot := filepath.Join(installRoot, "node-v"+release.Version+"-"+goos+"-"+goarch)
	if candidate := managedNodeRuntimeAt(targetRoot, goos); i.validateNodeRuntime(ctx, candidate) == nil {
		return candidate, nil
	}
	tempRoot, err := os.MkdirTemp(installRoot, ".node-install-")
	if err != nil {
		return nodeRuntime{}, fmt.Errorf("prepare Node.js download: %w", err)
	}
	defer os.RemoveAll(tempRoot)
	archivePath := filepath.Join(tempRoot, release.Filename)
	if err := i.downloadNodeArchive(ctx, baseURL+"/"+release.Filename, archivePath, release.Checksum); err != nil {
		return nodeRuntime{}, err
	}
	stagedRoot := filepath.Join(tempRoot, "node")
	if err := os.MkdirAll(stagedRoot, 0o755); err != nil {
		return nodeRuntime{}, fmt.Errorf("prepare Node.js extraction: %w", err)
	}
	if err := extractNodeArchive(archivePath, stagedRoot); err != nil {
		return nodeRuntime{}, err
	}
	staged := managedNodeRuntimeAt(stagedRoot, goos)
	if err := i.validateNodeRuntime(ctx, staged); err != nil {
		return nodeRuntime{}, err
	}
	if _, err := os.Stat(targetRoot); err == nil {
		return nodeRuntime{}, fmt.Errorf("managed Node.js target already exists but is invalid: %s", targetRoot)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nodeRuntime{}, fmt.Errorf("inspect managed Node.js target: %w", err)
	}
	if err := os.Rename(stagedRoot, targetRoot); err != nil {
		return nodeRuntime{}, fmt.Errorf("activate managed Node.js: %w", err)
	}
	return managedNodeRuntimeAt(targetRoot, goos), nil
}

func (i Installer) fetchBytes(ctx context.Context, url string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := i.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %s", url, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("GET %s exceeded %d bytes", url, limit)
	}
	return data, nil
}

func (i Installer) downloadNodeArchive(ctx context.Context, url, destination string, want [sha256.Size]byte) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := i.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download Node.js archive: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download Node.js archive: GET %s returned %s", url, response.Status)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create Node.js archive: %w", err)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, maxNodeArchiveBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("download Node.js archive: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("save Node.js archive: %w", closeErr)
	}
	if written > maxNodeArchiveBytes {
		return fmt.Errorf("Node.js archive exceeded %d bytes", maxNodeArchiveBytes)
	}
	if got := hash.Sum(nil); !equalChecksum(got, want[:]) {
		return errors.New("Node.js archive checksum mismatch")
	}
	return nil
}

func selectNodeRelease(manifest, goos, goarch string) (nodeRelease, error) {
	platform, err := nodePlatform(goos, goarch)
	if err != nil {
		return nodeRelease{}, err
	}
	extension := ".tar.gz"
	if goos == "windows" {
		extension = ".zip"
	}
	suffix := "-" + platform + extension
	for _, line := range strings.Split(manifest, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !strings.HasPrefix(fields[1], "node-v") || !strings.HasSuffix(fields[1], suffix) {
			continue
		}
		digest, decodeErr := hex.DecodeString(fields[0])
		if decodeErr != nil || len(digest) != sha256.Size {
			return nodeRelease{}, errors.New("invalid Node.js checksum manifest")
		}
		version := strings.TrimSuffix(strings.TrimPrefix(fields[1], "node-v"), suffix)
		if version == "" || !strings.HasPrefix(version, "24.") {
			return nodeRelease{}, errors.New("invalid Node.js release filename")
		}
		var checksum [sha256.Size]byte
		copy(checksum[:], digest)
		return nodeRelease{Version: version, Filename: fields[1], Checksum: checksum}, nil
	}
	return nodeRelease{}, fmt.Errorf("Node.js 24 LTS is unavailable for %s/%s", goos, goarch)
}

func nodePlatform(goos, goarch string) (string, error) {
	arch := ""
	switch goarch {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("managed Node.js does not support architecture %q", goarch)
	}
	switch goos {
	case "darwin", "linux":
		return goos + "-" + arch, nil
	case "windows":
		return "win-" + arch, nil
	default:
		return "", fmt.Errorf("managed Node.js does not support operating system %q", goos)
	}
}

func managedNodeRuntimeAt(root, goos string) nodeRuntime {
	if goos == "windows" {
		return nodeRuntime{
			NodePath:  filepath.Join(root, "node.exe"),
			NPMCLI:    filepath.Join(root, "node_modules", "npm", "bin", "npm-cli.js"),
			BinDir:    root,
			IsManaged: true,
		}
	}
	return nodeRuntime{
		NodePath:  filepath.Join(root, "bin", "node"),
		NPMCLI:    filepath.Join(root, "lib", "node_modules", "npm", "bin", "npm-cli.js"),
		BinDir:    filepath.Join(root, "bin"),
		IsManaged: true,
	}
}

func extractNodeArchive(archivePath, destination string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractNodeZIP(archivePath, destination)
	}
	return extractNodeTarGZ(archivePath, destination)
}

func extractNodeTarGZ(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open Node.js archive: %w", err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open Node.js gzip archive: %w", err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	var extracted int64
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read Node.js archive: %w", err)
		}
		target, ok, err := strippedArchiveTarget(destination, header.Name)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			extracted += header.Size
			if extracted > maxNodeExtractedBytes {
				return errors.New("extracted Node.js archive is too large")
			}
			if err := writeArchiveFile(target, reader, header.FileInfo().Mode().Perm()); err != nil {
				return err
			}
		}
	}
}

func extractNodeZIP(archivePath, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open Node.js zip archive: %w", err)
	}
	defer reader.Close()
	var extracted int64
	for _, entry := range reader.File {
		target, ok, err := strippedArchiveTarget(destination, entry.Name)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		extracted += int64(entry.UncompressedSize64)
		if extracted > maxNodeExtractedBytes {
			return errors.New("extracted Node.js archive is too large")
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		writeErr := writeArchiveFile(target, source, entry.Mode().Perm())
		closeErr := source.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func strippedArchiveTarget(destination, name string) (string, bool, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false, errors.New("unsafe path in Node.js archive")
	}
	parts := strings.Split(filepath.ToSlash(clean), "/")
	if len(parts) < 2 {
		return "", false, nil
	}
	relative := filepath.Join(parts[1:]...)
	target := filepath.Join(destination, relative)
	rel, err := filepath.Rel(destination, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false, errors.New("unsafe path in Node.js archive")
	}
	return target, true, nil
}

func writeArchiveFile(path string, source io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o644
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, source)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func equalChecksum(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var different byte
	for index := range left {
		different |= left[index] ^ right[index]
	}
	return different == 0
}

func (i Installer) resolvedGOOS() string {
	if value := strings.TrimSpace(i.GOOS); value != "" {
		return value
	}
	return runtime.GOOS
}

func (i Installer) resolvedGOARCH() string {
	if value := strings.TrimSpace(i.GOARCH); value != "" {
		return value
	}
	return runtime.GOARCH
}
