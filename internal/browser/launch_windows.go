//go:build windows

package browser

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"csgclaw/internal/processgroup"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"golang.org/x/sys/windows"
)

// Own Chromium and its descendants with the same native Windows job used for
// Agent processes. Rod's extracted leakless helper is quarantined by Defender
// on supported Windows installations, before Chromium can even start.
func launchChromium(ctx context.Context, l *launcher.Launcher) (string, func(), error) {
	profile, err := os.MkdirTemp("", "csgclaw-browser-")
	if err != nil {
		return "", nil, err
	}
	l.UserDataDir(profile).RemoteDebuggingPort(0).Leakless(false)
	log, err := os.Create(filepath.Join(profile, "startup.log"))
	if err != nil {
		_ = os.RemoveAll(profile)
		return "", nil, err
	}
	cmd := exec.Command(l.Get(flags.Bin), l.FormatArgs()...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Stdout, cmd.Stderr = log, log
	if err := processgroup.Start(cmd); err != nil {
		_ = log.Close()
		_ = os.RemoveAll(profile)
		return "", nil, err
	}
	// Keep the initial PID reserved even after an Edge launcher hands off to
	// a child. The job registry must never confuse a reused PID with its owner.
	owner, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = processgroup.Stop(cmd.Process.Pid, true)
		_ = cmd.Wait()
		_ = log.Close()
		_ = os.RemoveAll(profile)
		return "", nil, err
	}
	var stopOnce, cleanupOnce sync.Once
	stopProcess := func() {
		stopOnce.Do(func() {
			if err := processgroup.Stop(cmd.Process.Pid, true); err != nil {
				slog.Warn("stop browser process tree", "pid", cmd.Process.Pid, "error", err)
			}
		})
	}
	done := make(chan struct{})
	var exitErr error
	go func() {
		exitErr = cmd.Wait()
		_ = log.Close()
		// Edge can hand startup to a child and exit successfully. The job,
		// rather than the initial PID, owns the browser's complete lifetime.
		for {
			alive, err := processgroup.Alive(cmd.Process.Pid)
			if err != nil || !alive {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		stopProcess()
		_ = windows.CloseHandle(owner)
		close(done)
	}()
	stop := func() {
		cleanupOnce.Do(func() {
			stopProcess()
			select {
			case <-done:
				_ = os.RemoveAll(profile)
			case <-time.After(5 * time.Second):
				slog.Warn("browser process cleanup timed out", "pid", cmd.Process.Pid)
			}
		})
	}
	fail := func(err error) (string, func(), error) { stop(); return "", nil, err }
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if raw, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort")); err == nil {
			lines := strings.Fields(string(raw))
			if len(lines) == 2 {
				port, err := strconv.Atoi(lines[0])
				if err == nil && port > 0 && port <= 65535 && strings.HasPrefix(lines[1], "/devtools/browser/") {
					return "ws://127.0.0.1:" + lines[0] + lines[1], stop, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return fail(ctx.Err())
		case <-done:
			output, _ := os.ReadFile(filepath.Join(profile, "startup.log"))
			if len(output) > 4096 {
				output = output[len(output)-4096:]
			}
			return fail(fmt.Errorf("Chromium exited before its debugger became ready (exit: %v): %s", exitErr, strings.TrimSpace(string(output))))
		case <-ticker.C:
		}
	}
}
