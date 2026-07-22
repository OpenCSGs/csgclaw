//go:build linux

package codex

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

func stopRuntimeProcessesUsingDir(runtimeDir string) ([]int, error) {
	runtimeDir = strings.TrimSpace(runtimeDir)
	if runtimeDir == "" {
		return nil, fmt.Errorf("runtime directory is required")
	}
	runtimeDir, err := filepath.Abs(runtimeDir)
	if err != nil {
		return nil, fmt.Errorf("resolve runtime directory: %w", err)
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("read process table: %w", err)
	}

	pids := make([]int, 0)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 || pid == os.Getpid() {
			continue
		}
		matches, err := processUsesCodexRuntime(pid, runtimeDir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
				continue
			}
			return nil, err
		}
		if matches {
			pids = append(pids, pid)
		}
	}
	sort.Ints(pids)

	var stopErr error
	stopped := make([]int, 0, len(pids))
	for _, pid := range pids {
		if err := stopProcess(pid); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
			stopErr = errors.Join(stopErr, fmt.Errorf("stop process %d: %w", pid, err))
			continue
		}
		stopped = append(stopped, pid)
	}
	return stopped, stopErr
}

func processUsesCodexRuntime(pid int, runtimeDir string) (bool, error) {
	procDir := filepath.Join("/proc", strconv.Itoa(pid))
	info, err := os.Stat(procDir)
	if err != nil {
		return false, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return false, nil
	}

	cmdline, err := os.ReadFile(filepath.Join(procDir, "cmdline"))
	if err != nil {
		return false, err
	}
	args := splitNullTerminated(cmdline)
	if !isCodexAppServerCommand(args) {
		return false, nil
	}

	environ, err := os.ReadFile(filepath.Join(procDir, "environ"))
	if err != nil {
		return false, err
	}
	for _, entry := range splitNullTerminated(environ) {
		key, value, ok := strings.Cut(entry, "=")
		if ok && key == "CODEX_HOME" {
			return pathWithinRuntimeDir(runtimeDir, value), nil
		}
	}
	return false, nil
}
