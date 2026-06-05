package hostuser

import (
	"fmt"
	"os"
)

// RunUser returns the current process uid:gid for sandbox run --user alignment
// with bind-mounted host directories.
func RunUser() (string, error) {
	uid := os.Getuid()
	gid := os.Getgid()
	if uid < 0 || gid < 0 {
		return "", fmt.Errorf("host uid/gid is unavailable on this platform")
	}
	return fmt.Sprintf("%d:%d", uid, gid), nil
}
