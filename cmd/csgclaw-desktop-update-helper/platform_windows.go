//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const waitTimeoutResult = 0x00000102

type windowsParentProcess struct {
	handle windows.Handle
}

func platformDependencies() coordinatorDependencies {
	return coordinatorDependencies{
		openParent:      openWindowsParentProcess,
		runInstaller:    runWindowsInstaller,
		startExecutable: startWindowsExecutable,
		sleep:           time.Sleep,
	}
}

func openWindowsParentProcess(pid uint32) (parentProcess, error) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return nil, fmt.Errorf("open parent process %d: %w", pid, err)
	}
	return &windowsParentProcess{handle: handle}, nil
}

func (process *windowsParentProcess) Wait(timeout time.Duration) error {
	waitMilliseconds := uint32(timeout / time.Millisecond)
	result, err := windows.WaitForSingleObject(process.handle, waitMilliseconds)
	if err != nil {
		return err
	}
	switch result {
	case windows.WAIT_OBJECT_0:
		return nil
	case waitTimeoutResult:
		return errParentWaitTimeout
	default:
		return fmt.Errorf("unexpected parent wait result %#x", result)
	}
}

func (process *windowsParentProcess) Close() error {
	return windows.CloseHandle(process.handle)
}

func runWindowsInstaller(path string, output io.Writer) (int, error) {
	command := exec.Command(path, "--silent")
	command.Dir = filepath.Dir(path)
	command.Stdout = output
	command.Stderr = output
	command.SysProcAttr = hiddenWindowsProcessAttributes()
	err := command.Run()
	if err == nil {
		return 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode(), err
	}
	return -1, err
}

func startWindowsExecutable(path string) error {
	command := exec.Command(path)
	command.Dir = filepath.Dir(path)
	command.SysProcAttr = hiddenWindowsProcessAttributes()
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}

func hiddenWindowsProcessAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW,
		HideWindow:    true,
	}
}
