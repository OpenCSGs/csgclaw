// Package turnjournal records native session creation and dispatch intent before
// the corresponding binding or RPC becomes visible. Request records survive restart.
package turnjournal

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const Directory = "turn-dispatch"

var ErrUnconfirmed = errors.New("native message delivery needs confirmation")

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// claim never replaces a prior decision. If the process exits during a write,
// the incomplete record prevents a later sender from assuming it owns the RPC.
func claim(path, value string) (bool, string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return false, "", err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, os.ErrExist) {
		data, err := os.ReadFile(path)
		return false, string(data), err
	}
	if err != nil {
		return false, "", err
	}
	_, err = f.WriteString(value)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return true, value, err
}

func messagePath(root, key, id string) string {
	return filepath.Join(root, Directory, "messages", digest(key+"\x00"+id))
}

// AttemptedThread returns the native binding selected before dispatch.
func AttemptedThread(root, key, id string) (string, error) {
	if id == "" {
		return "", nil
	}
	if root == "" {
		return "", errors.New("runtime directory is required")
	}
	data, err := os.ReadFile(messagePath(root, key, id))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err == nil && len(data) == 0 {
		return "", fmt.Errorf("%w: incomplete dispatch record", ErrUnconfirmed)
	}
	return string(data), err
}

func native(root, id string) string {
	return filepath.Join(root, Directory, "threads", digest(id))
}

// Created must succeed before publishing the ID returned by thread/start or session/new.
func Created(root, id string) error {
	if root == "" {
		return errors.New("runtime directory is required")
	}
	_, _, err := claim(filepath.Join(native(root, id), "created"), "new")
	return err
}

// ReplaceUnused and Begin compete for the same immutable first action.
func ReplaceUnused(root, id string) (bool, error) {
	if root == "" {
		return false, errors.New("runtime directory is required")
	}
	data, err := os.ReadFile(filepath.Join(native(root, id), "created"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || string(data) != "new" {
		return false, err
	}
	_, action, err := claim(filepath.Join(native(root, id), "first"), "replace")
	return action == "replace", err
}

// Begin returns true for a previously attempted message. A crash after intent
// is recorded but before the RPC reaches the peer is deliberately ambiguous.
func Begin(root, key, thread, message string) (previous bool, err error) {
	if root == "" {
		return false, errors.New("runtime directory is required")
	}
	_, action, err := claim(filepath.Join(native(root, thread), "first"), "send")
	if err != nil {
		return false, err
	}
	if action != "send" {
		return false, fmt.Errorf("%w: session %q was retired or its first action is incomplete", ErrUnconfirmed, thread)
	}
	if message == "" {
		return false, nil
	}
	created, owner, err := claim(messagePath(root, key, message), thread)
	if err != nil {
		return false, err
	}
	if owner != thread {
		return false, fmt.Errorf("%w: message %q belongs to a different or incomplete session binding", ErrUnconfirmed, message)
	}
	return !created, nil
}

// Confirm records an actual native turn/start acknowledgement, independently
// of the HTTP response. It never turns a missing acknowledgement into a retry.
func Confirm(root, key, message, turn string) error {
	if message == "" || turn == "" {
		return nil
	}
	_, value, err := claim(messagePath(root, key, message)+".turn", turn)
	if err == nil && value != turn {
		return fmt.Errorf("%w: conflicting native turn IDs for %q", ErrUnconfirmed, message)
	}
	return err
}
func ConfirmedTurn(root, key, message string) (string, error) {
	data, err := os.ReadFile(messagePath(root, key, message) + ".turn")
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(data), err
}
