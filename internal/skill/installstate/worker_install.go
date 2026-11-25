package installstate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Replace authorizes a same-origin reinstall. Confirmation is additionally
// required for changing origins and binds both the installed copy and the new
// source. All checks and publication share the cross-process install lock.
type WorkerInstall struct {
	Replace      bool
	Confirmation string
}

type SourceConflict struct {
	Name, Installed, Requested, Confirmation string
}

func (e *SourceConflict) Error() string {
	return fmt.Sprintf("skill name conflict %q: installed source %q, requested source %q. Ask the user whether to overwrite. Only after confirmation retry with replace and confirmation %s", e.Name, e.Installed, e.Requested, e.Confirmation)
}

func (r Record) sameOrigin(other Record) bool {
	if r.SourceKind() != other.SourceKind() {
		return false
	}
	if r.SourceKind() == SourceBuiltin {
		return true
	}
	if r.SourceURL == "" || r.SourceURL != other.SourceURL {
		return false
	}
	if r.RemotePath == other.RemotePath {
		return true
	}
	// Older Hub installs recorded the full skill page, but not its repository
	// key. Only fill that gap when the identical page corroborates the known key.
	if r.SourceKind() != SourceMarket || (r.RemotePath != "" && other.RemotePath != "") {
		return false
	}
	key := r.RemotePath
	if key == "" {
		key = other.RemotePath
	}
	owner, name, ok := strings.Cut(key, "/")
	if !ok || !fs.ValidPath(key) || strings.Contains(name, "/") || strings.ContainsAny(key, `\%?#`) {
		return false
	}
	u, err := url.Parse(r.SourceURL)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" &&
		u.User == nil && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" &&
		strings.HasSuffix(u.EscapedPath(), "/skills/"+url.PathEscape(owner)+"/"+url.PathEscape(name))
}

func (r Record) originLabel() string {
	if u := r.DisplayURL(); u != "" {
		return u
	}
	if filepath.IsAbs(r.SourceURL) {
		return r.SourceKind() + ": " + r.SourceURL
	}
	return r.SourceKind()
}

func (w WorkerInstall) Commit(root, name, stage string, next *Record, apply func() error) error {
	unchanged := errors.New("already installed")
	err := updateChecked(root, map[string]*Record{name: next}, nil, func(records map[string]Record) error {
		info, err := os.Lstat(filepath.Join(root, name))
		if errors.Is(err, os.ErrNotExist) {
			if w.Confirmation != "" {
				return ErrInstallationChanged
			}
			return nil
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("invalid installed skill directory %q", name)
		}
		old := records[name]
		sameOrigin := old.sameOrigin(*next)
		if sameOrigin && w.Replace && w.Confirmation == "" {
			return nil
		}
		current, err := os.OpenRoot(filepath.Join(root, name))
		if err != nil {
			return err
		}
		defer current.Close()
		actual, err := TreeFingerprint(context.Background(), current.FS())
		if err != nil {
			return err
		}
		incoming, err := TreeFingerprint(context.Background(), os.DirFS(stage))
		if err != nil {
			return err
		}
		if !sameOrigin {
			// The incoming record gets fresh timestamps on each registry download;
			// bind its stable identity and the actual staged files instead.
			data, err := json.Marshal([]any{name, old, actual, next.SourceKind(), next.SourceURL, next.RemotePath, next.Ref, next.Version, incoming})
			if err != nil {
				return err
			}
			confirmation := fmt.Sprintf("%x", sha256.Sum256(data))
			if !w.Replace || w.Confirmation != confirmation {
				return &SourceConflict{name, old.originLabel(), next.originLabel(), confirmation}
			}
			return nil
		}
		if w.Confirmation != "" {
			return ErrInstallationChanged
		}
		// Reuse an identical bundle; the agent still runs the enable step.
		if actual == incoming && (next.Version == "" || next.Version == old.Version) {
			return unchanged
		}
		return fmt.Errorf("skill %q already installed from the same source: enable the installed copy, or replace for a reinstall", name)
	}, apply)
	if errors.Is(err, unchanged) {
		return nil
	}
	return err
}
