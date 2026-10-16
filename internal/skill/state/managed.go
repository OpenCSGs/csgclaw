package state

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type ManagedSkill struct {
	Name    string `json:"skill_name"`
	Version string `json:"bundle_version"`
	Digest  string `json:"content_digest"`
	Managed bool   `json:"managed"`
}
type SkillPlan struct {
	Before []ManagedSkill `json:"before"`
	After  []ManagedSkill `json:"after"`
	Bundle string         `json:"bundle"`
}

func DirectoryDigest(root string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("skill symlinks are unsupported")
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("skill contains a non-regular file")
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		_, _ = io.WriteString(hash, filepath.ToSlash(rel)+"\x00")
		if err := binary.Write(hash, binary.LittleEndian, info.Size()); err != nil {
			return err
		}
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
