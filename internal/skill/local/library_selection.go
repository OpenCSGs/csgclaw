package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"

	"csgclaw/internal/skill/discovery"
	"csgclaw/internal/skill/installstate"
	"csgclaw/internal/skill/system"
)

// LibraryBundle is the effective source and its provenance resolved together
// while the caller holds the global installation lock.
type LibraryBundle struct {
	Tree   fs.FS
	Origin installstate.Record
}

func ResolveLibraryBundle(root, name string, record installstate.Record) (LibraryBundle, error) {
	if clean, err := NormalizeName(name); err != nil || clean != name {
		return LibraryBundle{}, ErrSkillInvalid
	}
	found, err := Discovered(root, name)
	if err != nil {
		return LibraryBundle{}, err
	}
	if found != nil {
		return LibraryBundle{discovery.BoundedFS(found.Path), installstate.Record{Source: installstate.SourceDiscovered, SourceURL: found.Path, Version: found.Version}}, nil
	}
	dir, err := resolveInstalledDir(root, name)
	if err == nil {
		return LibraryBundle{discovery.BoundedFS(dir), record}, nil
	}
	if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, ErrSkillInvalid) {
		return LibraryBundle{}, err
	}
	if !system.IsName(name) {
		return LibraryBundle{}, err
	}
	source, err := system.ResolveSource(name)
	if err != nil {
		return LibraryBundle{}, err
	}
	tree, err := fs.Sub(source.FS, source.RootPath)
	return LibraryBundle{tree, installstate.Record{Source: installstate.SourceBuiltin}}, err
}

func (b LibraryBundle) Metadata() (description, version string, err error) {
	data, err := fs.ReadFile(b.Tree, skillFileName)
	if err != nil {
		return "", "", err
	}
	version = b.Origin.Version
	if front, ok := extractFrontmatter(data); ok {
		m, err := parseSkillMetadata(front)
		if err != nil {
			return "", "", err
		}
		description = m.Description
		if version == "" {
			version = m.Version
		}
	}
	return description, version, nil
}

// Selection can also be computed over the private staged copy. This checks
// the exact bytes to be installed, even for an externally edited discovery.
func (b LibraryBundle) Selection(ctx context.Context, name string) (string, error) {
	files, err := installstate.TreeFingerprint(ctx, b.Tree)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(struct {
		Name, Files string
		Origin      installstate.Record
	}{name, files, b.Origin})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
