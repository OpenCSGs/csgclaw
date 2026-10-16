//go:build !windows

package codex

import (
	"csgclaw/internal/processgroup"
	"syscall"
)

func processAlivePID(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func stopProcessTree(pid int) error { return processgroup.Stop(pid, true) }
