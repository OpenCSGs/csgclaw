package turnjournal

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAttemptAndReplacementAreExclusive(t *testing.T) {
	for i := 0; i < 32; i++ {
		dir := t.TempDir()
		if err := Created(dir, "new"); err != nil {
			t.Fatal(err)
		}
		gate := make(chan struct{})
		replaced := make(chan bool, 1)
		sent := make(chan bool, 1)
		go func() {
			<-gate
			ok, err := ReplaceUnused(dir, "new")
			if err != nil {
				t.Error(err)
			}
			replaced <- ok
		}()
		go func() {
			<-gate
			prior, err := Begin(dir, "room", "new", "hello")
			if err != nil && !errors.Is(err, ErrUnconfirmed) {
				t.Error(err)
			}
			sent <- err == nil && !prior
		}()
		close(gate)
		if <-replaced == <-sent {
			t.Fatal("replacement and dispatch must have one owner")
		}
	}
}

func TestJournalProtectsUnknownBindingsAndUnreadableReceipts(t *testing.T) {
	dir := t.TempDir()
	if allowed, err := ReplaceUnused(dir, "legacy"); err != nil || allowed {
		t.Fatalf("legacy: %t %v", allowed, err)
	}
	if previous, err := Begin(dir, "one", "legacy", "key"); err != nil || previous {
		t.Fatal("could not claim first request")
	}
	if previous, err := Begin(dir, "one", "legacy", "key"); err != nil || !previous {
		t.Fatal("did not recognize retry")
	}
	if _, err := Begin(dir, "one", "different", "key"); !errors.Is(err, ErrUnconfirmed) {
		t.Fatalf("redirected to another thread: %v", err)
	}
	if previous, err := Begin(dir, "two", "another", "key"); err != nil || previous {
		t.Fatal("independent conversation inherited a receipt")
	}
	if err := Confirm(dir, "one", "key", "turn-a"); err != nil {
		t.Fatal(err)
	}
	if err := Confirm(dir, "one", "key", "turn-b"); !errors.Is(err, ErrUnconfirmed) {
		t.Fatal("replaced acknowledged turn")
	}
	if id, err := ConfirmedTurn(dir, "one", "key"); err != nil || id != "turn-a" {
		t.Fatalf("ack=%q err=%v", id, err)
	}
	if err := os.WriteFile(messagePath(dir, "one", "partial"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Begin(dir, "one", "legacy", "partial"); !errors.Is(err, ErrUnconfirmed) {
		t.Fatalf("partial receipt allowed a retry: %v", err)
	}
	if err := os.MkdirAll(native(dir, "partial"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(native(dir, "partial"), "created"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if ok, err := ReplaceUnused(dir, "partial"); err != nil || ok {
		t.Fatal("incomplete creation proof allowed replacement")
	}
}
