//go:build windows

package connectorcli

import (
	"csgclaw/internal/processgroup"
	"errors"
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}
func processStarted(pid int) string {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
		return ""
	}
	// FILETIME uses 1601 as its epoch; the persisted .NET ticks use year 1.
	const epochOffset = uint64(504911232000000000)
	ticks := uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	return strconv.FormatUint(ticks+epochOffset, 10)
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
