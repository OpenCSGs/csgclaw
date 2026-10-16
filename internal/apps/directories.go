package apps

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type processDirs struct{ Home, Data, Plugin, Temp string }

func privateDirectory(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("App directory is not a regular directory")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect App directory")
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return fmt.Errorf("create private App directory")
	}
	if err := os.Chmod(path, 0700); err != nil {
		return fmt.Errorf("protect App directory permissions")
	}
	return nil
}

func (s *Service) installationPath(id string) (string, error) {
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != 16 {
		return "", fmt.Errorf("invalid App installation ID")
	}
	return filepath.Join(s.dataRoot, id), nil
}

func (s *Service) prepareInstallation(r record) (processDirs, error) {
	root, err := s.installationPath(r.InstallationID)
	if err != nil {
		return processDirs{}, err
	}
	if err := privateDirectory(s.dataRoot); err != nil {
		return processDirs{}, err
	}
	return prepareProcessDirs(root, r.Package)
}

func prepareProcessDirs(root string, pkg pluginPackage) (processDirs, error) {
	dirs := processDirs{Home: filepath.Join(root, "home"), Data: filepath.Join(root, "data"), Plugin: filepath.Join(root, "plugin"), Temp: filepath.Join(root, "tmp")}
	for _, dir := range []string{root, dirs.Home, dirs.Data, dirs.Plugin, dirs.Temp, filepath.Join(dirs.Home, ".config"), filepath.Join(dirs.Home, ".cache"), filepath.Join(dirs.Home, ".local"), filepath.Join(dirs.Home, ".local", "share")} {
		if err := privateDirectory(dir); err != nil {
			return processDirs{}, err
		}
	}
	files := map[string]any{"plugin.json": pkg.Manifest}
	if pkg.Manifest.Apps != "" {
		files[strings.TrimPrefix(pkg.Manifest.Apps, "./")] = map[string]any{"apps": pkg.Apps}
	}
	if pkg.Manifest.MCP != "" {
		files[strings.TrimPrefix(pkg.Manifest.MCP, "./")] = pkg.MCP
	}
	if pkg.Manifest.CLI != "" {
		files[strings.TrimPrefix(pkg.Manifest.CLI, "./")] = pkg.CLI
	}
	for filename, value := range files {
		if !fs.ValidPath(filename) || strings.Contains(filename, "\\") {
			return processDirs{}, fmt.Errorf("invalid package snapshot path")
		}

		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return processDirs{}, fmt.Errorf("encode App package snapshot")
		}
		data = append(data, '\n')
		file := filepath.Join(dirs.Plugin, filename)
		if err := privatePackageDirectory(dirs.Plugin, filepath.Dir(filename)); err != nil {
			return processDirs{}, err
		}
		if info, err := os.Lstat(file); err == nil {
			if !info.Mode().IsRegular() {
				return processDirs{}, fmt.Errorf("App snapshot is not a regular file")
			}
			if existing, err := os.ReadFile(file); err == nil && bytes.Equal(existing, data) {
				continue
			}
		} else if !os.IsNotExist(err) {
			return processDirs{}, fmt.Errorf("inspect App package snapshot")
		}
		if err := os.WriteFile(file, data, 0600); err != nil {
			return processDirs{}, fmt.Errorf("write App package snapshot")
		}
	}
	if len(pkg.SkillFiles) > 0 {
		ref := pkg.Manifest.Skills
		ref = strings.TrimPrefix(ref, "./")
		if !fs.ValidPath(ref) || ref == "." || strings.Contains(ref, "\\") {
			return processDirs{}, fmt.Errorf("invalid Skills directory")
		}
		for name, data := range pkg.SkillFiles {
			if !fs.ValidPath(name) || strings.Contains(name, "\\") {
				return processDirs{}, fmt.Errorf("invalid Skills file")
			}
			target := filepath.Join(dirs.Plugin, ref, filepath.FromSlash(name))
			if err := privatePackageDirectory(dirs.Plugin, filepath.Dir(filepath.Join(ref, filepath.FromSlash(name)))); err != nil {
				return processDirs{}, err
			}
			if info, err := os.Lstat(target); err == nil && !info.Mode().IsRegular() {
				return processDirs{}, fmt.Errorf("Skills file is not regular")
			}
			if err := os.WriteFile(target, data, 0600); err != nil {
				return processDirs{}, err
			}
		}
	}
	return dirs, nil
}

func (s *Service) removeInstallationData(id string) error {
	root, err := s.installationPath(id)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("remove App installation data")
	}
	return nil
}

func privatePackageDirectory(root, relative string) error {
	if relative == "." {
		return nil
	}
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		root = filepath.Join(root, part)
		if err := privateDirectory(root); err != nil {
			return err
		}
	}
	return nil
}
