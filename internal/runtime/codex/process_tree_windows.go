//go:build windows

package codex

import (
	"os/exec"
	"strconv"
)

func stopProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	err := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
	if err != nil && !processAlivePID(pid) {
		return nil
	}
	return err
}
