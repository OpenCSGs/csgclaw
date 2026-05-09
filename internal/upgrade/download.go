package upgrade

import (
	"archive/tar"
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
	if !strings.HasSuffix(asset.Name, ".tar.gz") {
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
	if err := extractTarGz(archivePath, extractDir); err != nil {
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

func validateBundleDir(bundleDir string) error {
	required := []string{
		filepath.Join(bundleDir, "bin", "csgclaw"),
		filepath.Join(bundleDir, "bin", "boxlite"),
	}
	for _, path := range required {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("release bundle is missing %s", strings.TrimPrefix(path, bundleDir+string(filepath.Separator)))
			}
			return fmt.Errorf("stat %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("release bundle entry %s is not a file", strings.TrimPrefix(path, bundleDir+string(filepath.Separator)))
		}
	}
	return nil
}
