package installstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
)

// TreeFingerprint binds a selection to relative paths, file contents and
// executable bits. Include empty directories and reject links/special files.
// Per-file hashes frame arbitrary file contents unambiguously.
func TreeFingerprint(ctx context.Context, tree fs.FS) (string, error) {
	h := sha256.New()
	err := fs.WalkDir(tree, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			fmt.Fprintf(h, "d%s\x00", path)
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported skill file: %s", path)
		}
		f, err := tree.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported skill file: %s", path)
		}
		content := sha256.New()
		if _, err := io.Copy(content, f); err != nil {
			return err
		}
		fmt.Fprintf(h, "f%s\x00%03o%x", path, info.Mode().Perm()&0111, content.Sum(nil))
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
