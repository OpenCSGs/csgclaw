//go:build windows

package processgroup

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var jobs = struct {
	sync.Mutex
	handles map[int]windows.Handle
}{handles: map[int]windows.Handle{}}

// Assign the suspended process before it can launch descendants. Closing the
// owner's handles also terminates commands when CSGClaw exits unexpectedly.
func Start(cmd *exec.Cmd) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		windows.CloseHandle(job)
		return err
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	if err = cmd.Start(); err != nil {
		windows.CloseHandle(job)
		return err
	}
	fail := func(err error) error { _ = cmd.Process.Kill(); _ = cmd.Wait(); windows.CloseHandle(job); return err }
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return fail(err)
	}
	err = windows.AssignProcessToJobObject(job, process)
	windows.CloseHandle(process)
	if err != nil {
		return fail(err)
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fail(err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	err = windows.Thread32First(snapshot, &entry)
	for err == nil {
		if entry.OwnerProcessID == uint32(cmd.Process.Pid) {
			thread, openErr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
			if openErr != nil {
				return fail(openErr)
			}
			jobs.Lock()
			jobs.handles[cmd.Process.Pid] = job
			jobs.Unlock()
			_, resumeErr := windows.ResumeThread(thread)
			windows.CloseHandle(thread)
			if resumeErr != nil {
				jobs.Lock()
				delete(jobs.handles, cmd.Process.Pid)
				jobs.Unlock()
				return fail(resumeErr)
			}
			return nil
		}
		err = windows.Thread32Next(snapshot, &entry)
	}
	return fail(errors.New("command primary thread could not be resumed"))
}

type jobAccounting struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

func active(job windows.Handle) (bool, error) {
	var info jobAccounting
	err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil)
	return info.ActiveProcesses != 0, err
}
func Alive(pid int) (bool, error) {
	jobs.Lock()
	defer jobs.Unlock()
	if job, ok := jobs.handles[pid]; ok {
		return active(job)
	}
	return processAlive(pid)
}

func processAlive(pid int) (bool, error) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(handle)
	state, err := windows.WaitForSingleObject(handle, 0)
	return state == uint32(windows.WAIT_TIMEOUT), err
}
func Stop(pid int, force bool) error {
	if pid <= 0 {
		return nil
	}
	jobs.Lock()
	defer jobs.Unlock()
	job, ok := jobs.handles[pid]
	if !ok {
		err := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
		if err != nil {
			// An exited process can still be opened while another caller owns
			// a handle to it. Runtime shutdown and the exit watcher both stop
			// the tree, so inspect its wait state before treating taskkill's
			// "process not found" error as a failed stop.
			if running, probeErr := processAlive(pid); probeErr == nil && !running {
				return nil
			}
			return err
		}
		return nil
	}
	if err := windows.TerminateJobObject(job, 1); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		running, err := active(job)
		if err != nil {
			return err
		}
		if !running {
			delete(jobs.handles, pid)
			return windows.CloseHandle(job)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("command job %d has not exited", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
