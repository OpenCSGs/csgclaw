package relay

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// InstanceRoot separates server installations sharing a home directory by their
// local API address (including port). It must not use a PID or random startup ID.
func InstanceRoot(root, localURL string) string {
	return filepath.Join(root, fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimRight(localURL, "/")))))
}

// StableClientID is shared by the server and diagnostic CLI for the same slot.
func StableClientID(root string) (string, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	lock, err := acquireStateLock(filepath.Join(root, "client-id.lock"))
	if err != nil {
		return "", err
	}
	defer lock.Close()
	path := filepath.Join(root, "client-id")
	data, err := os.ReadFile(path)
	if err == nil {
		id := strings.TrimSpace(string(data))
		if _, err = uuid.Parse(id); err != nil {
			return "", fmt.Errorf("invalid relay client ID file")
		}
		return id, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	id := uuid.NewString()
	if err = os.WriteFile(path, []byte(id+"\n"), 0600); err != nil {
		return "", err
	}
	return id, nil
}
