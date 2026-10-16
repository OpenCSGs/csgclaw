//go:build windows

package connectorcli

import (
	"csgclaw/internal/processgroup"
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

func configureCommand(*exec.Cmd) {}
func processStarted(pid int) string {
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "(Get-Process -Id "+strconv.Itoa(pid)+" -ErrorAction Stop).StartTime.ToUniversalTime().Ticks").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
func stopCommand(pid int) error { return processgroup.Stop(pid, true) }
func RecoverProcess(ref ProcessRef) error {
	if ref.PID <= 0 {
		return nil
	}
	actual := processStarted(ref.PID)
	if actual == "" {
		return nil
	}
	if ref.Started == "" || actual != ref.Started {
		return errors.New("CLI process identity could not be confirmed")
	}
	return stopCommand(ref.PID)
}
