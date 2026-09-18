//go:build windows

package dsh

import (
	"os/exec"
	"strconv"
)

func configureProcessGroup(*exec.Cmd) {}

func terminateProcessTree(pid int, force bool) error {
	if pid <= 0 {
		return nil
	}
	args := []string{"/PID", strconv.Itoa(pid), "/T"}
	if force {
		args = append(args, "/F")
	}
	return exec.Command("taskkill", args...).Run()
}
