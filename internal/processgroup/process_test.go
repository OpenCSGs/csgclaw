package processgroup

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestProcessGroupHelper(t *testing.T) {
	mode := os.Getenv("CSGCLAW_PROCESS_GROUP_TEST")
	switch mode {
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestProcessGroupHelper$")
		child.Env = append(os.Environ(), "CSGCLAW_PROCESS_GROUP_TEST=child")
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		for {
			if _, err := os.Stat(os.Getenv("CSGCLAW_PROCESS_GROUP_READY")); err == nil {
				os.Exit(0)
			}
			time.Sleep(5 * time.Millisecond)
		}
	case "child":
		if err := os.WriteFile(os.Getenv("CSGCLAW_PROCESS_GROUP_READY"), []byte("ready"), 0600); err != nil {
			os.Exit(2)
		}
		time.Sleep(time.Minute)
		os.Exit(0)
	}
}
func TestStopAfterParentExited(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestProcessGroupHelper$")
	cmd.Env = append(os.Environ(), "CSGCLAW_PROCESS_GROUP_TEST=parent", "CSGCLAW_PROCESS_GROUP_READY="+filepath.Join(t.TempDir(), "ready"))
	if err := Start(cmd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Stop(cmd.Process.Pid, true) })
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parent did not exit")
	}
	if alive, err := Alive(cmd.Process.Pid); err != nil || !alive {
		t.Fatalf("child alive=%v err=%v", alive, err)
	}
	if err := Stop(cmd.Process.Pid, true); err != nil {
		t.Fatal(err)
	}
	if alive, err := Alive(cmd.Process.Pid); err != nil || alive {
		t.Fatalf("remaining child alive=%v err=%v", alive, err)
	}
	if err := Stop(cmd.Process.Pid, true); err != nil {
		t.Fatalf("repeated stop: %v", err)
	}
}
