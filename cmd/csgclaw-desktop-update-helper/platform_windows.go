//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	waitTimeoutResult             = 0x00000102
	applicationWindowWaitTimeout  = 15 * time.Second
	applicationWindowPollInterval = 250 * time.Millisecond
	flashWindowAll                = 0x00000003
	flashWindowUntilForeground    = 0x0000000C
	setWindowPositionNoSize       = 0x0001
	setWindowPositionNoMove       = 0x0002
	setWindowPositionShowWindow   = 0x0040
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	isIconicProcedure            = user32.NewProc("IsIconic")
	showWindowAsyncProcedure     = user32.NewProc("ShowWindowAsync")
	bringWindowToTopProcedure    = user32.NewProc("BringWindowToTop")
	setForegroundWindowProcedure = user32.NewProc("SetForegroundWindow")
	setWindowPositionProcedure   = user32.NewProc("SetWindowPos")
	flashWindowExProcedure       = user32.NewProc("FlashWindowEx")
)

type windowsApplicationWindow struct {
	handle    windows.HWND
	processID uint32
}

type windowsApplicationWindowFinder struct {
	installRoot    string
	executableName string
	callback       uintptr
	found          windowsApplicationWindow
}

type flashWindowInfo struct {
	size      uint32
	window    windows.HWND
	flags     uint32
	count     uint32
	timeoutMS uint32
}

type windowsParentProcess struct {
	handle windows.Handle
}

func platformDependencies() coordinatorDependencies {
	return coordinatorDependencies{
		openParent:          openWindowsParentProcess,
		runInstaller:        runWindowsInstaller,
		relaunchApplication: relaunchWindowsApplication,
		sleep:               time.Sleep,
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
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}

func relaunchWindowsApplication(path string) (relaunchResult, error) {
	finder := newWindowsApplicationWindowFinder(path)
	result := relaunchResult{}
	window, err := finder.Find()
	if err != nil {
		result.windowLookupError = fmt.Sprintf("find existing application window: %v", err)
	} else if window.handle != 0 {
		activated := activateWindowsApplicationWindow(window)
		activated.foundExistingWindow = true
		return activated, nil
	}

	result.startedApplication = true
	if err := startWindowsExecutable(path); err != nil {
		return result, err
	}

	deadline := time.Now().Add(applicationWindowWaitTimeout)
	for {
		window, err = finder.Find()
		if err != nil {
			result.windowLookupError = fmt.Sprintf(
				"find relaunched application window: %v",
				err,
			)
			return result, nil
		}
		if window.handle != 0 {
			activated := activateWindowsApplicationWindow(window)
			activated.startedApplication = true
			return activated, nil
		}
		if !time.Now().Before(deadline) {
			return result, nil
		}
		time.Sleep(applicationWindowPollInterval)
	}
}

func newWindowsApplicationWindowFinder(
	rootExecutablePath string,
) *windowsApplicationWindowFinder {
	finder := &windowsApplicationWindowFinder{
		installRoot:    filepath.Clean(filepath.Dir(rootExecutablePath)),
		executableName: filepath.Base(rootExecutablePath),
	}
	finder.callback = windows.NewCallback(func(windowHandle uintptr, _ uintptr) uintptr {
		window := windows.HWND(windowHandle)
		if !windows.IsWindowVisible(window) {
			return 1
		}
		processID, executablePath, ok := windowProcess(window)
		if !ok || !finder.matches(executablePath) {
			return 1
		}
		finder.found = windowsApplicationWindow{
			handle:    window,
			processID: processID,
		}
		return 1
	})
	return finder
}

func (finder *windowsApplicationWindowFinder) Find() (windowsApplicationWindow, error) {
	finder.found = windowsApplicationWindow{}
	if err := windows.EnumWindows(finder.callback, nil); err != nil {
		return windowsApplicationWindow{}, err
	}
	return finder.found, nil
}

func (finder *windowsApplicationWindowFinder) matches(executablePath string) bool {
	if !strings.EqualFold(filepath.Base(executablePath), finder.executableName) {
		return false
	}
	relativePath, err := filepath.Rel(finder.installRoot, filepath.Clean(executablePath))
	if err != nil || relativePath == "." || relativePath == ".." {
		return false
	}
	return !strings.HasPrefix(relativePath, ".."+string(filepath.Separator))
}

func windowProcess(window windows.HWND) (uint32, string, bool) {
	var processID uint32
	if _, err := windows.GetWindowThreadProcessId(window, &processID); err != nil {
		return 0, "", false
	}
	handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		processID,
	)
	if err != nil {
		return 0, "", false
	}
	defer windows.CloseHandle(handle)

	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return 0, "", false
	}
	return processID, windows.UTF16ToString(buffer[:size]), true
}

func activateWindowsApplicationWindow(window windowsApplicationWindow) relaunchResult {
	result := relaunchResult{
		windowDetected:  true,
		windowProcessID: window.processID,
	}
	showCommand := uintptr(windows.SW_SHOW)
	if callWindowProcedure(isIconicProcedure, uintptr(window.handle)) {
		showCommand = windows.SW_RESTORE
	}
	result.windowRestored = callWindowProcedure(
		showWindowAsyncProcedure,
		uintptr(window.handle),
		showCommand,
	)
	result.windowBroughtToFront = bringWindowsWindowToFront(window.handle)
	_ = callWindowProcedure(setForegroundWindowProcedure, uintptr(window.handle))
	result.windowForeground = windows.GetForegroundWindow() == window.handle
	if !result.windowForeground {
		result.windowFlashed = flashWindowsWindow(window.handle)
	}
	return result
}

func bringWindowsWindowToFront(window windows.HWND) bool {
	flags := uintptr(
		setWindowPositionNoSize |
			setWindowPositionNoMove |
			setWindowPositionShowWindow,
	)
	topMost := ^uintptr(0)
	notTopMost := ^uintptr(1)
	madeTopMost := callWindowProcedure(
		setWindowPositionProcedure,
		uintptr(window),
		topMost,
		0,
		0,
		0,
		0,
		flags,
	)
	restoredZOrder := callWindowProcedure(
		setWindowPositionProcedure,
		uintptr(window),
		notTopMost,
		0,
		0,
		0,
		0,
		flags,
	)
	broughtToTop := callWindowProcedure(bringWindowToTopProcedure, uintptr(window))
	return madeTopMost && restoredZOrder && broughtToTop
}

func flashWindowsWindow(window windows.HWND) bool {
	info := flashWindowInfo{
		size:      uint32(unsafe.Sizeof(flashWindowInfo{})),
		window:    window,
		flags:     flashWindowAll | flashWindowUntilForeground,
		count:     0,
		timeoutMS: 0,
	}
	return callWindowProcedure(
		flashWindowExProcedure,
		uintptr(unsafe.Pointer(&info)),
	)
}

func callWindowProcedure(procedure *windows.LazyProc, arguments ...uintptr) bool {
	result, _, _ := procedure.Call(arguments...)
	return result != 0
}

func hiddenWindowsProcessAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW,
		HideWindow:    true,
	}
}
