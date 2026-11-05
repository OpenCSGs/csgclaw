package im

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func sessionFile(object attachmentObject) (AttachmentFile, error) {
	if !filepath.IsAbs(object.WorkspaceRoot) || !object.validSessionWorkspacePath() {
		return AttachmentFile{}, errAttachmentNotFound
	}
	rel := filepath.FromSlash(object.WorkspacePath)
	return AttachmentFile{
		Attachment:    attachmentFromObject(object),
		Path:          filepath.Join(object.WorkspaceRoot, rel),
		workspaceRoot: object.WorkspaceRoot, workspacePath: rel,
		SafeName: object.SafeName, MediaType: object.MediaType, DownloadToken: object.DownloadToken,
	}, nil
}

func (f AttachmentFile) IsWorkspaceFile() bool { return f.workspaceRoot != "" }

// WorkspaceMatches ensures the relative path given to the worker names the
// same file that the attachment was published from.
func (f AttachmentFile) WorkspaceMatches(root string) bool {
	canonical, err := filepath.EvalSymlinks(root)
	return err == nil && f.IsWorkspaceFile() && canonical == f.workspaceRoot
}

// Open pins the regular file before exposing bytes through a download URL.
// The stored root is canonical. Verify its ancestors as well as the relative
// path, because an earlier workspace directory could have been replaced.
func (f AttachmentFile) Open() (*os.File, error) {
	if !f.IsWorkspaceFile() {
		return os.Open(f.Path)
	}
	dir, err := openCanonicalSessionDir(f.workspaceRoot, filepath.Dir(f.workspacePath), false)
	if errors.Is(err, errWorkspaceDirectoryChanged) || errors.Is(err, os.ErrNotExist) {
		return nil, errAttachmentNotFound
	}
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	name := filepath.Base(f.workspacePath)
	before, err := dir.Lstat(name)
	if errors.Is(err, os.ErrNotExist) || err == nil && !before.Mode().IsRegular() {
		return nil, errAttachmentNotFound
	}
	if err != nil {
		return nil, err
	}
	opened, err := dir.OpenFile(name, os.O_RDONLY|sessionNonblockingRead, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errAttachmentNotFound
	}
	if err != nil {
		return nil, err
	}
	info, err := opened.Stat()
	if err != nil || !os.SameFile(before, info) {
		opened.Close()
		if err != nil {
			return nil, err
		}
		return nil, errAttachmentNotFound
	}
	return opened, nil
}

// WorkspaceMediaType identifies the current representation without consuming
// the file offset used for ranges or previews.
func (f AttachmentFile) WorkspaceMediaType(opened *os.File) (string, error) {
	var header [512]byte
	n, err := opened.ReadAt(header[:], 0)
	if err != nil && err != io.EOF {
		return "", err
	}
	detected := strings.SplitN(http.DetectContentType(header[:n]), ";", 2)[0]
	if bytes.Contains(header[:n], []byte("svg")) {
		dec := xml.NewDecoder(bytes.NewReader(header[:n]))
		for {
			token, parseErr := dec.Token()
			if parseErr != nil {
				break
			}
			if start, ok := token.(xml.StartElement); ok {
				if start.Name.Local == "svg" && (start.Name.Space == "" || start.Name.Space == "http://www.w3.org/2000/svg") {
					return "image/svg+xml", nil
				}
				break
			}
		}
	}
	if n == 0 {
		return "application/octet-stream", nil
	}
	return detected, nil
}

func (f *AttachmentFile) refreshWorkspaceAttributes() error {
	handle, err := f.Open()
	if err != nil {
		return err
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		return err
	}
	mediaType, err := f.WorkspaceMediaType(handle)
	if err != nil {
		return err
	}
	att := &f.Attachment
	att.SizeBytes, att.MediaType, att.Kind = info.Size(), mediaType, attachmentKindForMediaType(mediaType)
	att.Width, att.Height, att.PreviewURL = 0, 0, ""
	if att.Kind == attachmentKindImage {
		att.PreviewURL = att.DownloadURL
		if config, _, err := image.DecodeConfig(io.LimitReader(handle, 64<<10)); err == nil {
			att.Width, att.Height = config.Width, config.Height
		}
	}
	return nil
}

var errWorkspaceDirectoryChanged = errors.New("session workspace directory changed")

// Pin each ancestor of the canonical workspace. Only managed directories
// below the workspace may be created during publication.
func openCanonicalSessionDir(workspace, relative string, create bool) (*os.Root, error) {
	if !filepath.IsAbs(workspace) || !filepath.IsLocal(relative) {
		return nil, fmt.Errorf("invalid session workspace path")
	}
	volume := filepath.VolumeName(workspace) + string(filepath.Separator)
	ancestors, err := filepath.Rel(volume, workspace)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(volume)
	if err != nil {
		return nil, err
	}
	for i, section := range []string{ancestors, relative} {
		if section == "." {
			continue
		}
		for _, name := range strings.Split(section, string(filepath.Separator)) {
			child, err := sessionWorkspaceChild(root, name, create && i == 1)
			root.Close()
			if err != nil {
				return nil, err
			}
			root = child
		}
	}
	return root, nil
}

func sessionWorkspaceChild(parent *os.Root, name string, create bool) (*os.Root, error) {
	if create {
		if err := parent.Mkdir(name, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	before, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.IsDir() {
		return nil, fmt.Errorf("%w: %q must be a directory", errWorkspaceDirectoryChanged, name)
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	after, err := child.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		child.Close()
		return nil, fmt.Errorf("%w: %q", errWorkspaceDirectoryChanged, name)
	}
	return child, nil
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
