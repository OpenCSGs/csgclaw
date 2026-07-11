package codexcli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	DefaultDownloadBaseURL = "https://csgclaw.opencsg.com/codex-cli/latest"
	EnvDownloadBaseURL     = "CSGCLAW_CODEX_DOWNLOAD_BASE_URL"
	defaultInstallTimeout  = 15 * time.Minute
	maxDownloadSize        = int64(512 << 20)
	maxBinarySize          = int64(512 << 20)
	maxArchiveExpandedSize = int64(768 << 20)
)

var ErrUnsupportedPlatform = errors.New("unsupported Codex CLI platform")

type InstallState string

const (
	InstallStateInstalled    InstallState = "installed"
	InstallStateNotInstalled InstallState = "not_installed"
	InstallStateInstalling   InstallState = "installing"
	InstallStateFailed       InstallState = "failed"
)

type InstallStatus struct {
	State     InstallState
	Installed bool
	Path      string
	Message   string
}

type InstallerOptions struct {
	Locator    Locator
	HTTPClient *http.Client
	BaseURL    string
	GOOS       string
	GOARCH     string
	TargetPath string
}

type Installer struct {
	locator    Locator
	httpClient *http.Client
	baseURL    string
	goos       string
	goarch     string
	targetPath string

	mu        sync.Mutex
	active    *installCall
	lastError string
}

type installCall struct {
	done   chan struct{}
	status InstallStatus
	err    error
}

type downloadPlatform struct {
	os            string
	arch          string
	archiveBinary string
}

func NewInstaller(opts InstallerOptions) *Installer {
	goos := strings.ToLower(strings.TrimSpace(opts.GOOS))
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := strings.ToLower(strings.TrimSpace(opts.GOARCH))
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	locator := opts.Locator
	if strings.TrimSpace(locator.GOOS) == "" {
		locator.GOOS = goos
	}
	if targetPath := strings.TrimSpace(opts.TargetPath); targetPath != "" && strings.TrimSpace(locator.ManagedPath) == "" {
		locator.ManagedPath = targetPath
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultInstallTimeout}
	}
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(os.Getenv(EnvDownloadBaseURL)), "/")
	}
	if baseURL == "" {
		baseURL = DefaultDownloadBaseURL
	}
	return &Installer{
		locator:    locator,
		httpClient: client,
		baseURL:    baseURL,
		goos:       goos,
		goarch:     goarch,
		targetPath: strings.TrimSpace(opts.TargetPath),
	}
}

func (i *Installer) Status() InstallStatus {
	if i == nil {
		return InstallStatus{State: InstallStateFailed, Message: "Codex installer is not configured"}
	}
	if installedPath, err := i.locator.Locate(); err == nil {
		return InstallStatus{State: InstallStateInstalled, Installed: true, Path: installedPath}
	}
	i.mu.Lock()
	installing := i.active != nil
	lastError := i.lastError
	i.mu.Unlock()
	if installing {
		return InstallStatus{State: InstallStateInstalling}
	}
	if lastError != "" {
		return InstallStatus{State: InstallStateFailed, Message: lastError}
	}
	return InstallStatus{State: InstallStateNotInstalled}
}

func (i *Installer) Ensure(ctx context.Context) (InstallStatus, error) {
	if i == nil {
		err := errors.New("Codex installer is not configured")
		return InstallStatus{State: InstallStateFailed, Message: err.Error()}, err
	}
	if status := i.Status(); status.Installed {
		return status, nil
	}

	i.mu.Lock()
	if active := i.active; active != nil {
		i.mu.Unlock()
		select {
		case <-ctx.Done():
			return i.Status(), ctx.Err()
		case <-active.done:
			return active.status, active.err
		}
	}
	call := &installCall{done: make(chan struct{})}
	i.active = call
	i.lastError = ""
	i.mu.Unlock()

	status, err := i.install(ctx)
	if err != nil {
		status = InstallStatus{State: InstallStateFailed, Message: err.Error()}
	}
	i.mu.Lock()
	call.status = status
	call.err = err
	i.active = nil
	if err != nil {
		i.lastError = err.Error()
	}
	close(call.done)
	i.mu.Unlock()
	return status, err
}

func (i *Installer) install(ctx context.Context) (InstallStatus, error) {
	if status := i.Status(); status.Installed {
		return status, nil
	}
	platform, err := resolveDownloadPlatform(i.goos, i.goarch)
	if err != nil {
		return InstallStatus{}, err
	}
	downloadURL, err := i.downloadURL(platform)
	if err != nil {
		return InstallStatus{}, err
	}
	targetPath, err := i.resolvedTargetPath()
	if err != nil {
		return InstallStatus{}, err
	}
	if i.goos == "windows" && !strings.EqualFold(filepath.Ext(targetPath), ".exe") {
		return InstallStatus{}, fmt.Errorf("Codex install target %s must use the .exe extension on Windows", targetPath)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return InstallStatus{}, fmt.Errorf("create Codex install directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return InstallStatus{}, fmt.Errorf("create Codex download request: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "csgclaw-codex-installer")
	resp, err := i.httpClient.Do(req)
	if err != nil {
		return InstallStatus{}, fmt.Errorf("download Codex CLI: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		detail := strings.TrimSpace(string(message))
		if detail != "" {
			return InstallStatus{}, fmt.Errorf("download Codex CLI: server returned %s: %s", resp.Status, detail)
		}
		return InstallStatus{}, fmt.Errorf("download Codex CLI: server returned %s", resp.Status)
	}
	if resp.ContentLength > maxDownloadSize {
		return InstallStatus{}, fmt.Errorf("download Codex CLI: package size %d exceeds limit %d", resp.ContentLength, maxDownloadSize)
	}

	temp, err := os.CreateTemp(filepath.Dir(targetPath), ".codex-install-*")
	if err != nil {
		return InstallStatus{}, fmt.Errorf("create temporary Codex binary: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		if tempPath != "" {
			_ = os.Remove(tempPath)
		}
	}()

	limited := &io.LimitedReader{R: resp.Body, N: maxDownloadSize + 1}
	if i.goos == "windows" {
		err = installWindowsBinary(temp, limited)
	} else {
		err = installUnixArchive(temp, limited, platform.archiveBinary)
	}
	if err != nil {
		return InstallStatus{}, err
	}
	if err := validatePlatformBinary(temp, i.goos, i.goarch); err != nil {
		return InstallStatus{}, err
	}
	if limited.N <= 0 {
		return InstallStatus{}, fmt.Errorf("download Codex CLI: package exceeds limit %d", maxDownloadSize)
	}
	if err := temp.Chmod(0o755); err != nil {
		return InstallStatus{}, fmt.Errorf("make Codex binary executable: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return InstallStatus{}, fmt.Errorf("sync Codex binary: %w", err)
	}
	if err := temp.Close(); err != nil {
		return InstallStatus{}, fmt.Errorf("close Codex binary: %w", err)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return InstallStatus{}, fmt.Errorf("install Codex binary: %w", err)
	}
	tempPath = ""

	installedPath, err := i.locator.Locate()
	if err != nil {
		return InstallStatus{}, fmt.Errorf("verify installed Codex binary: %w", err)
	}
	return InstallStatus{State: InstallStateInstalled, Installed: true, Path: installedPath}, nil
}

func installWindowsBinary(dst *os.File, src io.Reader) error {
	written, err := io.Copy(dst, io.LimitReader(src, maxBinarySize+1))
	if err != nil {
		return fmt.Errorf("write Codex binary: %w", err)
	}
	if written > maxBinarySize {
		return fmt.Errorf("write Codex binary: binary size exceeds limit %d", maxBinarySize)
	}
	return nil
}

func installUnixArchive(dst *os.File, src io.Reader, expectedBinary string) error {
	gzipReader, err := gzip.NewReader(src)
	if err != nil {
		return fmt.Errorf("open Codex archive: %w", err)
	}
	defer gzipReader.Close()
	expanded := &io.LimitedReader{R: gzipReader, N: maxArchiveExpandedSize + 1}
	tarReader := tar.NewReader(expanded)
	found := false
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read Codex archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return fmt.Errorf("read Codex archive: unexpected non-regular entry %q", header.Name)
		}
		if strings.TrimSpace(header.Name) != expectedBinary {
			return fmt.Errorf("read Codex archive: unexpected entry %q", header.Name)
		}
		if found {
			return errors.New("read Codex archive: package contains multiple Codex binaries")
		}
		if header.Size <= 0 || header.Size > maxBinarySize {
			return fmt.Errorf("read Codex archive: binary size %d is invalid", header.Size)
		}
		written, err := io.Copy(dst, io.LimitReader(tarReader, maxBinarySize+1))
		if err != nil {
			return fmt.Errorf("extract Codex binary: %w", err)
		}
		if written != header.Size {
			return fmt.Errorf("extract Codex binary: wrote %d bytes, expected %d", written, header.Size)
		}
		found = true
	}
	if expanded.N <= 0 {
		return fmt.Errorf("read Codex archive: expanded package exceeds limit %d", maxArchiveExpandedSize)
	}
	if !found {
		return errors.New("read Codex archive: package does not contain a Codex binary")
	}
	return nil
}

func validatePlatformBinary(file *os.File, goos, goarch string) error {
	if file == nil {
		return errors.New("validate Codex binary: file is not configured")
	}
	switch strings.ToLower(strings.TrimSpace(goos)) {
	case "linux":
		header := make([]byte, 20)
		if _, err := file.ReadAt(header, 0); err != nil {
			return fmt.Errorf("validate Codex ELF binary: %w", err)
		}
		if !bytes.Equal(header[:4], []byte{0x7f, 'E', 'L', 'F'}) || header[4] != 2 || header[5] != 1 {
			return errors.New("validate Codex ELF binary: invalid 64-bit little-endian ELF header")
		}
		machine := binary.LittleEndian.Uint16(header[18:20])
		wantMachine := uint16(0)
		switch strings.ToLower(strings.TrimSpace(goarch)) {
		case "amd64":
			wantMachine = 62
		case "arm64":
			wantMachine = 183
		}
		if wantMachine == 0 || machine != wantMachine {
			return fmt.Errorf("validate Codex ELF binary: machine %d does not match %s", machine, goarch)
		}
	case "darwin", "macos":
		header := make([]byte, 8)
		if _, err := file.ReadAt(header, 0); err != nil {
			return fmt.Errorf("validate Codex Mach-O binary: %w", err)
		}
		if !bytes.Equal(header[:4], []byte{0xcf, 0xfa, 0xed, 0xfe}) {
			return errors.New("validate Codex Mach-O binary: invalid 64-bit Mach-O header")
		}
		cpuType := binary.LittleEndian.Uint32(header[4:8])
		wantCPUType := uint32(0)
		switch strings.ToLower(strings.TrimSpace(goarch)) {
		case "amd64":
			wantCPUType = 0x01000007
		case "arm64":
			wantCPUType = 0x0100000c
		}
		if wantCPUType == 0 || cpuType != wantCPUType {
			return fmt.Errorf("validate Codex Mach-O binary: CPU type %#x does not match %s", cpuType, goarch)
		}
	case "windows":
		dosHeader := make([]byte, 64)
		if _, err := file.ReadAt(dosHeader, 0); err != nil {
			return fmt.Errorf("validate Codex PE binary: %w", err)
		}
		if string(dosHeader[:2]) != "MZ" {
			return errors.New("validate Codex PE binary: missing MZ header")
		}
		peOffset := int64(binary.LittleEndian.Uint32(dosHeader[0x3c:0x40]))
		info, err := file.Stat()
		if err != nil {
			return fmt.Errorf("validate Codex PE binary: %w", err)
		}
		if peOffset < 64 || peOffset > info.Size()-6 {
			return fmt.Errorf("validate Codex PE binary: invalid PE offset %d", peOffset)
		}
		peHeader := make([]byte, 6)
		if _, err := file.ReadAt(peHeader, peOffset); err != nil {
			return fmt.Errorf("validate Codex PE binary: %w", err)
		}
		if string(peHeader[:4]) != "PE\x00\x00" {
			return errors.New("validate Codex PE binary: missing PE signature")
		}
		machine := binary.LittleEndian.Uint16(peHeader[4:6])
		wantMachine := uint16(0)
		switch strings.ToLower(strings.TrimSpace(goarch)) {
		case "amd64":
			wantMachine = 0x8664
		case "arm64":
			wantMachine = 0xaa64
		}
		if wantMachine == 0 || machine != wantMachine {
			return fmt.Errorf("validate Codex PE binary: machine %#x does not match %s", machine, goarch)
		}
	default:
		return fmt.Errorf("validate Codex binary: unsupported platform %s/%s", goos, goarch)
	}
	return nil
}

func (i *Installer) resolvedTargetPath() (string, error) {
	if targetPath := strings.TrimSpace(i.targetPath); targetPath != "" {
		return targetPath, nil
	}
	if explicitPath := strings.TrimSpace(i.locator.resolvedExplicitPath()); explicitPath != "" {
		if i.locator.isWindowsCommandShim(explicitPath) {
			return i.locator.resolvedManagedPath()
		}
		return explicitPath, nil
	}
	return DefaultManagedPath(i.goos)
}

func (i *Installer) downloadURL(platform downloadPlatform) (string, error) {
	value, err := url.JoinPath(i.baseURL, platform.os, platform.arch)
	if err != nil {
		return "", fmt.Errorf("build Codex download URL: %w", err)
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("build Codex download URL: %w", err)
	}
	query := parsed.Query()
	query.Set("package", "codex-cli")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func resolveDownloadPlatform(goos, goarch string) (downloadPlatform, error) {
	switch strings.ToLower(strings.TrimSpace(goos)) {
	case "linux":
		switch strings.ToLower(strings.TrimSpace(goarch)) {
		case "amd64":
			return downloadPlatform{os: "linux", arch: "amd64", archiveBinary: "codex-x86_64-unknown-linux-musl"}, nil
		case "arm64":
			return downloadPlatform{os: "linux", arch: "arm64", archiveBinary: "codex-aarch64-unknown-linux-musl"}, nil
		}
	case "darwin", "macos":
		switch strings.ToLower(strings.TrimSpace(goarch)) {
		case "amd64":
			return downloadPlatform{os: "macos", arch: "x64", archiveBinary: "codex-x86_64-apple-darwin"}, nil
		case "arm64":
			return downloadPlatform{os: "macos", arch: "arm64", archiveBinary: "codex-aarch64-apple-darwin"}, nil
		}
	case "windows":
		switch strings.ToLower(strings.TrimSpace(goarch)) {
		case "amd64", "arm64":
			return downloadPlatform{os: "windows", arch: strings.ToLower(strings.TrimSpace(goarch))}, nil
		}
	}
	return downloadPlatform{}, fmt.Errorf("%w: %s/%s", ErrUnsupportedPlatform, goos, goarch)
}

// SupportedPlatform reports whether the managed download service publishes a Codex CLI build for the platform.
func SupportedPlatform(goos, goarch string) bool {
	_, err := resolveDownloadPlatform(goos, goarch)
	return err == nil
}
