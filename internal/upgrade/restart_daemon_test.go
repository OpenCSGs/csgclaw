package upgrade

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	appbootstrap "csgclaw/internal/app"
)

func TestRestartDaemonWaitsForRuntimeLockRelease(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pidPath := filepath.Join(home, ".csgclaw", "server.pid")
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(pidPath), err)
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", pidPath, err)
	}

	instanceLock, err := appbootstrap.AcquireInstanceLock()
	if err != nil {
		t.Fatalf("AcquireInstanceLock() error = %v", err)
	}
	t.Cleanup(func() { _ = instanceLock.Release() })

	originalExec := execCommandContext
	t.Cleanup(func() { execCommandContext = originalExec })

	executableDir := t.TempDir()
	executablePath := filepath.Join(executableDir, "csgclaw")
	lockReleased := make(chan struct{})
	releaseErr := make(chan error, 1)
	var releaseOnce sync.Once
	startBeforeRelease := false
	execCommandContext = func(_ context.Context, _ string, args ...string) *exec.Cmd {
		if len(args) == 1 && args[0] == "stop" {
			releaseOnce.Do(func() {
				go func() {
					time.Sleep(75 * time.Millisecond)
					releaseErr <- instanceLock.Release()
					close(lockReleased)
				}()
			})
		}
		if len(args) >= 2 && args[len(args)-2] == "serve" && args[len(args)-1] == "--daemon" {
			select {
			case <-lockReleased:
			default:
				startBeforeRelease = true
			}
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	result, err := RestartDaemon(context.Background(), executablePath, RestartOptions{})
	if err != nil {
		t.Fatalf("RestartDaemon() error = %v", err)
	}
	if startBeforeRelease {
		t.Fatal("RestartDaemon() started the new daemon before the old daemon released runtime.lock")
	}
	if !result.DaemonWasRunning || !result.Restarted {
		t.Fatalf("RestartDaemon() = %#v, want restarted daemon", result)
	}
	if err := <-releaseErr; err != nil {
		t.Fatalf("Release() error = %v", err)
	}
}
