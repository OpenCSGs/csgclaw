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

// StableClientID restores the identity of an existing slot. Platform sandboxes
// can also recover a newly provisioned identity after losing their local state.
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
	id := newClientID()
	if err = os.WriteFile(path, []byte(id+"\n"), 0600); err != nil {
		return "", err
	}
	return id, nil
}

func newClientID() string {
	sandboxType := strings.TrimSpace(os.Getenv("OPENCSG_SANDBOX_TYPE"))
	instanceID := strings.TrimSpace(os.Getenv("CSGCLAW_NAME"))
	if instanceID != "" && (sandboxType == "csgclaw" || sandboxType == "csgclaw-workspace") {
		name := "https://opencsg.com/csgclaw/relay-client/v1\x00" + sandboxType + "\x00" + instanceID
		return uuid.NewSHA1(uuid.NameSpaceURL, []byte(name)).String()
	}
	return uuid.NewString()
}
