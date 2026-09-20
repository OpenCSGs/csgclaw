package apps

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	for filename, value := range map[string]any{"plugin.json": pkg.Manifest, "apps.json": map[string]any{"apps": pkg.Apps}} {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return processDirs{}, fmt.Errorf("encode App package snapshot")
		}
		data = append(data, '\n')
		file := filepath.Join(dirs.Plugin, filename)
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
