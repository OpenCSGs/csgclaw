package connectorcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func (i Installer) importPreinstalled(ctx context.Context, stage string) (bool, error) {
	if i.PreinstalledDir == "" {
		return false, nil
	}
	source := filepath.Join(i.PreinstalledDir, i.Spec.Executable, i.Spec.Version)
	if _, err := os.Lstat(source); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	data, err := os.ReadFile(filepath.Join(source, "package.json"))
	if err != nil {
		return false, fmt.Errorf("read preinstalled CLI manifest: %w", err)
	}
	var manifest packageManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return false, fmt.Errorf("read preinstalled CLI manifest: %w", err)
	}
	if manifest.Version != i.Spec.Version || manifest.SkillsCommit != i.Spec.Skills.Version {
		return false, nil
	}
	if err := copyPackageTree(ctx, source, stage); err != nil {
		return false, fmt.Errorf("copy preinstalled CLI package: %w", err)
	}
	return true, nil
}

func copyPackageTree(ctx context.Context, source, target string) error {
	return filepath.WalkDir(source, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(source, name)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("preinstalled CLI package must contain regular files")
		}
		input, err := os.Open(name)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600|info.Mode().Perm()&0100)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		return errors.Join(copyErr, output.Close())
	})
}
