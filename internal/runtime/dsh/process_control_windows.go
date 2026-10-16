//go:build windows

package dsh

import (
	"csgclaw/internal/processgroup"
	"os/exec"
)

func configureProcessGroup(*exec.Cmd) {}

func terminateProcessTree(pid int, force bool) error { return processgroup.Stop(pid, force) }
