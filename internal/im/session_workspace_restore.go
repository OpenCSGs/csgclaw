package im

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/utils/filebrowse"
)

// Only SessionAttachmentFile creates this source, after checking ownership.
type sessionWorkspaceSource struct {
	path, relative, sha string
	size                int64
}

// RestoreSessionWorkspaceFile keeps an existing work file or restores the
// authorized original in the agent's current workspace when it is missing.
func RestoreSessionWorkspaceFile(ctx context.Context, workspaceRoot string, file AttachmentFile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	source := file.sessionWorkspace
	if source == nil {
		return ErrSessionAttachmentNotFound
	}
	dir, err := openSessionWorkspaceDir(workspaceRoot, filepath.Dir(source.relative))
	if err != nil {
		return err
	}
	defer dir.Close()
	name := filepath.Base(source.relative)
	if err := sessionWorkspaceRegularFile(dir, name); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	snapshot, err := filebrowse.OpenFile(filepath.Dir(source.path), filepath.Base(source.path))
	if err != nil {
		return err
	}
	defer snapshot.Close()
	return restoreMissingSessionFile(ctx, dir, name, snapshot, source.size, source.sha)
}

func restoreMissingSessionFile(ctx context.Context, dir *os.Root, name string, snapshot io.Reader, size int64, checksum string) error {
	hash := sha256.New()
	limited := &io.LimitedReader{R: attachmentReader(ctx, snapshot), N: size + 1}
	temp, err := stageSessionWorkspaceFile(dir, io.TeeReader(limited, hash))
	if err != nil {
		return err
	}
	defer dir.Remove(temp)
	if err := ctx.Err(); err != nil {
		return err
	}
	if limited.N != 1 || fmt.Sprintf("%x", hash.Sum(nil)) != checksum {
		return fmt.Errorf("session attachment snapshot checksum mismatch")
	}
	err = dir.Link(temp, name)
	if err == nil || errors.Is(err, os.ErrExist) {
		return sessionWorkspaceRegularFile(dir, name)
	}
	return err
}

// The configured workspace is the authority boundary. Pin each directory
// inside it before opening the next component or publishing a file.
func openSessionWorkspaceDir(workspaceRoot, relativeDir string) (*os.Root, error) {
	if !filepath.IsAbs(workspaceRoot) {
		return nil, fmt.Errorf("an absolute session workspace is required")
	}
	if err := os.MkdirAll(workspaceRoot, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(workspaceRoot)
	if err != nil {
		return nil, err
	}
	for _, name := range strings.Split(relativeDir, string(filepath.Separator)) {
		child, err := sessionWorkspaceChild(root, name)
		root.Close()
		if err != nil {
			return nil, err
		}
		root = child
	}
	return root, nil
}

func sessionWorkspaceChild(parent *os.Root, name string) (*os.Root, error) {
	if err := parent.Mkdir(name, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	before, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.IsDir() {
		return nil, fmt.Errorf("session attachment directory %q must be a directory", name)
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	after, err := child.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		child.Close()
		return nil, fmt.Errorf("session attachment directory %q changed while opening", name)
	}
	return child, nil
}

func sessionWorkspaceRegularFile(dir *os.Root, name string) error {
	info, err := dir.Lstat(name)
	if err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("session attachment %q must be a regular file", name)
	}
	return err
}

type cancellableAttachmentReader struct {
	ctx context.Context
	io.Reader
}

func attachmentReader(ctx context.Context, reader io.Reader) io.Reader {
	if ctx.Done() == nil {
		return reader
	}
	return cancellableAttachmentReader{ctx, reader}
}

func (r cancellableAttachmentReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(b)
}
