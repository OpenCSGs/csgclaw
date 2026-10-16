//go:build !windows

package dsh

import (
	"csgclaw/internal/processgroup"
	"os/exec"
	"syscall"
)

func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateProcessTree(pid int, force bool) error { return processgroup.Stop(pid, force) }
