package installstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

var ErrInstallationChanged = errors.New("skill_installation_changed")

// WithSelection holds the same lock as installers while consuming a selected
// installed copy. Do not call another installstate transaction in consume.
func WithSelection(root, name, expected string, consume func() error) error {
	return ReadSnapshot(root, func(records map[string]Record) error {
		actual, err := snapshotToken(root, name, records[name])
		if err != nil {
			return err
		}
		if actual != expected {
			return ErrInstallationChanged
		}
		return consume()
	})
}

// Snapshot identifies the exact local files and installation record reviewed by a user.
func Snapshot(root, name string) (Record, string, error) {
	var record Record
	var token string
	err := ReadSnapshot(root, func(records map[string]Record) error {
		record = records[name]
		var err error
		token, err = snapshotToken(root, name, record)
		return err
	})
	return record, token, err
}

func snapshotToken(root, name string, record Record) (string, error) {
	if name == "" || name == "." || name == ".." || name != filepath.Base(name) {
		return "", fmt.Errorf("invalid skill name")
	}
	dir := filepath.Join(root, name)
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("skill directory must not be a symbolic link")
	}
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported skill file: %s", entry.Name())
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	digest, err := BundleSHA256(dir)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append(append(data, 0), digest...))
	return hex.EncodeToString(sum[:]), nil
}
