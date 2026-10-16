//go:build !windows

package connectorcli

import (
	"csgclaw/internal/processgroup"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func configureCommand(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func processStarted(pid int) string {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "lstart=").Output()
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
		return stopCommand(ref.PID)
	}
	if ref.Started == "" || actual != ref.Started {
		return errors.New("CLI process identity could not be confirmed")
	}
	if err := stopCommand(ref.PID); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if processStarted(ref.PID) == "" {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("CLI process has not exited")
}
