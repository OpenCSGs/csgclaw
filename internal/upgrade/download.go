package upgrade

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type PreparedBundle struct {
	WorkDir     string
	ArchivePath string
	BundleDir   string
}

// BundleLayout describes the official csgclaw bundle shape after extraction.
// All official bundles use the same root directory layout:
//
//	csgclaw/
//	  bin/
//	    csgclaw[.exe]
//	    boxlite[.exe]   (optional)
//
// The main csgclaw binary is always required. Bundled boxlite is optional so
// release artifacts can support platforms where only the Docker-backed runtime
// is shipped.
type BundleLayout struct {
	RootDir     string
	CSGClawPath string
	BoxLitePath string
}

func (c Client) PrepareRelease(ctx context.Context, asset ReleaseAsset, parentDir string) (PreparedBundle, error) {
	if strings.TrimSpace(asset.Name) == "" {
		return PreparedBundle{}, fmt.Errorf("release asset name is required")
	}
	if strings.TrimSpace(asset.DownloadURL) == "" {
		return PreparedBundle{}, fmt.Errorf("release asset %s is missing download URL", asset.Name)
	}
	if asset.Size <= 0 {
		return PreparedBundle{}, fmt.Errorf("release asset %s is missing size metadata", asset.Name)
	}
	// Temporary: some published release assets do not include sha256 metadata yet.
	// Restore the guard below once release assets consistently publish sha256 again.
	// if strings.TrimSpace(asset.SHA256) == "" {
	// 	return PreparedBundle{}, fmt.Errorf("release asset %s is missing sha256 metadata", asset.Name)
	// }
	switch {
	case strings.HasSuffix(asset.Name, ".tar.gz"):
	case strings.HasSuffix(asset.Name, ".zip"):
	default:
		return PreparedBundle{}, fmt.Errorf("unsupported release archive format for %s", asset.Name)
	}

	workDir, err := os.MkdirTemp(parentDir, "csgclaw-upgrade-*")
	if err != nil {
		return PreparedBundle{}, fmt.Errorf("create temp dir: %w", err)
	}
	keepWorkDir := false
	defer func() {
		if !keepWorkDir {
			_ = os.RemoveAll(workDir)
		}
	}()

	archivePath := filepath.Join(workDir, asset.Name)
	if err := c.downloadAsset(ctx, asset, archivePath); err != nil {
		return PreparedBundle{}, err
	}

	extractDir := filepath.Join(workDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return PreparedBundle{}, fmt.Errorf("mkdir %s: %w", extractDir, err)
	}
	if err := extractArchive(archivePath, extractDir); err != nil {
		return PreparedBundle{}, err
	}

	bundleDir := filepath.Join(extractDir, "csgclaw")
	if err := validateBundleDir(bundleDir); err != nil {
		return PreparedBundle{}, err
	}

	keepWorkDir = true
	return PreparedBundle{
		WorkDir:     workDir,
		ArchivePath: archivePath,
		BundleDir:   bundleDir,
	}, nil
}

func (c Client) downloadAsset(ctx context.Context, asset ReleaseAsset, targetPath string) error {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download release asset %s: %w", asset.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download release asset %s: unexpected status %s", asset.Name, resp.Status)
	}

	file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", targetPath, err)
	}

	// Temporary: skip sha256 verification until release assets consistently ship
	// sha256 metadata again. Keep the original verification logic below for quick restore.
	// hash := sha256.New()
	// written, err := io.Copy(io.MultiWriter(file, hash), resp.Body)
	written, err := io.Copy(file, resp.Body)
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("write %s: %w", targetPath, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", targetPath, err)
	}

	if written != asset.Size {
		return fmt.Errorf("downloaded size mismatch for %s: got %d want %d", asset.Name, written, asset.Size)
	}
	// gotSHA := hex.EncodeToString(hash.Sum(nil))
	// wantSHA := strings.ToLower(strings.TrimSpace(asset.SHA256))
	// if gotSHA != wantSHA {
	// 	return fmt.Errorf("downloaded sha256 mismatch for %s: got %s want %s", asset.Name, gotSHA, wantSHA)
	// }
	return nil
}

func extractTarGz(archivePath, targetDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", archivePath, err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("read release archive %s: %w", archivePath, err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read release archive entry: %w", err)
		}

		targetPath, err := archiveTargetPath(targetDir, hdr.Name)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", targetPath, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(targetPath), err)
			}
			mode := os.FileMode(hdr.Mode)
			if mode == 0 {
				mode = 0o644
			}
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return fmt.Errorf("open %s: %w", targetPath, err)
			}
			if _, err := io.Copy(file, tr); err != nil {
				_ = file.Close()
				return fmt.Errorf("write %s: %w", targetPath, err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("close %s: %w", targetPath, err)
			}
		default:
			return fmt.Errorf("release archive contains unsupported entry %s", hdr.Name)
		}
	}
}

func extractZip(archivePath, targetDir string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("read release archive %s: %w", archivePath, err)
	}
	defer zr.Close()

	for _, file := range zr.File {
		targetPath, err := archiveTargetPath(targetDir, file.Name)
		if err != nil {
			return err
		}

		info := file.FileInfo()
		switch {
		case info.IsDir():
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", targetPath, err)
			}
		case info.Mode().IsRegular():
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(targetPath), err)
			}
			mode := info.Mode().Perm()
			if mode == 0 {
				mode = 0o644
			}
			src, err := file.Open()
			if err != nil {
				return fmt.Errorf("open release archive entry %s: %w", file.Name, err)
			}
			dst, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				_ = src.Close()
				return fmt.Errorf("open %s: %w", targetPath, err)
			}
			if _, err := io.Copy(dst, src); err != nil {
				_ = dst.Close()
				_ = src.Close()
				return fmt.Errorf("write %s: %w", targetPath, err)
			}
			if err := dst.Close(); err != nil {
				_ = src.Close()
				return fmt.Errorf("close %s: %w", targetPath, err)
			}
			if err := src.Close(); err != nil {
				return fmt.Errorf("close release archive entry %s: %w", file.Name, err)
			}
		default:
			return fmt.Errorf("release archive contains unsupported entry %s", file.Name)
		}
	}
	return nil
}

func extractArchive(archivePath, targetDir string) error {
	switch {
	case strings.HasSuffix(archivePath, ".tar.gz"):
		return extractTarGz(archivePath, targetDir)
	case strings.HasSuffix(archivePath, ".zip"):
		return extractZip(archivePath, targetDir)
	default:
		return fmt.Errorf("unsupported release archive format for %s", filepath.Base(archivePath))
	}
}

func archiveTargetPath(rootDir, name string) (string, error) {
	cleanName := filepath.Clean(strings.TrimSpace(name))
	if cleanName == "." || cleanName == "" {
		return "", fmt.Errorf("release archive contains invalid entry %q", name)
	}
	if filepath.IsAbs(cleanName) {
		return "", fmt.Errorf("release archive contains invalid entry %q", name)
	}
	targetPath := filepath.Join(rootDir, cleanName)
	rel, err := filepath.Rel(rootDir, targetPath)
	if err != nil {
		return "", fmt.Errorf("resolve archive entry %q: %w", name, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("release archive contains invalid entry %q", name)
	}
	return targetPath, nil
}

func inspectBundleDir(bundleDir string) (BundleLayout, error) {
	if err := validateBundleMarker(bundleDir); err != nil {
		return BundleLayout{}, err
	}
	csgclawPath, err := requiredBundleExecutable(bundleDir, "csgclaw")
	if err != nil {
		return BundleLayout{}, err
	}
	boxlitePath, err := optionalBundleExecutable(bundleDir, "boxlite")
	if err != nil {
		return BundleLayout{}, err
	}
	return BundleLayout{
		RootDir:     bundleDir,
		CSGClawPath: csgclawPath,
		BoxLitePath: boxlitePath,
	}, nil
}

func validateBundleDir(bundleDir string) error {
	_, err := inspectBundleDir(bundleDir)
	return err
}

func requireRegularBundleFile(bundleDir, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("release bundle is missing %s", bundleRelativePath(bundleDir, path))
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("release bundle entry %s is not a file", bundleRelativePath(bundleDir, path))
	}
	return nil
}

func requiredBundleExecutable(bundleDir, baseName string) (string, error) {
	for _, path := range bundleExecutableCandidates(bundleDir, baseName) {
		if _, err := os.Lstat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("stat %s: %w", path, err)
		}
		if err := requireRegularBundleFile(bundleDir, path); err == nil {
			return path, nil
		} else {
			return "", err
		}
	}
	return "", fmt.Errorf("release bundle is missing %s", filepath.Join("bin", baseName))
}

func optionalRegularBundleFile(bundleDir, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("release bundle entry %s is not a file", bundleRelativePath(bundleDir, path))
	}
	return nil
}

func optionalBundleExecutable(bundleDir, baseName string) (string, error) {
	for _, path := range bundleExecutableCandidates(bundleDir, baseName) {
		if _, err := os.Lstat(path); err == nil {
			if err := optionalRegularBundleFile(bundleDir, path); err != nil {
				return "", err
			}
			return path, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat %s: %w", path, err)
		}
	}
	return "", nil
}

func bundleExecutableCandidates(bundleDir, baseName string) []string {
	return []string{
		filepath.Join(bundleDir, "bin", baseName),
		filepath.Join(bundleDir, "bin", baseName+".exe"),
	}
}

func bundleRelativePath(bundleDir, path string) string {
	return strings.TrimPrefix(path, bundleDir+string(filepath.Separator))
}
