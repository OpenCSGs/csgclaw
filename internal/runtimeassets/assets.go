package runtimeassets

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"csgclaw/internal/config"
)

const (
	SandboxToolsDirName = "sandbox-tools"
	HostToolsDirName    = "bin"
	SandboxCLIName      = "csgclaw-cli"
	HostCLIBundleDir    = "host"
)

type RefreshResult struct {
	SandboxToolsDir string
	SandboxCLIPath  string
	SandboxCLISync  bool
	HostToolsDir    string
	HostCLIPath     string
	HostCLISync     bool
}

func DefaultSandboxToolsDir() (string, error) {
	return config.DefaultDomainDir(SandboxToolsDirName)
}

func DefaultHostToolsDir() (string, error) {
	return config.DefaultDomainDir(HostToolsDirName)
}

func HostCLIName(goos string) string {
	if strings.EqualFold(strings.TrimSpace(goos), "windows") {
		return SandboxCLIName + ".exe"
	}
	return SandboxCLIName
}

func RefreshFromCurrentExecutable() (RefreshResult, error) {
	exe, err := os.Executable()
	if err != nil {
		return RefreshResult{}, err
	}
	var last RefreshResult
	for _, candidate := range executablePathCandidates(exe) {
		root := filepath.Dir(filepath.Dir(candidate))
		result, err := RefreshFromBundle(root)
		if err != nil {
			return RefreshResult{}, err
		}
		if result.SandboxCLISync || result.HostCLISync {
			return result, nil
		}
		if result.SandboxToolsDir != "" || result.HostToolsDir != "" {
			last = result
		}
	}
	return last, nil
}

func RefreshFromBundle(bundleRoot string) (RefreshResult, error) {
	bundleRoot = strings.TrimSpace(bundleRoot)
	if bundleRoot == "" {
		return RefreshResult{}, nil
	}
	toolsDir, err := DefaultSandboxToolsDir()
	if err != nil {
		return RefreshResult{}, err
	}
	hostToolsDir, err := DefaultHostToolsDir()
	if err != nil {
		return RefreshResult{}, err
	}
	result := RefreshResult{SandboxToolsDir: toolsDir, HostToolsDir: hostToolsDir}
	source := filepath.Join(bundleRoot, "bin", "csgclaw_dir", SandboxCLIName)
	if info, err := os.Stat(source); err == nil && info.Mode().IsRegular() {
		target := filepath.Join(toolsDir, SandboxCLIName)
		if err := SyncFile(source, target, 0o755); err != nil {
			return RefreshResult{}, err
		}
		result.SandboxCLIPath = target
		result.SandboxCLISync = true
	}

	hostCLIName := HostCLIName(runtime.GOOS)
	hostSource := filepath.Join(bundleRoot, "bin", "csgclaw_dir", HostCLIBundleDir, hostCLIName)
	if info, err := os.Stat(hostSource); err == nil && info.Mode().IsRegular() {
		hostTarget := filepath.Join(hostToolsDir, hostCLIName)
		if err := SyncFile(hostSource, hostTarget, 0o755); err != nil {
			return RefreshResult{}, err
		}
		result.HostCLIPath = hostTarget
		result.HostCLISync = true
	}
	return result, nil
}

func SyncFile(source, target string, mode os.FileMode) error {
	same, err := filesHaveSameSHA256(source, target)
	if err != nil {
		return fmt.Errorf("compare runtime asset: %w", err)
	}
	if same {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create runtime asset directory: %w", err)
	}
	src, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open runtime asset: %w", err)
	}
	defer src.Close()
	tmp, err := os.CreateTemp(filepath.Dir(target), ".runtime-asset-*")
	if err != nil {
		return fmt.Errorf("create runtime asset temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod runtime asset temp file: %w", err)
	}
	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("copy runtime asset: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close runtime asset temp file: %w", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		if removeErr := os.Remove(target); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("replace runtime asset: %w", err)
		}
		if retryErr := os.Rename(tmpPath, target); retryErr != nil {
			return fmt.Errorf("replace runtime asset: %w", retryErr)
		}
	}
	return nil
}

func executablePathCandidates(exe string) []string {
	exe = strings.TrimSpace(exe)
	if exe == "" {
		return nil
	}
	candidates := []string{exe}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil && resolved != "" && resolved != exe {
		candidates = append(candidates, resolved)
	}
	return candidates
}

func filesHaveSameSHA256(left, right string) (bool, error) {
	leftHash, err := fileSHA256(left)
	if err != nil {
		return false, err
	}
	rightHash, err := fileSHA256(right)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return leftHash == rightHash, nil
}

func fileSHA256(path string) ([sha256.Size]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return [sha256.Size]byte{}, err
	}
	var sum [sha256.Size]byte
	copy(sum[:], hash.Sum(nil))
	return sum, nil
}
