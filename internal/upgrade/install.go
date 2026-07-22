package upgrade

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"csgclaw/internal/runtimeassets"
)

var (
	evalSymlinks = filepath.EvalSymlinks
	renamePath   = os.Rename
	removeAll    = os.RemoveAll
	mkdirAll     = os.MkdirAll
	readDir      = os.ReadDir
	openFile     = os.OpenFile
	nowUTC       = func() time.Time { return time.Now().UTC() }
)

const originalExecutableEnvVar = "CSGCLAW_ORIGINAL_EXECUTABLE"

var ErrNotOfficialBundle = errors.New("current executable is not installed from an official csgclaw bundle")

type InstalledBundle struct {
	InstallRoot string `json:"install_root,omitempty"`
}

type AutoUpgradeSupport struct {
	Supported   bool
	Reason      string
	InstallRoot string
}

func (c Client) InstallPrepared(prepared PreparedBundle) (InstalledBundle, error) {
	if strings.TrimSpace(prepared.BundleDir) == "" {
		return InstalledBundle{}, fmt.Errorf("prepared bundle dir is required")
	}
	if err := validateBundleDir(prepared.BundleDir); err != nil {
		return InstalledBundle{}, err
	}

	installRoot, err := c.officialInstallRoot()
	if err != nil {
		return InstalledBundle{}, err
	}
	if err := installBundle(prepared.BundleDir, installRoot); err != nil {
		return InstalledBundle{}, err
	}
	if err := refreshCompanionLauncher(installRoot); err != nil {
		return InstalledBundle{}, fmt.Errorf("expose companion CLI: %w", err)
	}
	if _, err := runtimeassets.RefreshFromBundle(installRoot); err != nil {
		return InstalledBundle{}, fmt.Errorf("refresh runtime assets: %w", err)
	}
	return InstalledBundle{InstallRoot: installRoot}, nil
}

func (c Client) officialInstallRoot() (string, error) {
	exe, err := c.resolvedExecutablePath()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}

	for _, candidate := range executablePathCandidates(exe) {
		root, ok := bundleInstallRoot(candidate)
		if !ok {
			continue
		}
		if launcherRoot, ok := installedBundleRootFromLauncher(root); ok {
			return launcherRoot, nil
		}
		if isLauncherInstallRoot(root) {
			continue
		}
		if err := validateBundleDir(root); err == nil {
			return root, nil
		}
		if isLegacyOfficialInstallRoot(root) {
			return root, nil
		}
	}
	return "", ErrNotOfficialBundle
}

func (c Client) resolvedExecutablePath() (string, error) {
	if c.ExecutablePath != nil {
		return c.ExecutablePath()
	}
	if exe := strings.TrimSpace(os.Getenv(originalExecutableEnvVar)); exe != "" {
		return exe, nil
	}
	return os.Executable()
}

func executablePathCandidates(exe string) []string {
	exe = strings.TrimSpace(exe)
	if exe == "" {
		return nil
	}

	candidates := []string{exe}
	if resolved, err := evalSymlinks(exe); err == nil && resolved != "" && resolved != exe {
		candidates = append(candidates, resolved)
	}
	return candidates
}

func bundleInstallRoot(exePath string) (string, bool) {
	exePath = filepath.Clean(strings.TrimSpace(exePath))
	if exePath == "" {
		return "", false
	}
	if !isCSGClawExecutableName(filepath.Base(exePath)) {
		return "", false
	}
	binDir := filepath.Dir(exePath)
	if filepath.Base(binDir) != "bin" {
		return "", false
	}
	return filepath.Dir(binDir), true
}

func isLegacyOfficialInstallRoot(root string) bool {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || filepath.Base(root) != "csgclaw" {
		return false
	}
	if _, err := os.Lstat(bundleMarkerPath(root)); err == nil {
		return false
	} else if !os.IsNotExist(err) {
		return false
	}
	if !isOfficialInstallerManagedPath(root) {
		return false
	}
	if hasSourceCheckoutMarker(root) {
		return false
	}
	if _, err := requiredBundleExecutable(root, "csgclaw"); err != nil {
		return false
	}
	if _, err := optionalBundleExecutable(root, "boxlite"); err != nil {
		return false
	}
	return true
}

func isOfficialInstallerManagedPath(root string) bool {
	versionDir := filepath.Dir(root)
	libAppDir := filepath.Dir(versionDir)
	libDir := filepath.Dir(libAppDir)
	if filepath.Base(libAppDir) != "csgclaw" {
		return false
	}
	if filepath.Base(libDir) != "lib" {
		return false
	}
	return filepath.Base(versionDir) != "." && filepath.Base(versionDir) != string(filepath.Separator)
}

func refreshCompanionLauncher(installRoot string) error {
	if !isOfficialInstallerManagedPath(installRoot) {
		return nil
	}
	versionDir := filepath.Dir(installRoot)
	libAppDir := filepath.Dir(versionDir)
	libDir := filepath.Dir(libAppDir)
	launcherDir := filepath.Join(filepath.Dir(libDir), "bin")
	if !hasLauncher(launcherDir, "csgclaw") {
		return nil
	}

	source, err := optionalBundleExecutable(installRoot, "csgclaw-cli")
	if err != nil || source == "" {
		return err
	}
	target := filepath.Join(launcherDir, filepath.Base(source))
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("companion launcher %s is not a regular file", target)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat companion launcher: %w", err)
	} else if hasLauncher(launcherDir, "csgclaw-cli") {
		return nil
	}
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat companion CLI: %w", err)
	}
	return copyFile(source, target, info.Mode())
}

func hasLauncher(dir, baseName string) bool {
	for _, name := range []string{baseName, baseName + ".exe", baseName + ".cmd"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func installedBundleRootFromLauncher(root string) (string, bool) {
	root = filepath.Clean(strings.TrimSpace(root))
	if !isLauncherInstallRoot(root) {
		return "", false
	}

	libDir := filepath.Join(root, "lib", "csgclaw")
	entries, err := readDir(libDir)
	if err != nil {
		return "", false
	}

	type bundleCandidate struct {
		version string
		root    string
	}

	var candidates []bundleCandidate
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		bundleRoot := filepath.Join(libDir, entry.Name(), "csgclaw")
		if err := validateBundleDir(bundleRoot); err != nil && !isLegacyOfficialInstallRoot(bundleRoot) {
			continue
		}
		candidates = append(candidates, bundleCandidate{
			version: entry.Name(),
			root:    bundleRoot,
		})
	}
	if len(candidates) == 0 {
		return "", false
	}
	slices.SortFunc(candidates, func(a, b bundleCandidate) int {
		return compareSemver(b.version, a.version)
	})
	return candidates[0].root, true
}

func isLauncherInstallRoot(root string) bool {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || filepath.Base(root) != "csgclaw" {
		return false
	}
	info, err := os.Lstat(filepath.Join(root, "lib", "csgclaw"))
	return err == nil && info.IsDir()
}

func hasSourceCheckoutMarker(root string) bool {
	for _, name := range []string{".git", "go.mod", "go.work"} {
		if _, err := os.Lstat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}

func (c Client) AutoUpgradeSupport(currentVersion string) AutoUpgradeSupport {
	currentVersion = strings.TrimSpace(currentVersion)
	if currentVersion == "dev" || isLocalBuildVersion(currentVersion) {
		return AutoUpgradeSupport{Reason: "local_build"}
	}

	root, err := c.officialInstallRoot()
	if err == nil {
		return AutoUpgradeSupport{
			Supported:   true,
			InstallRoot: root,
		}
	}
	if errors.Is(err, ErrNotOfficialBundle) {
		return AutoUpgradeSupport{Reason: "not_official_bundle"}
	}
	return AutoUpgradeSupport{Reason: "unknown"}
}

func isCSGClawExecutableName(name string) bool {
	switch strings.TrimSpace(name) {
	case "csgclaw", "csgclaw.exe":
		return true
	default:
		return false
	}
}

func installBundle(bundleDir, installRoot string) error {
	if err := validateBundleDir(installRoot); err != nil && !isLegacyOfficialInstallRoot(installRoot) {
		return fmt.Errorf("%w: invalid current bundle: %v", ErrNotOfficialBundle, err)
	}

	parentDir := filepath.Dir(installRoot)
	baseName := filepath.Base(installRoot)

	stagingParent, err := os.MkdirTemp(parentDir, baseName+".upgrade-*")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}
	stagingRoot := filepath.Join(stagingParent, baseName)
	cleanupStaging := true
	defer func() {
		if cleanupStaging {
			_ = removeAll(stagingParent)
		}
	}()

	if err := copyDir(bundleDir, stagingRoot); err != nil {
		return err
	}

	backupRoot := filepath.Join(parentDir, baseName+".backup."+nowUTC().Format("20060102150405"))
	if err := renamePath(installRoot, backupRoot); err != nil {
		return fmt.Errorf("backup current bundle: %w", err)
	}

	if err := renamePath(stagingRoot, installRoot); err != nil {
		if rollbackErr := renamePath(backupRoot, installRoot); rollbackErr != nil {
			return fmt.Errorf("install new bundle: %v; rollback failed: %w", err, rollbackErr)
		}
		return fmt.Errorf("install new bundle: %w", err)
	}

	cleanupStaging = false
	_ = removeAll(stagingParent)
	_ = removeAll(backupRoot)
	return nil
}

func copyDir(srcDir, dstDir string) error {
	entries, err := readDir(srcDir)
	if err != nil {
		return fmt.Errorf("read %s: %w", srcDir, err)
	}
	if err := mkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dstDir, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", srcPath, err)
		}
		switch {
		case info.IsDir():
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if err := copyFile(srcPath, dstPath, info.Mode()); err != nil {
				return err
			}
		default:
			return fmt.Errorf("bundle entry %s has unsupported mode %s", srcPath, info.Mode())
		}
	}
	return nil
}

func copyFile(srcPath, dstPath string, mode fs.FileMode) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", srcPath, err)
	}
	defer src.Close()

	if err := mkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dstPath), err)
	}
	dst, err := openFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return fmt.Errorf("open %s: %w", dstPath, err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return fmt.Errorf("write %s: %w", dstPath, err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("close %s: %w", dstPath, err)
	}
	return nil
}
