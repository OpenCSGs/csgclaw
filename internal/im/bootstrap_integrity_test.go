package im

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBootstrapSaveReplacesWithoutTruncatingPreviousFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := DefaultBootstrap()
	if err := SaveBootstrap(path, state); err != nil {
		t.Fatal(err)
	}
	previous, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the previous inode accessible to detect an in-place overwrite.
	link := path + ".previous"
	if err := os.Link(path, link); err != nil {
		t.Skipf("filesystem does not support hard links: %v", err)
	}
	state.Users = append(state.Users, User{ID: "user-worker", Name: "worker", Role: "worker"})
	if err := SaveBootstrap(path, state); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(link)
	if err != nil || !bytes.Equal(got, previous) {
		t.Fatalf("previous file overwritten: %q, %v", got, err)
	}
	reloaded, err := LoadBootstrap(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, user := range reloaded.Users {
		found = found || user.ID == "user-worker"
	}
	if !found {
		t.Fatal("replacement did not persist worker")
	}
}

func TestBootstrapInterruptedWritePreservesCommittedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := SaveBootstrap(path, DefaultBootstrap()); err != nil {
		t.Fatal(err)
	}
	previous, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err = atomicWriteReader(path, io.MultiReader(strings.NewReader(`{"users":`), failingBootstrapReader{}), 0o600)
	if err == nil {
		t.Fatal("interrupted write succeeded")
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, previous) {
		t.Fatalf("committed state changed after failed write: %q, %v", got, err)
	}
	if _, err := LoadBootstrap(path); err != nil {
		t.Fatal(err)
	}
	temps, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".state.json.tmp-*"))
	if err != nil || len(temps) != 0 {
		t.Fatalf("temporary files remain: %v, %v", temps, err)
	}
}

type failingBootstrapReader struct{}

func (failingBootstrapReader) Read([]byte) (int, error) {
	return 0, errors.New("simulated write interruption")
}

func TestBootstrapCorruptionPreservesDataAndExplainsRecovery(t *testing.T) {
	for _, corrupt := range []string{"\x00\x00", "", `{"users":`} {
		t.Run(corrupt, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			if err := os.WriteFile(path, []byte(corrupt), 0o600); err != nil {
				t.Fatal(err)
			}
			err := EnsureBootstrapState(path)
			if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "do not delete") {
				t.Fatalf("expected actionable error, got %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != corrupt {
				t.Fatalf("corrupted file changed: %q, %v", got, err)
			}
		})
	}
}

func TestMissingBootstrapDoesNotEraseConversationData(t *testing.T) {
	for _, relative := range []string{"sessions/room-kept.jsonl", "sessions/blobs/room-kept/msg-kept.json", "threads/room-kept/msg-kept.json", "assets/objects/asset-kept.json"} {
		t.Run(relative, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "state.json")
			artifact := filepath.Join(dir, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(artifact), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(artifact, []byte("preserve me"), 0o600); err != nil {
				t.Fatal(err)
			}
			err := EnsureBootstrapState(path)
			if err == nil || !strings.Contains(err.Error(), "conversation data remains") {
				t.Fatalf("expected refusal to reset missing state, got %v", err)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("state recreated: %v", err)
			}
			got, err := os.ReadFile(artifact)
			if err != nil || string(got) != "preserve me" {
				t.Fatalf("conversation data changed: %q, %v", got, err)
			}
		})
	}
}

func TestMissingBootstrapAllowsEmptyDirectories(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"sessions/blobs", "threads", "assets/blobs/sha256", "assets/objects"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := EnsureBootstrapState(filepath.Join(dir, "state.json")); err != nil {
		t.Fatal(err)
	}
}
