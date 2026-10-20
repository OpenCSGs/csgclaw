//go:build windows

package processgroup

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestStopExitedProcessWithRetainedHandle(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		name := "untracked"
		if tracked {
			name = "job"
		}
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestProcessGroupHelper$")
			var err error
			if tracked {
				err = Start(cmd)
			} else {
				err = cmd.Start()
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = Stop(cmd.Process.Pid, true) })

			// Keep the PID openable even after Wait releases exec.Cmd's handle,
			// as happens when shutdown races with another process owner.
			held, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
			if err != nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatal(err)
			}
			defer windows.CloseHandle(held)
			if err := cmd.Wait(); err != nil {
				t.Fatal(err)
			}
			probe, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
			if err != nil {
				t.Fatalf("retained process must remain openable after exit: %v", err)
			}
			windows.CloseHandle(probe)
			if alive, err := Alive(cmd.Process.Pid); err != nil || alive {
				t.Fatalf("exited process alive=%v err=%v", alive, err)
			}
			for attempt := range 3 {
				if err := Stop(cmd.Process.Pid, true); err != nil {
					t.Fatalf("stop attempt %d with retained handle: %v", attempt+1, err)
				}
			}
		})
	}
}

func TestStopPreservesTaskkillFailureForRunningProcess(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestProcessGroupHelper$")
	cmd.Env = append(os.Environ(), "CSGCLAW_PROCESS_GROUP_TEST=child", "CSGCLAW_PROCESS_GROUP_READY="+filepath.Join(t.TempDir(), "ready"))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	// Make taskkill unavailable while leaving the target alive. A failed stop
	// must still surface an error when no exit has been confirmed.
	t.Setenv("PATH", t.TempDir())
	if err := Stop(cmd.Process.Pid, true); err == nil {
		t.Fatal("stop of a running process succeeded without taskkill")
	}
	if alive, err := Alive(cmd.Process.Pid); err != nil || !alive {
		t.Fatalf("running process alive=%v err=%v", alive, err)
	}
}
