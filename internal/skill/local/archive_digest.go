package local

import (
	"csgclaw/internal/skill/installstate"
	"fmt"
	"os"
)

// ArchiveDigest uses the same normalized file layout and digest as installation.
// It only extracts to a temporary directory and never alters an installed skill.
func ArchiveDigest(name, filename string, archive []byte) (string, error) {
	if _, err := NormalizeName(name); err != nil {
		return "", err
	}
	if int64(len(archive)) > maxArchiveBytes {
		return "", ErrSkillArchiveInvalid
	}
	dir, err := os.MkdirTemp("", "csgclaw-skill-digest-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	if err := extractArchive(archive, dir); err != nil {
		return "", err
	}
	root, extractedName, err := resolveExtractedSkillDir(dir, filename)
	if err != nil {
		return "", err
	}
	if extractedName != name {
		return "", fmt.Errorf("%w: archive skill name changed", ErrSkillArchiveInvalid)
	}
	return installstate.BundleSHA256(root)
}
