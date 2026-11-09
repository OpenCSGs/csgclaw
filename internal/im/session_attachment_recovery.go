package im

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type pendingSessionBatch struct {
	Workspace string               `json:"workspace"`
	Session   string               `json:"session"`
	Response  string               `json:"response"`
	Files     []pendingSessionFile `json:"files"`
}

type pendingSessionFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	SHA  string `json:"sha256"`
	Size int64  `json:"size"`
}

func canonicalSessionWorkspace(path string) (string, error) {
	if err := os.MkdirAll(path, 0700); err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}

func (store attachmentStore) lockFile(name string, exclusive bool) (*os.File, error) {
	if err := os.MkdirAll(string(store), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(string(store), name), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockSessionPublications(f, exclusive); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func (b pendingSessionBatch) relativeDir() string {
	return filepath.Join(".csgclaw", "attachments", "sessions", b.Session, b.Response)
}

func (b pendingSessionBatch) tempName(key string, index int) string {
	return fmt.Sprintf(".upload-%s-%d", key, index)
}

func (store attachmentStore) pendingPath(key string) string {
	return filepath.Join(string(store), "pending", key+".json")
}

func (store attachmentStore) writePending(key string, batch pendingSessionBatch) error {
	data, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	return atomicWriteFile(store.pendingPath(key), data, 0600)
}

func removeMissingOK(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Writers keep a shared OS lock. Recovery re-reads the directory under the
// exclusive lock, including when two Services open the same state at once.
func (store attachmentStore) recoverPendingBatches() error {
	dir := filepath.Join(string(store), "pending")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) || err == nil && len(entries) == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	lock, err := store.lockFile("publication.lock", true)
	if err != nil {
		return err
	}
	defer lock.Close()
	entries, err = os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") && strings.Contains(name, ".json.tmp-") {
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				return err
			}
			continue
		}
		key, ok := strings.CutSuffix(name, ".json")
		if !ok || !isAttachmentPathSegment(key) {
			continue
		}
		data, err := os.ReadFile(store.pendingPath(key))
		if err != nil {
			return err
		}
		var batch pendingSessionBatch
		if err := json.Unmarshal(data, &batch); err != nil {
			return err
		}
		if err := store.recoverSessionBatch(key, batch, nil); err != nil {
			return err
		}
	}
	return nil
}

func (store attachmentStore) recoverSessionBatch(key string, batch pendingSessionBatch, dir *os.Root) error {
	if !filepath.IsAbs(batch.Workspace) || !isAttachmentPathSegment(batch.Session) || !isAttachmentPathSegment(batch.Response) || len(batch.Files) == 0 {
		return fmt.Errorf("invalid pending session attachment batch %q", key)
	}
	for _, file := range batch.Files {
		if file.ID != "" && (!validAttachmentID(file.ID) || !isAttachmentPathSegment(file.Name) || !validSHA256(file.SHA) || file.Size < 0) {
			return fmt.Errorf("invalid pending session attachment in %q", key)
		}
	}
	committed := false
	if id := batch.Files[0].ID; id != "" {
		info, err := os.Stat(store.objectPath(id))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		committed = err == nil && info.Mode().IsRegular()
	}
	if dir == nil {
		var err error
		dir, err = openCanonicalSessionDir(batch.Workspace, batch.relativeDir(), false)
		if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, errWorkspaceDirectoryChanged) {
			return err
		}
		if dir != nil {
			defer dir.Close()
		}
	}
	if dir != nil {
		for i, file := range batch.Files {
			temp := batch.tempName(key, i)
			// A remaining temp has not yet been renamed to its public name.
			_, err := dir.Lstat(temp)
			if errors.Is(err, os.ErrNotExist) && !committed && file.ID != "" {
				if err := removeOriginalWorkspaceCopy(dir, file); err != nil {
					return err
				}
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := removeMissingOK(dir.Remove(temp)); err != nil {
				return err
			}
		}
	}
	if !committed {
		if err := store.removeIncompleteIndexes(batch.Files); err != nil {
			return err
		}
	}
	return removeMissingOK(os.Remove(store.pendingPath(key)))
}

// An interrupted atomic index write may leave either an index or its temporary
// file. Only names belonging to this unpublished batch are eligible for removal.
func (store attachmentStore) removeIncompleteIndexes(files []pendingSessionFile) error {
	dir := filepath.Join(string(store), assetObjectsDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return removeMissingOK(err)
	}
	ids := make(map[string]bool, len(files))
	for _, file := range files {
		if file.ID != "" {
			ids[file.ID] = true
		}
	}
	for _, entry := range entries {
		name := entry.Name()
		id, index := strings.CutSuffix(name, ".json")
		if !index {
			id, _, index = strings.Cut(strings.TrimPrefix(name, "."), ".json.tmp-")
		}
		if index && ids[id] && entry.Type().IsRegular() {
			if err := removeMissingOK(os.Remove(filepath.Join(dir, name))); err != nil {
				return err
			}
		}
	}
	return nil
}

func removeOriginalWorkspaceCopy(dir *os.Root, file pendingSessionFile) error {
	name := file.ID + "-" + file.Name
	before, err := dir.Lstat(name)
	if errors.Is(err, os.ErrNotExist) || err == nil && (!before.Mode().IsRegular() || before.Size() != file.Size) {
		return nil
	}
	if err != nil {
		return err
	}
	f, err := dir.OpenFile(name, os.O_RDONLY|sessionNonblockingRead, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) {
		f.Close()
		return err
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(f, file.Size+1))
	f.Close()
	if err != nil || n != file.Size || fmt.Sprintf("%x", hash.Sum(nil)) != file.SHA {
		return err // Keep workspace edits.
	}
	current, err := dir.Lstat(name)
	if errors.Is(err, os.ErrNotExist) || err == nil && (!os.SameFile(current, before) || !current.ModTime().Equal(before.ModTime())) {
		return nil
	}
	if err != nil {
		return err
	}
	return removeMissingOK(dir.Remove(name))
}
