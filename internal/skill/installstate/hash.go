package installstate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

func BundleSHA256(root string) (string, error) {
	hasher := sha256.New()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if _, err := hasher.Write([]byte(rel)); err != nil {
			return err
		}
		if _, err := hasher.Write([]byte{0}); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := hasher.Write(data); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("hash skill bundle: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
